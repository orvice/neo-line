package certmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func seedRevokeTestCert(t *testing.T, st *managedCertFakeStore, activeID string) string {
	t.Helper()
	st.seedReadyIssuer("iss_1")
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		RegistrationStatus: store.IssuerRegistrationReady,
		AccountKeyPEM:      testAccountKeyPEM,
		DirectoryURL:       "https://acme.test/directory",
		Email:              "admin@example.com",
	}
	st.seedDNS("dns_1")
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	fullchain, _ := generateTestBundle(t, []string{"example.com"}, store.CertKeyTypeECP256)
	certID := "mcert_revoke"
	snap := store.IssueConfigSnapshot{
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              store.CertKeyTypeECP256,
	}
	st.certs[certID] = store.ManagedCertificate{
		ID:                   certID,
		Name:                 "revoke-test",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              store.CertKeyTypeECP256,
		ActiveVersion: &store.CertificateVersion{
			ID:                  activeID,
			ConfigSnapshot:      snap,
			FullchainPEM:        string(fullchain),
			PrivateKeyPEM:       "KEY",
			CertificateIssuerID: "iss_1",
			NotBefore:           notBefore,
			NotAfter:            notAfter,
			KeyType:             store.CertKeyTypeECP256,
			CreatedAt:           notBefore,
		},
	}
	st.certOrd = []string{certID}
	return certID
}

const testAccountKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIBs3fK8mH8V+3Q8Y5nJ1mK2pL4rT6vW9xA0bC1dE2fG3hI4jK5lM6nO7p
Q8rS9tU0vW1xY2zA3bC4dE5fG6hI7jK8lM9nO0pQ1rS2tU3vW4xY5zA6bC7dE8fG9
hI0jK1lM2nO3pQ4rS5tU6vW7xY8zA9bC0dE1fG2hI3jK4lM5nO6pQ7rS8tU9vW0xY1
zA2bC3dE4fG5hI6jK7lM8nO9pQ0rS1tU2vW3xY4zA5bC6dE7fG8hI9jK0lM1nO2pQ3
-----END EC PRIVATE KEY-----`

func TestSubmitRevokeVersionImmediateBlock(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	m := NewManagerWithACME(st, nil, &fakeIssueACME{})
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}

	op, err := m.SubmitRevokeVersion(context.Background(), certID, activeID, 0)
	if err != nil {
		t.Fatalf("submit revoke: %v", err)
	}
	if op.Type != store.CertOpTypeRevoke {
		t.Fatalf("op type = %q", op.Type)
	}
	cert := st.certs[certID]
	if !cert.ActiveVersion.RevokePending {
		t.Fatal("expected revoke_pending immediately")
	}
	pub, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if pub.BundleAvailable {
		t.Fatal("expected bundle unavailable during pending revoke")
	}
	if pub.ActiveValidity != store.CertValidityRevoked {
		t.Fatalf("validity = %q, want Revoked", pub.ActiveValidity)
	}
}

func TestRevokeCAFailureKeepsBlockAndRetries(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	acme := &fakeIssueACME{
		revokeFn: func(context.Context, store.CertificateIssuer, []byte, *uint) error {
			return errors.New("acme revoke failed")
		},
	}
	m := NewManagerWithACME(st, nil, acme)
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	m.SetReplicaID("test-replica")
	m.SetJitter(func(time.Duration) time.Duration { return 0 })

	op, err := m.SubmitRevokeVersion(context.Background(), certID, activeID, 1)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.runOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if !cert.ActiveVersion.RevokePending {
		t.Fatal("expected revoke_pending after CA failure")
	}
	if cert.ActiveVersion.RevokedAt != nil {
		t.Fatal("expected not fully revoked yet")
	}
	stored := st.ops[op.ID]
	if stored.Status != store.CertOpStatusPending {
		t.Fatalf("op status = %q, want Pending for retry", stored.Status)
	}
	if stored.NextAttemptAt == nil {
		t.Fatal("expected next_attempt_at scheduled")
	}
	if stored.RevokeReason != 1 {
		t.Fatalf("reason = %d, want 1", stored.RevokeReason)
	}
}

func TestRevokeSuccessSetsRevokedAtNoAutoRollback(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	prev := store.CertificateVersion{ID: "cver_prev", FullchainPEM: "PREV", PrivateKeyPEM: "PREV-KEY"}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})
	var gotReason *uint
	acme := &fakeIssueACME{
		revokeFn: func(_ context.Context, _ store.CertificateIssuer, _ []byte, reason *uint) error {
			gotReason = reason
			return nil
		},
	}
	m := NewManagerWithACME(st, nil, acme)
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	m.SetReplicaID("test-replica")

	op, err := m.SubmitRevokeVersion(context.Background(), certID, activeID, 4)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.runOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion.RevokedAt == nil {
		t.Fatal("expected revoked_at set")
	}
	if cert.ActiveVersion.RevokePending {
		t.Fatal("expected revoke_pending cleared")
	}
	if cert.ActiveVersion.ID != activeID {
		t.Fatal("active should not auto-rollback to previous")
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != "cver_prev" {
		t.Fatal("previous should remain unchanged")
	}
	if gotReason == nil || *gotReason != 4 {
		t.Fatalf("reason = %v, want 4", gotReason)
	}
	if st.ops[op.ID].Status != store.CertOpStatusSucceeded {
		t.Fatalf("op status = %q", st.ops[op.ID].Status)
	}
}

func TestRevokeActiveDoesNotActivatePrevious(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	prev := store.CertificateVersion{ID: "cver_prev", FullchainPEM: "PREV", PrivateKeyPEM: "PREV-KEY"}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})
	m := NewManagerWithACME(st, nil, &fakeIssueACME{})
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	m.SetReplicaID("test-replica")

	op, _ := m.SubmitRevokeVersion(context.Background(), certID, activeID, 0)
	m.runOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion.ID != activeID {
		t.Fatal("previous was auto-activated")
	}
}

func TestSubmitRevokeIdempotentWhenRunning(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	m := NewManagerWithACME(st, nil, &fakeIssueACME{})

	first, err := m.SubmitRevokeVersion(context.Background(), certID, activeID, 0)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := m.SubmitRevokeVersion(context.Background(), certID, activeID, 0)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same op, got %q and %q", first.ID, second.ID)
	}
}

func TestRevokePreviousVersion(t *testing.T) {
	st := newManagedCertFakeStore()
	activeID := "cver_active"
	certID := seedRevokeTestCert(t, st, activeID)
	prevID := "cver_prev"
	prevChain, _ := generateTestBundle(t, []string{"prev.example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{
		ID:                  prevID,
		FullchainPEM:        string(prevChain),
		PrivateKeyPEM:       "PREV-KEY",
		CertificateIssuerID: "iss_1",
		NotBefore:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:            time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})
	m := NewManagerWithACME(st, nil, &fakeIssueACME{})
	m.SetReplicaID("test-replica")

	op, err := m.SubmitRevokeVersion(context.Background(), certID, prevID, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.runOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.PreviousVersion.RevokedAt == nil {
		t.Fatal("expected previous revoked")
	}
	if cert.ActiveVersion.RevokedAt != nil {
		t.Fatal("active should remain valid")
	}
	_, err = m.ActivatePreviousVersion(context.Background(), certID, prevID)
	if !errors.Is(err, store.ErrVersionRevoked) {
		t.Fatalf("activate revoked previous: %v", err)
	}
	_ = op
}
