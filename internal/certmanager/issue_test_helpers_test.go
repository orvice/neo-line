package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/orvice/neo-line/internal/store"
)

type fakeIssueACME struct {
	issueFn  func(ctx context.Context, req IssueRequest) (IssueResult, error)
	revokeFn func(ctx context.Context, issuer store.CertificateIssuer, leaf []byte, reason *uint) error
}

func (f *fakeIssueACME) FetchDirectory(context.Context, string) (DirectoryMeta, error) {
	return DirectoryMeta{}, nil
}

func (f *fakeIssueACME) RegisterAccount(context.Context, store.CertificateIssuer) error {
	return nil
}

func (f *fakeIssueACME) IssueCertificate(ctx context.Context, req IssueRequest) (IssueResult, error) {
	if f.issueFn != nil {
		return f.issueFn(ctx, req)
	}
	return IssueResult{}, nil
}

func (f *fakeIssueACME) RevokeCertificate(ctx context.Context, issuer store.CertificateIssuer, leaf []byte, reason *uint) error {
	if f.revokeFn != nil {
		return f.revokeFn(ctx, issuer, leaf, reason)
	}
	return nil
}

type dnsFactoryFunc func(store.DNSProviderAccount) (challenge.Provider, error)

func (fn dnsFactoryFunc) NewProvider(account store.DNSProviderAccount) (challenge.Provider, error) {
	return fn(account)
}

func successIssueACME(t *testing.T, keyType string, cleanupFail bool) *fakeIssueACME {
	t.Helper()
	return &fakeIssueACME{
		issueFn: func(_ context.Context, req IssueRequest) (IssueResult, error) {
			for _, d := range req.Domains {
				if err := req.DNS.Present(d, "tok", "keyAuth-value"); err != nil {
					return IssueResult{}, err
				}
			}
			fullchain, keyPEM := generateTestBundle(t, req.Domains, keyType)
			if cleanupFail {
				if p, ok := req.DNS.(*FakeDNSProvider); ok {
					for _, z := range p.zones {
						z.cleanupFail = true
					}
				}
			}
			for _, d := range req.Domains {
				_ = req.DNS.CleanUp(d, "tok", "keyAuth-value")
			}
			return IssueResult{FullchainPEM: fullchain, PrivateKeyPEM: keyPEM}, nil
		},
	}
}

func generateTestBundle(t *testing.T, domains []string, keyType string) ([]byte, []byte) {
	t.Helper()
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	var leafKey crypto.PrivateKey
	switch keyType {
	case store.CertKeyTypeRSA2048:
		leafKey, err = rsa.GenerateKey(rand.Reader, 2048)
	default:
		leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}

	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: domains[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     domains,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, caTpl, publicKey(leafKey), caKey)
	if err != nil {
		t.Fatal(err)
	}

	keyPEM, err := marshalPrivateKeyPKCS8(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return []byte(buf.String()), keyPEM
}

func publicKey(key crypto.PrivateKey) crypto.PublicKey {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func newIssueTestManager(t *testing.T, st Store, acme ACMEClient, dns *FakeDNSProvider) *Manager {
	t.Helper()
	m := NewManagerWithDeps(st, nil, acme, dnsFactoryFunc(func(store.DNSProviderAccount) (challenge.Provider, error) {
		return dns, nil
	}))
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	m.SetReplicaID("test-replica")
	return m
}

func seedIssueTestStore(st *managedCertFakeStore, domains []string, keyType string, staging bool) (certID, opID string) {
	st.seedReadyIssuer("iss_1")
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		RegistrationStatus: store.IssuerRegistrationReady,
		StagingUntrusted:   staging,
	}
	st.seedDNS("dns_1")
	st.dns["dns_1"] = store.DNSProviderAccount{ID: "dns_1", APIToken: "tok"}
	if keyType == "" {
		keyType = store.CertKeyTypeECP256
	}
	cert := store.ManagedCertificate{
		ID:                   "mcert_1",
		Name:                 "test",
		Domains:              domains,
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              keyType,
	}
	st.certs[cert.ID] = cert
	st.certOrd = []string{cert.ID}
	op := store.CertificateOperation{
		ID:                   "cop_1",
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot: store.IssueConfigSnapshot{
			Domains:              domains,
			CertificateIssuerID:  "iss_1",
			DNSProviderAccountID: "dns_1",
			KeyType:              keyType,
		},
	}
	st.ops[op.ID] = op
	st.opOrd = []string{op.ID}
	return cert.ID, op.ID
}

func failIssueACME(msg string) *fakeIssueACME {
	return &fakeIssueACME{
		issueFn: func(context.Context, IssueRequest) (IssueResult, error) {
			return IssueResult{}, errors.New(msg)
		},
	}
}

func mismatchIssueACME(t *testing.T, domains []string) *fakeIssueACME {
	t.Helper()
	return &fakeIssueACME{
		issueFn: func(_ context.Context, req IssueRequest) (IssueResult, error) {
			fullchain, _ := generateTestBundle(t, domains, store.CertKeyTypeECP256)
			otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			keyPEM, _ := marshalPrivateKeyPKCS8(otherKey)
			return IssueResult{FullchainPEM: fullchain, PrivateKeyPEM: keyPEM}, nil
		},
	}
}
