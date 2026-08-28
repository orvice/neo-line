package certmanager

import (
	"os"
	"testing"

	legodns01 "github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/orvice/neo-line/internal/store"
)

func TestCloudflareDNSFactoryDisablesCNAMEFollowingByDefault(t *testing.T) {
	previous, wasSet := os.LookupEnv(legoDisableCNAMESupportEnv)
	if err := os.Unsetenv(legoDisableCNAMESupportEnv); err != nil {
		t.Fatalf("unset %s: %v", legoDisableCNAMESupportEnv, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(legoDisableCNAMESupportEnv, previous)
			return
		}
		_ = os.Unsetenv(legoDisableCNAMESupportEnv)
	})

	NewCloudflareDNSFactory(nil)

	if got := os.Getenv(legoDisableCNAMESupportEnv); got != "true" {
		t.Fatalf("%s = %q, want true", legoDisableCNAMESupportEnv, got)
	}
	if legoCNAMEFollowEnabled() {
		t.Fatal("CNAME following enabled by default")
	}
	info := legodns01.GetChallengeInfo("host.example.com", "test-key-authorization")
	if info.EffectiveFQDN != "_acme-challenge.host.example.com." {
		t.Fatalf("effective FQDN = %q, want source challenge name", info.EffectiveFQDN)
	}
}

func TestCloudflareDNSFactoryHonorsExplicitCNAMEFollowing(t *testing.T) {
	t.Setenv(legoDisableCNAMESupportEnv, "false")

	NewCloudflareDNSFactory(nil)

	if got := os.Getenv(legoDisableCNAMESupportEnv); got != "false" {
		t.Fatalf("%s = %q, want false", legoDisableCNAMESupportEnv, got)
	}
	if !legoCNAMEFollowEnabled() {
		t.Fatal("explicit CNAME following setting was overridden")
	}
}

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
