package certmanager

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/orvice/neo-line/internal/store"
)

// DNSProviderFactory builds DNS-01 challenge providers from stored accounts.
type DNSProviderFactory interface {
	NewProvider(account store.DNSProviderAccount) (challenge.Provider, error)
}

// CloudflareDNSFactory creates lego Cloudflare DNS-01 providers.
type CloudflareDNSFactory struct {
	HTTPClient *http.Client
}

func NewCloudflareDNSFactory(httpClient *http.Client) *CloudflareDNSFactory {
	return &CloudflareDNSFactory{HTTPClient: httpClient}
}

func (f *CloudflareDNSFactory) NewProvider(account store.DNSProviderAccount) (challenge.Provider, error) {
	if account.Provider != "" && account.Provider != store.DNSProviderCloudflare {
		return nil, store.ErrInvalidDNSProvider
	}
	cfg := cloudflare.NewDefaultConfig()
	cfg.AuthToken = account.APIToken
	timeout := account.PropagationTimeoutSeconds
	if timeout == 0 {
		timeout = store.DefaultDNSPropagationTimeoutSecs
	}
	cfg.PropagationTimeout = time.Duration(timeout) * time.Second
	cfg.TTL = 120
	if f.HTTPClient != nil {
		cfg.HTTPClient = f.HTTPClient
	}
	provider, err := cloudflare.NewDNSProviderConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("cloudflare dns provider: %w", err)
	}
	return provider, nil
}
