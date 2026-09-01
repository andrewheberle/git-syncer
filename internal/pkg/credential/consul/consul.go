package consul

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/andrewheberle/git-syncer/internal/pkg/credential"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/hashicorp/consul/api/v2"
	"golang.org/x/crypto/ssh"
)

type Fetcher struct {
	clientCert         string
	clientKey          string
	clientCaKeys       string
	httpUserKey        string
	httpPasswordKey    string
	auth               string
	logger             *slog.Logger
	ttl                time.Duration
	sshHostKeyCallback ssh.HostKeyCallback

	client   *api.KV
	mu       sync.Mutex
	username string
	password string
	expiry   time.Time
}

func New(addr string, opts ...Option) (*Fetcher, error) {
	f := &Fetcher{
		logger: slog.New(slog.DiscardHandler),
		ttl:    time.Minute * 5,
		auth:   credential.BasicAuth,
	}

	for _, o := range opts {
		o(f)
	}

	if f.auth != credential.BasicAuth && f.auth != credential.BearerAuth {
		return nil, fmt.Errorf("invalid auth type: %s", f.auth)
	}

	f.logger = f.logger.With("addr", addr, "userkey", f.httpUserKey, "passwordkey", f.httpPasswordKey)

	tlsConfig := api.TLSConfig{}
	if f.clientCert != "" && f.clientKey != "" {
		f.logger = f.logger.With("cert", f.clientCert, "key", f.clientKey)
		tlsConfig.CertFile = f.clientCert
		tlsConfig.KeyFile = f.clientKey
	}

	if f.clientCaKeys != "" {
		f.logger = f.logger.With("ca", f.clientCaKeys)
		tlsConfig.CAFile = f.clientCaKeys
	}

	conf := api.DefaultConfig()
	conf.Address = addr
	conf.TLSConfig = tlsConfig

	client, err := api.NewClient(conf)
	if err != nil {
		return nil, err
	}
	f.client = client.KV()

	f.logger.Debug("completed set up of consul based credential fetcher")

	return f, nil
}

func (f *Fetcher) Authorizer(r *http.Request) error {
	username, password, err := f.fetchCredentials()
	if err != nil {
		return err
	}

	if f.auth == credential.BasicAuth {
		r.SetBasicAuth(username, password)
	} else {
		r.Header.Set("Authorization", "Bearer "+password)
	}

	return nil
}

var ErrNoHostKeyCallbackSet = errors.New("a host key callback must be set")

func (f *Fetcher) ClientConfig(context.Context, *transport.Request) (*ssh.ClientConfig, error) {
	if f.sshHostKeyCallback == nil {
		return nil, ErrNoHostKeyCallbackSet
	}

	_, password, err := f.fetchCredentials()
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey([]byte(password))
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		HostKeyCallback: f.sshHostKeyCallback,
		User:            "git",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
	}, nil
}

func (f *Fetcher) fetchCredentials() (username, password string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// check if cached
	if time.Now().Before(f.expiry) {
		return f.username, f.password, nil
	}

	// fetch password
	f.logger.Debug("fetching password from consul", "key", f.httpPasswordKey)
	kv, _, err := f.client.Get(f.httpPasswordKey, nil)
	if err != nil {
		return "", "", fmt.Errorf("error fetching password key %s: %w", f.httpPasswordKey, err)
	}
	if kv == nil {
		return "", "", fmt.Errorf("password key not found %s", f.httpPasswordKey)
	}
	password = string(kv.Value)

	// set or fetch username
	if f.httpUserKey != "" {
		kv, _, err := f.client.Get(f.httpUserKey, nil)
		if err != nil {
			return "", "", fmt.Errorf("error fetching username key %s: %w", f.httpUserKey, err)
		}
		if kv == nil {
			return "", "", fmt.Errorf("username key not found: %s", f.httpUserKey)
		}

		username = string(kv.Value)
	}

	// save for later
	f.username = username
	f.password = password
	f.expiry = time.Now().Add(f.ttl)

	return username, password, nil
}
