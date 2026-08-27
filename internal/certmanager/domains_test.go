package certmanager

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeDomainsUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "dedupe and trim", in: []string{" Foo.com.", "foo.com"}, want: []string{"foo.com"}},
		{name: "wildcard", in: []string{"*.example.com"}, want: []string{"*.example.com"}},
		{name: "empty", in: []string{}, wantErr: true},
		{name: "bad wildcard", in: []string{"*.*.example.com"}, wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDomains(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestNormalizeDomainsIDNA(t *testing.T) {
	got, err := NormalizeDomains([]string{"München.de"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "xn--mnchen-3ya.de" {
		t.Fatalf("got %q", got[0])
	}
}

func TestNormalizeKeyTypeViaCreate(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)
	got, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "rsa",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              "rsa_2048",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyType != "rsa_2048" {
		t.Fatalf("key type = %q", got.KeyType)
	}
}

func TestInvalidKeyType(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)
	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "bad-key",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              "ed25519",
	})
	if !errors.Is(err, ErrInvalidKeyType) {
		t.Fatalf("got %v", err)
	}
}
