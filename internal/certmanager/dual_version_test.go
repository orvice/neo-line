package certmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func seedActiveCert(st *managedCertFakeStore, domains []string, keyType string) (certID string, activeID string) {
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	activeID = "cver_active"
	certID = "mcert_1"
	snap := store.IssueConfigSnapshot{
		Domains:              domains,
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              keyType,
	}
	st.certs[certID] = store.ManagedCertificate{
		ID:                   certID,
		Name:                 "prod",
		Domains:              domains,
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              keyType,
		AutoRenewEnabled:     true,
		RenewBeforeDays:      30,
		ActiveVersion: &store.CertificateVersion{
			ID:             activeID,
			ConfigSnapshot: snap,
			FullchainPEM:   "ACTIVE-CHAIN",
			PrivateKeyPEM:  "ACTIVE-KEY",
			NotBefore:      notBefore,
			NotAfter:       notAfter,
			KeyType:        keyType,
			CreatedAt:      notBefore,
		},
	}
	st.certOrd = []string{certID}
	return certID, activeID
}

func TestUpdateDesiredDoesNotPublish(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, activeID := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}

	got, err := m.UpdateManagedCertificate(context.Background(), certID, ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"new.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.HasUnpublishedDesiredChanges {
		t.Fatal("expected unpublished desired changes")
	}
	cert := st.certs[certID]
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID != activeID {
		t.Fatalf("active changed unexpectedly: %v", cert.ActiveVersion)
	}
	if cert.Domains[0] != "new.example.com" {
		t.Fatalf("desired domains = %v", cert.Domains)
	}
}

func setFakeCert(st *managedCertFakeStore, certID string, fn func(*store.ManagedCertificate)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cert := st.certs[certID]
	fn(&cert)
	st.certs[certID] = cert
}

func TestIssueFailureKeepsActiveAndPrevious(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{ID: "cver_prev", FullchainPEM: "PREV", PrivateKeyPEM: "PREV-KEY"}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
		c.Domains = []string{"changed.example.com"}
	})

	op := store.CertificateOperation{
		ID:                   "cop_2",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot: store.IssueConfigSnapshot{
			Domains:              []string{"changed.example.com"},
			CertificateIssuerID:  "iss_1",
			DNSProviderAccountID: "dns_1",
			KeyType:              store.CertKeyTypeECP256,
		},
	}
	st.ops[op.ID] = op
	st.opOrd = append(st.opOrd, op.ID)

	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	m := newIssueTestManager(t, st, failIssueACME("dns failure"), dns)
	m.runIssueOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID != "cver_active" {
		t.Fatalf("active = %v", cert.ActiveVersion)
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != "cver_prev" {
		t.Fatalf("previous = %v", cert.PreviousVersion)
	}
}

func TestIssueSuccessSwapsVersions(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, activeID := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.Domains = []string{"new.example.com"}
	})

	op := store.CertificateOperation{
		ID:                   "cop_2",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot: store.IssueConfigSnapshot{
			Domains:              []string{"new.example.com"},
			CertificateIssuerID:  "iss_1",
			DNSProviderAccountID: "dns_1",
			KeyType:              store.CertKeyTypeECP256,
		},
	}
	st.ops[op.ID] = op
	st.opOrd = append(st.opOrd, op.ID)

	dns := NewFakeDNSProvider(&fakeDNSZone{name: "new.example.com"})
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.runIssueOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID == activeID {
		t.Fatalf("expected new active, got %v", cert.ActiveVersion)
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != activeID {
		t.Fatalf("expected old active as previous, got %v", cert.PreviousVersion)
	}
	if cert.Domains[0] != "new.example.com" {
		t.Fatal("desired config should remain edited value")
	}
}

func TestThirdVersionDropsOldestPrevious(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, firstActive := seedActiveCert(st, []string{"v1.example.com"}, store.CertKeyTypeECP256)
	secondActive := store.CertificateVersion{
		ID:             "cver_v2",
		ConfigSnapshot: st.certs[certID].ActiveVersion.ConfigSnapshot,
		FullchainPEM:   "V2",
		PrivateKeyPEM:  "V2-KEY",
		NotBefore:      st.certs[certID].ActiveVersion.NotBefore,
		NotAfter:       st.certs[certID].ActiveVersion.NotAfter,
		KeyType:        store.CertKeyTypeECP256,
		CreatedAt:      time.Now().UTC(),
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = c.ActiveVersion
		c.ActiveVersion = &secondActive
	})

	op := store.CertificateOperation{
		ID:                   "cop_3",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot: store.IssueConfigSnapshot{
			Domains:              []string{"v3.example.com"},
			CertificateIssuerID:  "iss_1",
			DNSProviderAccountID: "dns_1",
			KeyType:              store.CertKeyTypeECP256,
		},
	}
	st.ops[op.ID] = op
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "v3.example.com"})
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.runIssueOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != "cver_v2" {
		t.Fatalf("previous should be v2, got %v", cert.PreviousVersion)
	}
	if cert.PreviousVersion.ID == firstActive {
		t.Fatal("oldest version PEM should be dropped")
	}
	if cert.PreviousVersion.FullchainPEM != "V2" {
		t.Fatalf("previous pem = %q", cert.PreviousVersion.FullchainPEM)
	}
}

func TestActivatePreviousRollsBackDesiredUnchanged(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"desired.example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{
		ID:             "cver_prev",
		ConfigSnapshot: store.IssueConfigSnapshot{Domains: []string{"old.example.com"}},
		FullchainPEM:   "PREV",
		PrivateKeyPEM:  "PREV-KEY",
		NotBefore:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		KeyType:        store.CertKeyTypeECP256,
		CreatedAt:      time.Now().UTC(),
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
		c.Domains = []string{"desired.example.com", "extra.example.com"}
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}
	got, err := m.ActivatePreviousVersion(context.Background(), certID, "cver_prev")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if got.ActiveVersion == nil || got.ActiveVersion.ID != "cver_prev" {
		t.Fatalf("active = %v", got.ActiveVersion)
	}
	if got.Domains[0] != "desired.example.com" {
		t.Fatalf("desired changed: %v", got.Domains)
	}
	if st.certs[certID].PreviousVersion == nil || st.certs[certID].PreviousVersion.ID != "cver_active" {
		t.Fatalf("previous should be old active")
	}
}

func TestActivateExpiredPreviousAllowed(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{
		ID:             "cver_expired",
		ConfigSnapshot: store.IssueConfigSnapshot{Domains: []string{"old.example.com"}},
		FullchainPEM:   "EXPIRED",
		PrivateKeyPEM:  "EXPIRED-KEY",
		NotBefore:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:       time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		KeyType:        store.CertKeyTypeECP256,
		CreatedAt:      time.Now().UTC(),
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}
	_, err := m.ActivatePreviousVersion(context.Background(), certID, "cver_expired")
	if err != nil {
		t.Fatalf("activate expired previous: %v", err)
	}
}

func TestActivateRevokedPreviousRejected(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	revokedAt := time.Now().UTC()
	prev := store.CertificateVersion{
		ID:             "cver_revoked",
		ConfigSnapshot: store.IssueConfigSnapshot{Domains: []string{"old.example.com"}},
		FullchainPEM:   "REVOKED",
		PrivateKeyPEM:  "REVOKED-KEY",
		NotBefore:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyType:        store.CertKeyTypeECP256,
		CreatedAt:      time.Now().UTC(),
		RevokedAt:      &revokedAt,
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})

	m := NewManager(st, nil)
	_, err := m.ActivatePreviousVersion(context.Background(), certID, "cver_revoked")
	if !errors.Is(err, store.ErrVersionRevoked) {
		t.Fatalf("expected ErrVersionRevoked, got %v", err)
	}
}

func TestDownloadPreviousBundle(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{
		ID:             "cver_prev",
		ConfigSnapshot: store.IssueConfigSnapshot{Domains: []string{"old.example.com"}},
		FullchainPEM:   "PREV-CHAIN",
		PrivateKeyPEM:  "PREV-KEY",
		NotBefore:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyType:        store.CertKeyTypeECP256,
		CreatedAt:      time.Now().UTC(),
	}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	bundle, err := m.GetCertificateBundle(context.Background(), certID, VersionSlotPrevious)
	if err != nil {
		t.Fatalf("download previous: %v", err)
	}
	if bundle.VersionID != "cver_prev" || string(bundle.FullchainPEM) != "PREV-CHAIN" {
		t.Fatalf("bundle = %+v", bundle)
	}
}

func TestRenewalDueValidity(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.RenewBeforeDays = 30
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}
	got, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveValidity != store.CertValidityRenewalDue {
		t.Fatalf("validity = %q, want RenewalDue", got.ActiveValidity)
	}
}
