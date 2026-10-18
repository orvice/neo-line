package certmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestDeleteManagedCertificateRequiresZeroServers(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.ServerIDs = []string{"srv_1"}
	})
	m := NewManager(st, nil)

	err := m.DeleteManagedCertificate(context.Background(), certID)
	if !errors.Is(err, store.ErrManagedCertificateHasServerAssignments) {
		t.Fatalf("expected server assignment error, got %v", err)
	}
}

func TestDeleteManagedCertificateRequiresNoRunningOps(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	st.ops["cop_run"] = store.CertificateOperation{
		ID:                   "cop_run",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusRunning,
	}
	st.opOrd = append(st.opOrd, "cop_run")
	m := NewManager(st, nil)

	err := m.DeleteManagedCertificate(context.Background(), certID)
	if !errors.Is(err, store.ErrManagedCertificateOperationInFlight) {
		t.Fatalf("expected running op error, got %v", err)
	}
}

func TestDeleteManagedCertificateCascadesLocalState(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	st.ops["cop_done"] = store.CertificateOperation{
		ID:                   "cop_done",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusSucceeded,
	}
	st.opOrd = []string{"cop_done"}
	m := NewManager(st, nil)

	if err := m.DeleteManagedCertificate(context.Background(), certID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := st.certs[certID]; ok {
		t.Fatal("certificate should be deleted")
	}
	if _, ok := st.ops["cop_done"]; ok {
		t.Fatal("operations should be cascaded")
	}
}

func TestDeleteIssuerBlockedWhenReferenced(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "uses-issuer",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = m.DeleteCertificateIssuer(context.Background(), "iss_1")
	if !errors.Is(err, store.ErrCertificateResourceReferenced) {
		t.Fatalf("expected reference error, got %v", err)
	}
}

func TestDeleteIssuerAllowedWhenUnreferenced(t *testing.T) {
	st := newManagedCertFakeStore()
	count, err := st.CountManagedCertificatesReferencingIssuer(context.Background(), "iss_orphan")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d", count)
	}
}

func TestDeleteDNSBlockedWhenReferenced(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "uses-dns",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = m.DeleteDNSProviderAccount(context.Background(), "dns_1")
	if !errors.Is(err, store.ErrCertificateResourceReferenced) {
		t.Fatalf("expected reference error, got %v", err)
	}
}

func TestAutoRenewDisabledManualRenewStillWorks(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.AutoRenewEnabled = false
	})
	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}

	r := NewReconciler(m)
	r.Reconcile(context.Background())
	if len(st.ops) > 0 {
		for _, op := range st.ops {
			if op.Type == store.CertOpTypeRenew {
				t.Fatal("reconciler should not enqueue renew when auto_renew disabled")
			}
		}
	}

	op, err := m.SubmitRenewOperation(context.Background(), certID)
	if err != nil {
		t.Fatalf("manual renew: %v", err)
	}
	if op.Type != store.CertOpTypeRenew {
		t.Fatalf("op type = %q", op.Type)
	}
	pub, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.BundleAvailable {
		t.Fatal("active bundle should remain available when auto_renew disabled")
	}
}
