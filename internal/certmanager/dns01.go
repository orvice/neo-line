package certmanager

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/orvice/neo-line/internal/store"
)

const legoDisableCNAMESupportEnv = "LEGO_DISABLE_CNAME_SUPPORT"

// DNSProviderFactory builds DNS-01 challenge providers from stored accounts.
type DNSProviderFactory interface {
	NewProvider(account store.DNSProviderAccount) (challenge.Provider, error)
}

// CloudflareDNSFactory creates lego Cloudflare DNS-01 providers.
type CloudflareDNSFactory struct {
	HTTPClient *http.Client
}

func NewCloudflareDNSFactory(httpClient *http.Client) *CloudflareDNSFactory {
	configureLegoDNS01CNAMEPolicy()
	return &CloudflareDNSFactory{HTTPClient: httpClient}
}

func configureLegoDNS01CNAMEPolicy() {
	if _, configured := os.LookupEnv(legoDisableCNAMESupportEnv); !configured {
		_ = os.Setenv(legoDisableCNAMESupportEnv, "true")
	}
}

func legoCNAMEFollowEnabled() bool {
	disabled, _ := strconv.ParseBool(os.Getenv(legoDisableCNAMESupportEnv))
	return !disabled
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
