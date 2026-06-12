package consul

import (
	"log/slog"

	"github.com/andrewheberle/git-syncer/internal/pkg/credential"
	"golang.org/x/crypto/ssh"
)

type Option func(*Fetcher)

func WithUserKey(key string) Option {
	return func(f *Fetcher) {
		f.httpUserKey = key
	}
}

func WithPasswordKey(key string) Option {
	return func(f *Fetcher) {
		f.httpPasswordKey = key
	}
}

func WithHTTPAuth(auth credential.AuthType) Option {
	return func(f *Fetcher) {
		f.auth = auth.String()
	}
}

func WithClientTLS(cert, key string) Option {
	return func(f *Fetcher) {
		f.clientCert = cert
		f.clientKey = key
	}
}

func WithClientCA(ca string) Option {
	return func(f *Fetcher) {
		f.clientCaKeys = ca
	}
}

func WithHostKeyCallback(fn ssh.HostKeyCallback) Option {
	return func(f *Fetcher) {
		f.sshHostKeyCallback = fn
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(f *Fetcher) {
		f.logger = logger
	}
}
