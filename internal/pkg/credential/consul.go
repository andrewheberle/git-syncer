package credential

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/consul/api/v2"
)

type ConsulFetcher struct {
	clientCert      string
	clientKey       string
	clientCaKeys    string
	httpUserKey     string
	httpPasswordKey string
	logger          *slog.Logger
	ttl             time.Duration

	client   *api.KV
	mu       sync.Mutex
	username string
	password string
	expiry   time.Time
}

var _ Fetcher = &ConsulFetcher{}

func NewConsul(addr string, opts ...ConsulFetcherOption) (*ConsulFetcher, error) {
	f := &ConsulFetcher{
		logger: slog.New(slog.DiscardHandler),
		ttl:    time.Minute * 5,
	}

	for _, o := range opts {
		o(f)
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

func (f *ConsulFetcher) Authorizer(r *http.Request) error {
	username, password, err := f.fetchCredentials()
	if err != nil {
		return err
	}

	r.SetBasicAuth(username, password)

	return nil
}

func (f *ConsulFetcher) fetchCredentials() (username, password string, err error) {
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

type ConsulFetcherOption func(*ConsulFetcher)

func WithHTTPKeys(userkey, passwordkey string) ConsulFetcherOption {
	return func(f *ConsulFetcher) {
		f.httpUserKey = userkey
		f.httpPasswordKey = passwordkey
	}
}

func WithClientTLS(cert, key string) ConsulFetcherOption {
	return func(f *ConsulFetcher) {
		f.clientCert = cert
		f.clientKey = key
	}
}

func WithClientCA(ca string) ConsulFetcherOption {
	return func(f *ConsulFetcher) {
		f.clientCaKeys = ca
	}
}

func WithLogger(logger *slog.Logger) ConsulFetcherOption {
	return func(f *ConsulFetcher) {
		f.logger = logger
	}
}
