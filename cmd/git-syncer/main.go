package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/andrewheberle/git-syncer/internal/pkg/credential/consul"
	"github.com/andrewheberle/git-syncer/internal/pkg/syncer"
	"github.com/go-git/go-git/v6/plumbing/transport/ssh/knownhosts"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/oklog/run"
	"github.com/spf13/pflag"
)

var Version = "dev"

func main() {
	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("config", "", "Path to configuration file")
	f.String("git.url", "", "URL of git repository (only required for the initial clone)")
	f.String("git.workdir", "", "Directory for the git repository")
	f.String("git.remote", "origin", "The git remote name")
	f.String("git.http.auth", "basic", "HTTP Authentication type for git operations")
	f.String("git.ssh.knownhosts", "", "Path to known_hosts file to verify SSH host keys (required for private SSH remotes)")
	f.String("change.command", "", "Command to run on changes")
	f.String("change.filter", ".*", "Filter to limit changes to trigger the configured command (if any)")
	f.Duration("interval", 0, "Refresh interval")
	f.Bool("debug", false, "Enable debug logging")
	f.Bool("version", false, "Show version and exit")
	f.String("consul.addr", "", "Address of Consul KV store")
	f.String("consul.git.user", "", "Consul key that holds HTTP username (used for basic auth only)")
	f.String("consul.git.password", "", "Consul key that holds HTTP password/token or SSH key")
	f.String("consul.cert", "", "Client certificate for Consul authentication")
	f.String("consul.key", "", "Client key for Consul authentication")
	f.String("consul.ca", "", "CA to verify connection to Consul")

	// parse command line
	if err := f.Parse(os.Args[1:]); err != nil {
		if !errors.Is(err, pflag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "error parsing command line flags: %s\n", err)
			os.Exit(1)
		}
	}

	// handle if version was requested
	if version, err := f.GetBool("version"); err == nil && version {
		fmt.Printf("git-syncer %s\n", Version)
		os.Exit(0)
	}

	// load any config file
	k := koanf.New(".")
	if config, err := f.GetString("config"); err != nil {
		fmt.Fprintf(os.Stderr, "error getting flag value: %s\n", err)
		os.Exit(1)
	} else if config != "" {
		if err := k.Load(file.Provider(config), yaml.Parser()); err != nil {
			fmt.Fprintf(os.Stderr, "error loading configuration: %s\n", err)
			os.Exit(1)
		}
	}

	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		fmt.Fprintf(os.Stderr, "error loading configuration: %s\n", err)
		os.Exit(1)
	}

	logLevel := new(slog.LevelVar)
	logger := slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		}),
	).With(
		"interval", k.Duration("interval"),
		slog.Group("git",
			"url", k.String("git.url"),
			"workdir", k.String("git.workdir"),
			"remote", k.String("git.remote"),
		),
		slog.Group("change",
			"command", k.String("change.command"),
			"filter", k.String("change.filter"),
		),
	)

	if k.Bool("debug") {
		logLevel.Set(slog.LevelDebug)
	}

	opts := []syncer.SyncerOption{
		syncer.WithLogger(logger),
		syncer.WithInterval(k.Duration("interval")),
		syncer.WithCommand(k.String("change.command")),
		syncer.WithFilter(k.String("change.filter")),
		syncer.WithRemoteName(k.String("git.remote")),
	}

	if consulAddr := k.String("consul.addr"); consulAddr != "" {
		consulPasswordKey := k.String("consul.git.password")
		if consulPasswordKey == "" {
			logger.Error("value is required for consul.git.password when consul.addr is set")
			os.Exit(1)
		}

		consulUsernameKey := k.String("consul.git.username")

		logger.Debug("keys set for username and password", "git.user", consulUsernameKey, "git.password", consulPasswordKey)

		consulOpts := []consul.Option{
			consul.WithLogger(logger.WithGroup("consul")),
			consul.WithPasswordKey(consulPasswordKey),
			consul.WithHTTPAuth(k.String("git.http.auth")),
		}

		if gitSshKnownHosts := k.String("git.ssh.knownhosts"); gitSshKnownHosts != "" {
			db, err := knownhosts.NewDB(gitSshKnownHosts)
			if err != nil {
				logger.Error("could not set up known_hosts", "error", err)
				os.Exit(1)
			}
			consulOpts = append(consulOpts, consul.WithHostKeyCallback(db.HostKeyCallback()))
		}
		if consulUsernameKey != "" {
			consulOpts = append(consulOpts, consul.WithUserKey(consulUsernameKey))
		}
		if consulClientCA := k.String("consul.ca"); consulClientCA != "" {
			consulOpts = append(consulOpts, consul.WithClientCA(consulClientCA))
		}
		consulClientCert := k.String("consul.cert")
		consulClientKey := k.String("consul.key")
		if consulClientCert != "" && consulClientKey != "" {
			consulOpts = append(consulOpts, consul.WithClientTLS(consulClientCert, consulClientKey))
		}

		fetcher, err := consul.New(consulAddr, consulOpts...)
		if err != nil {
			logger.Error("could not set up fetcher", "error", err)
			os.Exit(1)
		}

		opts = append(opts, syncer.WithHTTPAuth(fetcher), syncer.WithSSHAuth(fetcher))
	}

	s, err := syncer.New(k.String("git.url"), k.String("git.workdir"), opts...)
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
