package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/object"
)

type Syncer struct {
	dir string

	interval      time.Duration
	logger        *slog.Logger
	commandString string
	filterString  string
	gitOptions    []client.Option

	command  *exec.Cmd
	filter   *regexp.Regexp
	repo     *git.Repository
	worktree *git.Worktree
	ctx      context.Context
	cancel   context.CancelFunc
}

func New(repo, dir string, opts ...SyncerOption) (*Syncer, error) {
	s := &Syncer{
		dir:          dir,
		logger:       slog.New(slog.DiscardHandler),
		interval:     0,
		filterString: ".*",
		gitOptions:   make([]client.Option, 0),
	}

	// set up context to cancel any operations in progress on shutdown
	s.ctx, s.cancel = context.WithCancel(context.Background())

	for _, o := range opts {
		o(s)
	}

	r, err := s.openOrClone(repo)
	if err != nil {
		return nil, fmt.Errorf("could not open repository: %w", err)
	}
	s.logger.Info("opened git repository")
	s.repo = r

	w, err := r.Worktree()
	if err != nil {
		defer func() { _ = r.Close() }()
		return nil, fmt.Errorf("could not get worktree for repository: %w", err)
	}
	s.worktree = w

	if s.commandString != "" {
		s.command = exec.Command("/bin/sh", "-c", s.commandString)
	}

	if s.filterString != "" {
		re, err := regexp.Compile(s.filterString)
		if err != nil {
			return nil, err
		}
		s.filter = re
	}

	return s, nil
}

func (s *Syncer) Run() error {
	defer s.cancel()

	if err := s.run(); err != nil {
		s.logger.Error("error during initial run", "error", err)
		return err
	}

	if s.interval == 0 {
		s.logger.Info("completed initial sync and exiting as interval was zero")
		return nil
	}

	t := time.NewTicker(s.interval)
	for {
		select {
		case <-t.C:
			s.logger.Info("waking up to run scheduled sync")
			if err := s.run(); err != nil {
				s.logger.Error("error during scheduled sync", "error", err)
			}
		case <-s.ctx.Done():
			s.logger.Info("shutting down")

			if err := s.repo.Close(); err != nil {
				s.logger.Error("error closing repository", "error", err)

				return err
			}

			return nil
		}
	}
}

func (s *Syncer) ShutDown() {
	s.cancel()
}

func (s *Syncer) run() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(time.Second*30))
	defer cancel()

	cur, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("could not get current reference: %w", err)
	}

	curcommit, err := object.GetCommit(s.repo.Storer, cur.Hash())
	if err != nil {
		return fmt.Errorf("could not get current commit: %w", err)
	}

	s.logger.Debug("starting pull", "ref", cur.Hash().String())

	opts := &git.PullOptions{ClientOptions: s.gitOptions}
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("git options validation error: %w", err)
	}

	if err := s.worktree.PullContext(ctx, opts); err != nil {
		if !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("error during fetch: %w", err)
		}
	}

	new, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("could not get new reference: %w", err)
	}

	s.logger.Debug("finished pull", "ref", new.Hash().String())

	if new.Hash().Equal(cur.Hash()) {
		s.logger.Debug("no changes", "ref", cur.Hash().String())
		return nil
	}

	s.logger.Info("pulled changes", "ref", new.Hash().String())

	if s.command == nil {
		s.logger.Debug("no command set")

		return nil
	}

	matched, err := s.changesMatched(curcommit, new.Hash())
	if err != nil {
		return fmt.Errorf("error checking for changes: %w", err)
	}

	if matched {
		s.logger.Debug("running command on detected changes")
		return s.command.Run()
	}

	s.logger.Debug("no changes matched")

	return nil
}

func (s *Syncer) openOrClone(repo string) (*git.Repository, error) {
	stat, err := os.Stat(s.dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("could not open: %w", err)
		}

		return s.clone(repo)
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("was not a directory: %s", s.dir)
	}

	empty, err := isDirEmpty(s.dir)
	if err != nil {
		return nil, err
	}

	if empty {
		return s.clone(repo)
	}

	return git.PlainOpen(s.dir)
}

func (s *Syncer) clone(repo string) (*git.Repository, error) {
	opts := &git.CloneOptions{
		URL:           repo,
		ClientOptions: s.gitOptions,
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid clone options: %w", err)
	}

	s.logger.Info("cloning respository")

	return git.PlainClone(s.dir, opts)
}

func (s *Syncer) changesMatched(prev *object.Commit, h plumbing.Hash) (bool, error) {
	cur, err := object.GetCommit(s.repo.Storer, h)
	if err != nil {
		return false, fmt.Errorf("could not get current commit: %w", err)
	}

	ot, err := prev.Tree()
	if err != nil {
		return false, fmt.Errorf("could not get old tree: %w", err)
	}

	nt, err := cur.Tree()
	if err != nil {
		return false, fmt.Errorf("could not get new tree: %w", err)
	}

	changes, err := ot.Diff(nt)
	if err != nil {
		return false, fmt.Errorf("could not get changes between old and new tree: %w", err)
	}

	for _, change := range changes {
		if change.From.Name != "" {
			if s.filter.MatchString(change.From.Name) {
				s.logger.Debug("change matched", "file", change.From.Name)

				return true, nil
			}

			s.logger.Debug("change did not match", "file", change.From.Name)
		}

		if change.To.Name != "" {
			if s.filter.MatchString(change.To.Name) {
				s.logger.Debug("change matched", "file", change.To.Name)

				return true, nil
			}

			s.logger.Debug("change did not match", "file", change.To.Name)
		}
	}

	s.logger.Debug("no changes matched")

	return false, nil
}

func isDirEmpty(path string) (bool, error) {
	// Open the directory
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to open directory: %w", err)
	}
	defer f.Close()

	// Try reading just one entry
	entries, err := f.Readdirnames(1)
	if err != nil {
		if err == io.EOF {
			return true, nil
		}
		return false, fmt.Errorf("failed to read directory: %w", err)
	}

	// If we got at least one entry, it's not empty
	return len(entries) == 0, nil
}
