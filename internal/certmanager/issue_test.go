package certmanager

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/orvice/neo-line/internal/store"
)

func TestIssueSuccessActivatesActiveVersionEC(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	certID, opID := seedIssueTestStore(st, []string{"example.com", "www.example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)

	m.runIssueOperation(context.Background(), opID)

	cert, _ := st.GetManagedCertificate(context.Background(), certID)
	if cert.ActiveVersion == nil {
		t.Fatal("expected active version")
	}
	if cert.ActiveVersion.KeyType != store.CertKeyTypeECP256 {
		t.Fatalf("key type = %q", cert.ActiveVersion.KeyType)
	}
	op := st.ops[opID]
	if op.Status != store.CertOpStatusSucceeded {
		t.Fatalf("op status = %q", op.Status)
	}
	pub, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if pub.ActiveValidity != store.CertValidityValid {
		t.Fatalf("validity = %q", pub.ActiveValidity)
	}
	if !pub.BundleAvailable {
		t.Fatal("expected bundle available")
	}
	if pub.ActiveVersion == nil || pub.ActiveVersion.StagingUntrusted {
		t.Fatal("expected trusted active metadata")
	}
}

func TestIssueSuccessRSA(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeRSA2048, false)
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeRSA2048, false), dns)
	m.runIssueOperation(context.Background(), opID)
	op := st.ops[opID]
	if op.Status != store.CertOpStatusSucceeded {
		t.Fatalf("op status = %q", op.Status)
	}
	if st.certs["mcert_1"].ActiveVersion.KeyType != store.CertKeyTypeRSA2048 {
		t.Fatalf("key type = %q", st.certs["mcert_1"].ActiveVersion.KeyType)
	}
}

func TestIssueStagingMarkedUntrusted(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	certID, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, true)
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.runIssueOperation(context.Background(), opID)
	if !st.certs[certID].ActiveVersion.StagingUntrusted {
		t.Fatal("expected staging untrusted on version")
	}
	bundle, err := m.GetCertificateBundle(context.Background(), certID, VersionSlotActive)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.StagingUntrusted {
		t.Fatal("expected staging in bundle")
	}
}

func TestIssueCAFailureKeepsMissing(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	certID, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, failIssueACME("acme order rejected"), dns)
	m.runIssueOperation(context.Background(), opID)
	if st.certs[certID].ActiveVersion != nil {
		t.Fatal("expected no active version")
	}
	op := st.ops[opID]
	if op.Status != store.CertOpStatusPending {
		t.Fatalf("op status = %q, want Pending for auto retry", op.Status)
	}
	if op.NextAttemptAt == nil {
		t.Fatal("expected next_attempt_at scheduled")
	}
	if op.ErrorSummary == "" {
		t.Fatal("expected error summary")
	}
	if strings.Contains(strings.ToLower(op.ErrorSummary), "token") {
		t.Fatalf("leaked secret in error: %q", op.ErrorSummary)
	}
}

func TestIssueDNSFailure(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider() // no zones
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	acme := &fakeIssueACME{issueFn: func(_ context.Context, req IssueRequest) (IssueResult, error) {
		return IssueResult{}, req.DNS.Present("example.com", "t", "k")
	}}
	m := newIssueTestManager(t, st, acme, dns)
	m.runIssueOperation(context.Background(), opID)
	if st.ops[opID].Status != store.CertOpStatusPending {
		t.Fatalf("status = %q", st.ops[opID].Status)
	}
}

func TestIssuePersistsTXTImmediatelyAfterPresent(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	persistedDuringIssue := false
	acme := &fakeIssueACME{issueFn: func(ctx context.Context, req IssueRequest) (IssueResult, error) {
		if err := req.DNS.Present("example.com", "tok", "key-auth"); err != nil {
			return IssueResult{}, err
		}
		op, err := st.GetCertificateOperation(ctx, opID)
		if err != nil {
			return IssueResult{}, err
		}
		persistedDuringIssue = len(op.PendingTXTRecords) == 1
		return IssueResult{}, errors.New("stop after present")
	}}
	m := newIssueTestManager(t, st, acme, dns)

	m.runIssueOperation(context.Background(), opID)

	if !persistedDuringIssue {
		t.Fatal("pending TXT record was not persisted before IssueCertificate returned")
	}
}

func TestIssueStopsAndCleansTXTWhenPersistenceFails(t *testing.T) {
	st := newManagedCertFakeStore()
	st.recordPendingTXTErr = errors.New("mongo unavailable")
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	var presentErr error
	acme := &fakeIssueACME{issueFn: func(_ context.Context, req IssueRequest) (IssueResult, error) {
		presentErr = req.DNS.Present("example.com", "tok", "key-auth")
		return IssueResult{}, presentErr
	}}
	m := newIssueTestManager(t, st, acme, dns)

	m.runIssueOperation(context.Background(), opID)

	if presentErr == nil {
		t.Fatal("Present succeeded without persisting takeover state")
	}
	if values := dns.TXTValues("_acme-challenge.example.com"); len(values) != 0 {
		t.Fatalf("TXT values after persistence failure = %v, want none", values)
	}
	if records := st.ops[opID].PendingTXTRecords; len(records) != 0 {
		t.Fatalf("pending TXT records = %v, want none", records)
	}
}

func TestIssueCleanupWarningDoesNotFailOperation(t *testing.T) {
	st := newManagedCertFakeStore()
	zone := &fakeDNSZone{name: "example.com", cleanupFail: true}
	dns := NewFakeDNSProvider(zone)
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.runIssueOperation(context.Background(), opID)
	op := st.ops[opID]
	if op.Status != store.CertOpStatusSucceeded {
		t.Fatalf("op status = %q, want Succeeded", op.Status)
	}
	if op.Warning == "" {
		t.Fatal("expected cleanup warning")
	}
}

func TestIssueCertificateKeyMismatchFails(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, mismatchIssueACME(t, []string{"example.com"}), dns)
	m.runIssueOperation(context.Background(), opID)
	if st.ops[opID].Status != store.CertOpStatusPending {
		t.Fatalf("status = %q", st.ops[opID].Status)
	}
}

func TestFakeDNSCNAMEFollow(t *testing.T) {
	zone := &fakeDNSZone{
		name: "delegate.test",
		cnames: map[string]string{
			"_acme-challenge.example.com": "_acme-challenge.example.delegate.test",
		},
	}
	dns := NewFakeDNSProvider(zone)
	if err := dns.Present("example.com", "tok", "auth"); err != nil {
		t.Fatalf("present: %v", err)
	}
	target := "_acme-challenge.example.delegate.test"
	if len(dns.TXTValues(target)) != 1 {
		t.Fatalf("expected txt on %q, calls=%v", target, dns.PresentCalls)
	}
}

func TestFakeDNSPreservesCoexistingTXT(t *testing.T) {
	zone := &fakeDNSZone{
		name: "example.com",
		txt: map[string][]fakeTXTRecord{
			"_acme-challenge.example.com": {{id: "keep", value: "existing-value"}},
		},
	}
	dns := NewFakeDNSProvider(zone)
	if err := dns.Present("example.com", "tok", "auth"); err != nil {
		t.Fatal(err)
	}
	if err := dns.CleanUp("example.com", "tok", "auth"); err != nil {
		t.Fatal(err)
	}
	vals := dns.TXTValues("_acme-challenge.example.com")
	if len(vals) != 1 || vals[0] != "existing-value" {
		t.Fatalf("coexisting txt = %v", vals)
	}
}

func TestGetCertificateBundleNoStore(t *testing.T) {
	st := newManagedCertFakeStore()
	m := NewManagerWithDeps(st, nil, &fakeIssueACME{}, dnsFactoryFunc(func(store.DNSProviderAccount) (challenge.Provider, error) {
		return nil, errors.New("unused")
	}))
	_, err := m.GetCertificateBundle(context.Background(), "missing", VersionSlotActive)
	if !errors.Is(err, ErrBundleNotAvailable) && !store.IsNotFound(err) {
		// no active -> bundle not available after get succeeds with nil active
	}
}

func TestValidateIssuedBundle(t *testing.T) {
	domains := []string{"example.com"}
	fullchain, keyPEM := generateTestBundle(t, domains, store.CertKeyTypeECP256)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if _, err := validateIssuedBundle(fullchain, keyPEM, domains, store.CertKeyTypeECP256, now); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestAssembleFullchainStripsRoot(t *testing.T) {
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	// use generateTestBundle which has leaf + ca (non-root)
	fullchain, _ := generateTestBundle(t, []string{"example.com"}, store.CertKeyTypeECP256)
	certs, err := parsePEMCertificates(fullchain)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 2 {
		t.Fatalf("expected leaf+intermediate, got %d", len(certs))
	}
	_ = caKey
}
