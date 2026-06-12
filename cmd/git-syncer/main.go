package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/andrewheberle/git-syncer/internal/pkg/credential"
	"github.com/andrewheberle/git-syncer/internal/pkg/credential/consul"
	"github.com/andrewheberle/git-syncer/internal/pkg/syncer"
	"github.com/oklog/run"
	"github.com/spf13/pflag"
)

var Version = "dev"

func main() {
	var (
		repo              string
		dir               string
		authType          credential.AuthType
		interval          time.Duration
		debug             bool
		version           bool
		filter            string
		command           string
		consulAddr        string
		consulUsernameKey string
		consulPasswordKey string
		consulClientCert  string
		consulClientKey   string
		consulClientCA    string
	)

	pflag.StringVar(&repo, "git.url", "", "URL of git repository (only required for the initial clone)")
	pflag.StringVar(&dir, "git.workdir", "", "Directory for the git repository")
	pflag.Var(&authType, "git.httpauth", "HTTP Authentication type for git operations")
	pflag.StringVar(&command, "change.command", "", "Command to run on changes")
	pflag.StringVar(&filter, "change.filter", ".*", "Filter to limit changes to trigger the configured command (if any)")
	pflag.DurationVar(&interval, "interval", 0, "Refresh interval")
	pflag.BoolVar(&debug, "debug", false, "Enable debug logging")
	pflag.BoolVar(&version, "version", false, "Show version and exit")
	pflag.StringVar(&consulAddr, "consul.addr", "", "Address of Consul KV store")
	pflag.StringVar(&consulUsernameKey, "consul.git.user", "", "Consul key that holds git username")
	pflag.StringVar(&consulPasswordKey, "consul.git.password", "", "Consul key that holds git password")
	pflag.StringVar(&consulClientCert, "consul.cert", "", "Client certificate for Consul authentication")
	pflag.StringVar(&consulClientKey, "consul.key", "", "Client key for Consul authentication")
	pflag.StringVar(&consulClientCA, "consul.ca", "", "CA to verify connection to Consul")
	pflag.Parse()

	if version {
		fmt.Printf("git-syncer %s\n", Version)
		os.Exit(0)
	}

	logLevel := new(slog.LevelVar)
	logger := slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		}),
	).With(
		"interval", interval,
		slog.Group("git", "url", repo, "workdir", dir),
		slog.Group("change", "command", command, "filter", filter),
	)

	if debug {
		logLevel.Set(slog.LevelDebug)
	}

	opts := []syncer.SyncerOption{
		syncer.WithLogger(logger),
		syncer.WithInterval(interval),
		syncer.WithCommand(command),
		syncer.WithFilter(filter),
	}

	if consulAddr != "" {
		if consulPasswordKey == "" {
			logger.Error("value is required for consul.passwordkey when consul.addr is set")
			os.Exit(1)
		}

		logger.Debug("keys set for username and password", "username", consulUsernameKey, "password", consulPasswordKey)

		consulOpts := []consul.Option{
			consul.WithLogger(logger.WithGroup("consul")),
			consul.WithPasswordKey(consulPasswordKey),
			consul.WithHTTPAuth(authType),
		}

		if consulUsernameKey != "" {
			consulOpts = append(consulOpts, consul.WithUserKey(consulUsernameKey))
		}
		if consulClientCA != "" {
			consulOpts = append(consulOpts, consul.WithClientCA(consulClientCA))
		}
		if consulClientCert != "" && consulClientKey != "" {
			consulOpts = append(consulOpts, consul.WithClientTLS(consulClientCert, consulClientKey))
		}

		fetcher, err := consul.New(consulAddr, consulOpts...)
		if err != nil {
			logger.Error("could not set up fetcher", "error", err)
			os.Exit(1)
		}

		opts = append(opts, syncer.WithHTTPAuth(fetcher))
	}

	s, err := syncer.New(repo, dir, opts...)
	if err != nil {
		logger.Error("error setting up syncer", "error", err)
		os.Exit(1)
	}

	g := &run.Group{}
	g.Add(s.Run, func(err error) {
		s.ShutDown()
	})

	ch := make(chan os.Signal, 1)
	g.Add(func() error {

		signal.Notify(ch, os.Interrupt)

		sig, ok := <-ch
		if ok && sig != nil {
			logger.Info("shutting down as interrupt signal was received")
			s.ShutDown()
		}

		return nil
	}, func(err error) {
		signal.Stop(ch)
		close(ch)
	})

	if err := g.Run(); err != nil {
		logger.Error("error during run", "error", err)
		os.Exit(1)
	}
}
