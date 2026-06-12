package syncer

import (
	"log/slog"
	"time"

	"github.com/go-git/go-git/v6/plumbing/client"
)

type SyncerOption func(*Syncer)

func WithLogger(logger *slog.Logger) SyncerOption {
	return func(s *Syncer) {
		s.logger = logger
	}
}

func WithInterval(interval time.Duration) SyncerOption {
	return func(s *Syncer) {
		s.interval = interval
	}
}

func WithCommand(command string) SyncerOption {
	return func(s *Syncer) {
		s.commandString = command
	}
}

func WithFilter(filter string) SyncerOption {
	return func(s *Syncer) {
		s.filterString = filter
	}
}

func WithSSHAuth(auth client.SSHAuth) SyncerOption {
	return func(s *Syncer) {
		s.gitOptions = append(s.gitOptions, client.WithSSHAuth(auth))
	}
}

func WithHTTPAuth(auth client.HTTPAuth) SyncerOption {
	return func(s *Syncer) {
		s.gitOptions = append(s.gitOptions, client.WithHTTPAuth(auth))
	}
}

func WithRemoteName(remote string) SyncerOption {
	return func(s *Syncer) {
		s.gitRemoteName = remote
	}
}
