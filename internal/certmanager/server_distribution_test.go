package certmanager

import (
	"context"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func seedServerCert(st *managedCertFakeStore, cert store.ManagedCertificate) {
	st.certs[cert.ID] = cert
	st.certOrd = append(st.certOrd, cert.ID)
}

func TestListServerCertificatesActiveMetadata(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	notAfter := now.Add(90 * 24 * time.Hour)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		Name:      "web",
		Domains:   []string{"desired.example.com"},
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID: "cver_1",
			ConfigSnapshot: store.IssueConfigSnapshot{
				Domains: []string{"active.example.com"},
			},
			KeyType:         store.CertKeyTypeECP256,
			LeafFingerprint: "fp-active",
			NotBefore:       now.Add(-24 * time.Hour),
			NotAfter:        notAfter,
		},
	})
	m := NewManager(st, nil)
	m.clock = fixedClock{t: now}

	list, err := m.ListServerCertificates(context.Background(), "srv_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	got := list[0]
	if got.Name != "" {
		t.Fatalf("name should be hidden when active exists, got %q", got.Name)
	}
	if got.ActiveVersionID != "cver_1" || !got.Available {
		t.Fatalf("active metadata = %+v", got)
	}
	if len(got.Domains) != 1 || got.Domains[0] != "active.example.com" {
		t.Fatalf("domains = %v", got.Domains)
	}
	if got.Validity != store.CertValidityValid {
		t.Fatalf("validity = %q", got.Validity)
	}
}

func TestListServerCertificatesMissingActive(t *testing.T) {
	st := newManagedCertFakeStore()
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		Name:      "pending",
		Domains:   []string{"pending.example.com"},
		ServerIDs: []string{"srv_1"},
	})
	st.ops["op_1"] = store.CertificateOperation{
		ID:                   "op_1",
		ManagedCertificateID: "mcert_1",
		Status:               store.CertOpStatusFailed,
		ErrorSummary:         "dns validation failed",
	}
	st.opOrd = append(st.opOrd, "op_1")
	m := NewManager(st, nil)

	list, err := m.ListServerCertificates(context.Background(), "srv_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Available || list[0].Validity != store.CertValidityMissing {
		t.Fatalf("entry = %+v", list[0])
	}
	if list[0].Name != "pending" || list[0].ErrorSummary != "dns validation failed" {
		t.Fatalf("missing metadata = %+v", list[0])
	}
}

func TestGetServerCertificateBundleIsolationAndEnumeration(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			ConfigSnapshot: store.IssueConfigSnapshot{
				Domains: []string{"a.example.com"},
			},
			KeyType:   store.CertKeyTypeECP256,
			NotBefore: now,
			NotAfter:  now.Add(24 * time.Hour),
		},
	})
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_other",
		ServerIDs: []string{"srv_2"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_2",
			FullchainPEM:  "chain2",
			PrivateKeyPEM: "key2",
			NotBefore:     now,
			NotAfter:      now.Add(24 * time.Hour),
		},
	})
	m := NewManager(st, nil)
	m.clock = fixedClock{t: now}

	if _, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "missing"); err != ErrCertificateNotAuthorized {
		t.Fatalf("missing cert err = %v", err)
	}
	if _, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_other"); err != ErrCertificateNotAuthorized {
		t.Fatalf("other server cert err = %v", err)
	}
	bundle, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_1")
	if err != nil {
		t.Fatal(err)
	}
	if string(bundle.FullchainPEM) != "chain" || string(bundle.PrivateKeyPEM) != "key" {
		t.Fatalf("bundle bytes mismatch")
	}
	if bundle.VersionID != "cver_1" {
		t.Fatalf("version = %q", bundle.VersionID)
	}
}

func TestGetServerCertificateBundleUnassignImmediate(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			NotBefore:     now,
			NotAfter:      now.Add(24 * time.Hour),
		},
	})
	m := NewManager(st, nil)
	if _, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_1"); err != ErrCertificateNotAuthorized {
		t.Fatalf("unassigned err = %v", err)
	}
}

func TestGetServerCertificateBundleExpiredStillDownloadable(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			NotBefore:     now.Add(-48 * time.Hour),
			NotAfter:      now.Add(-1 * time.Hour),
		},
	})
	m := NewManager(st, nil)
	m.clock = fixedClock{t: now}
	bundle, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_1")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Validity != store.CertValidityExpired {
		t.Fatalf("validity = %q", bundle.Validity)
	}
}

func TestGetServerCertificateBundleRevokeBlocks(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	revoked := now.Add(-time.Hour)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			NotBefore:     now.Add(-24 * time.Hour),
			NotAfter:      now.Add(24 * time.Hour),
			RevokedAt:     &revoked,
		},
	})
	m := NewManager(st, nil)
	if _, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_1"); err != ErrBundleNotAvailable {
		t.Fatalf("revoked err = %v", err)
	}
}

func TestGetServerCertificateBundleRevokePendingBlocks(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			NotBefore:     now,
			NotAfter:      now.Add(24 * time.Hour),
			RevokePending: true,
		},
	})
	m := NewManager(st, nil)
	if _, err := m.GetServerCertificateBundle(context.Background(), "srv_1", "mcert_1"); err != ErrBundleNotAvailable {
		t.Fatalf("pending revoke err = %v", err)
	}
}

func TestListServerCertificatesStagingFlag(t *testing.T) {
	st := newManagedCertFakeStore()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedServerCert(st, store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:               "cver_1",
			StagingUntrusted: true,
			ConfigSnapshot:   store.IssueConfigSnapshot{Domains: []string{"staging.example.com"}},
			NotBefore:        now,
			NotAfter:         now.Add(24 * time.Hour),
		},
	})
	m := NewManager(st, nil)
	m.clock = fixedClock{t: now}
	list, err := m.ListServerCertificates(context.Background(), "srv_1")
	if err != nil {
		t.Fatal(err)
	}
	if !list[0].StagingUntrusted {
		t.Fatal("expected staging flag")
	}
}
