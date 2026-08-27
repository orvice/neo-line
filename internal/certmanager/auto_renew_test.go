package certmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestEffectiveRenewalWindow90DayCert(t *testing.T) {
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(90 * 24 * time.Hour)
	got := effectiveRenewalWindowDays(notBefore, notAfter, 30)
	if got != 30 {
		t.Fatalf("effective days = %d, want 30", got)
	}
}

func TestEffectiveRenewalWindowShortCertThird(t *testing.T) {
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(30 * 24 * time.Hour)
	got := effectiveRenewalWindowDays(notBefore, notAfter, 30)
	if got != 10 {
		t.Fatalf("effective days = %d, want 10 (lifetime/3)", got)
	}
}

func TestRenewalDueBoundaryExactWindowStart(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(90 * 24 * time.Hour)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.RenewBeforeDays = 30
		c.ActiveVersion.NotBefore = notBefore
		c.ActiveVersion.NotAfter = notAfter
	})

	m := NewManager(st, nil)
	windowStart := notAfter.Add(-30 * 24 * time.Hour)
	m.clock = fixedClock{t: windowStart}
	got, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveValidity != store.CertValidityRenewalDue {
		t.Fatalf("validity = %q, want RenewalDue at window start", got.ActiveValidity)
	}
}

func TestRenewalDueBeforeWindowStillValid(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(90 * 24 * time.Hour)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.ActiveVersion.NotBefore = notBefore
		c.ActiveVersion.NotAfter = notAfter
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: notAfter.Add(-31 * 24 * time.Hour)}
	got, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveValidity != store.CertValidityValid {
		t.Fatalf("validity = %q, want Valid", got.ActiveValidity)
	}
}

func TestAutoRenewDisabledReconcilerSkips(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.AutoRenewEnabled = false
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)}
	NewReconciler(m).Reconcile(context.Background())

	if len(st.ops) != 0 {
		t.Fatalf("expected no renew op when auto-renew disabled, got %d ops", len(st.ops))
	}
}

func TestAutoRenewUsesActiveSnapshotNotDesired(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"active.example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.Domains = []string{"desired.example.com"}
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)}
	op, err := m.SubmitRenewOperation(context.Background(), certID)
	if err != nil {
		t.Fatalf("submit renew: %v", err)
	}
	if op.ConfigSnapshot.Domains[0] != "active.example.com" {
		t.Fatalf("renew snapshot domains = %v, want active snapshot", op.ConfigSnapshot.Domains)
	}
}

func TestManualRenewWhenAutoRenewOff(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.AutoRenewEnabled = false
	})

	m := NewManager(st, nil)
	op, err := m.SubmitRenewOperation(context.Background(), certID)
	if err != nil {
		t.Fatalf("manual renew: %v", err)
	}
	if op.Type != store.CertOpTypeRenew {
		t.Fatalf("op type = %q", op.Type)
	}
}

func TestSubmitRenewOperationIdempotent(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	m := NewManager(st, nil)

	first, err := m.SubmitRenewOperation(context.Background(), certID)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := m.SubmitRenewOperation(context.Background(), certID)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same op, got %q and %q", first.ID, second.ID)
	}
	if len(st.ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(st.ops))
	}
}

func TestRenewFailureKeepsActiveAndPrevious(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, activeID := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	prev := store.CertificateVersion{ID: "cver_prev", FullchainPEM: "PREV", PrivateKeyPEM: "PREV-KEY"}
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.PreviousVersion = &prev
		c.Domains = []string{"changed.example.com"}
	})

	op := store.CertificateOperation{
		ID:                   "cop_renew",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot:       st.certs[certID].ActiveVersion.ConfigSnapshot,
	}
	st.ops[op.ID] = op
	st.opOrd = append(st.opOrd, op.ID)

	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	m := newIssueTestManager(t, st, failIssueACME("renew failure"), dns)
	m.runRenewOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID != activeID {
		t.Fatalf("active changed: %v", cert.ActiveVersion)
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != "cver_prev" {
		t.Fatalf("previous changed: %v", cert.PreviousVersion)
	}
}

func TestRenewSuccessSwapsVersionsWithNewKey(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, activeID := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	oldFP := st.certs[certID].ActiveVersion.LeafFingerprint
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.Domains = []string{"desired.example.com"}
	})

	op := store.CertificateOperation{
		ID:                   "cop_renew",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot:       st.certs[certID].ActiveVersion.ConfigSnapshot,
	}
	st.ops[op.ID] = op

	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.runRenewOperation(context.Background(), op.ID)

	cert := st.certs[certID]
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID == activeID {
		t.Fatal("expected new active after renew")
	}
	if cert.ActiveVersion.LeafFingerprint == oldFP {
		t.Fatal("expected new private key / fingerprint after renew")
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != activeID {
		t.Fatalf("expected old active as previous, got %v", cert.PreviousVersion)
	}
	if cert.Domains[0] != "desired.example.com" {
		t.Fatal("desired config must remain unchanged after renew")
	}
	if opSnap := st.ops["cop_renew"].ConfigSnapshot.Domains[0]; opSnap != "example.com" {
		t.Fatalf("renew op used desired domains: %q", opSnap)
	}
}

func TestReconcilerEnqueuesRenewOnRenewalDue(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.AutoRenewEnabled = true
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}
	NewReconciler(m).Reconcile(context.Background())

	if len(st.ops) != 1 {
		t.Fatalf("expected 1 renew op from reconciler, got %d", len(st.ops))
	}
	for _, op := range st.ops {
		if op.Type != store.CertOpTypeRenew {
			t.Fatalf("op type = %q, want Renew", op.Type)
		}
	}
}

func TestReconcilerStopsAfterCancel(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}
	ctx, cancel := context.WithCancel(context.Background())
	reconciler := NewReconciler(m)
	reconciler.interval = 10 * time.Millisecond

	done := make(chan struct{})
	go func() {
		reconciler.Start(ctx)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop after cancel")
	}

	before := len(st.ops)
	time.Sleep(30 * time.Millisecond)
	if len(st.ops) != before {
		t.Fatalf("reconciler picked up work after stop: ops %d -> %d", before, len(st.ops))
	}
}

func TestSubmitRenewRejectedWhenIssueRunning(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	st.ops["cop_issue"] = store.CertificateOperation{
		ID:                   "cop_issue",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusRunning,
	}
	st.opOrd = append(st.opOrd, "cop_issue")

	m := NewManager(st, nil)
	_, err := m.SubmitRenewOperation(context.Background(), certID)
	if !errors.Is(err, ErrIssuanceOperationInFlight) {
		t.Fatalf("expected ErrIssuanceOperationInFlight, got %v", err)
	}
}

func TestUpdateBlocksIssueFieldsWhileRenewRunning(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	st.ops["cop_renew"] = store.CertificateOperation{
		ID:                   "cop_renew",
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusRunning,
	}
	st.opOrd = append(st.opOrd, "cop_renew")

	m := NewManager(st, nil)
	_, err := m.UpdateManagedCertificate(context.Background(), certID, ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"other.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if !errors.Is(err, ErrIssueFieldsLocked) {
		t.Fatalf("expected ErrIssueFieldsLocked, got %v", err)
	}
}

func TestNextRenewalAtMetadata(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(90 * 24 * time.Hour)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.RenewBeforeDays = 30
		c.ActiveVersion.NotBefore = notBefore
		c.ActiveVersion.NotAfter = notAfter
	})

	m := NewManager(st, nil)
	m.clock = fixedClock{t: notBefore.Add(24 * time.Hour)}
	got, err := m.GetManagedCertificate(context.Background(), certID)
	if err != nil {
		t.Fatal(err)
	}
	wantNext := notAfter.Add(-30 * 24 * time.Hour)
	if got.NextRenewalAt == nil || !got.NextRenewalAt.Equal(wantNext) {
		t.Fatalf("next_renewal_at = %v, want %v", got.NextRenewalAt, wantNext)
	}
	if got.EffectiveRenewalWindowDays != 30 {
		t.Fatalf("effective window = %d, want 30", got.EffectiveRenewalWindowDays)
	}
}
