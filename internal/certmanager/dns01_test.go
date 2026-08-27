package certmanager

import (
	"testing"

	"github.com/orvice/neo-line/internal/store"
)

func TestCloudflareDNSFactoryNewProvider(t *testing.T) {
	f := NewCloudflareDNSFactory(nil)
	provider, err := f.NewProvider(store.DNSProviderAccount{
		Provider:                  store.DNSProviderCloudflare,
		APIToken:                  "test-token",
		PropagationTimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestCloudflareDNSFactoryRejectsUnknownProvider(t *testing.T) {
	f := NewCloudflareDNSFactory(nil)
	_, err := f.NewProvider(store.DNSProviderAccount{
		Provider: "route53",
		APIToken: "x",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
