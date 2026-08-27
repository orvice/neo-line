package certmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/certnotify"
	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

// TestCertificateLifecycleSmoke exercises the full managed-certificate lifecycle
// through the certmanager public seam with deterministic Store/ACME/DNS/notifier/clock
// fakes. No real CA, Cloudflare, S3, or external notification endpoints are used.
func TestCertificateLifecycleSmoke(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		RegistrationStatus: store.IssuerRegistrationReady,
	}
	st.seedDNS("dns_1")
	st.dns["dns_1"] = store.DNSProviderAccount{ID: "dns_1", APIToken: "cf-token"}
	st.servers["srv_1"] = store.Server{ID: "srv_1"}
	st.notify["ntf_1"] = store.NotifyGroup{
		ID: "ntf_1",
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook"},
		},
	}

	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"}, &fakeDNSZone{name: "v2.example.com"})
	acme := successIssueACME(t, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, acme, dns)
	m.clock = fixedClock{t: base}

	rec := &notify.RecordingSender{}
	notifier := certnotify.NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{
		"webhook": rec,
	}, nil), nil)
	m.SetCertNotifier(notifier)

	// --- Prerequisites: create ManagedCertificate → auto Pending Issue ---
	created, err := m.CreateManagedCertificate(ctx, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
	})
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certID := created.ID
	if len(st.ops) != 1 {
		t.Fatalf("expected 1 issue op on create, got %d", len(st.ops))
	}
	var firstOpID string
	for id := range st.ops {
		firstOpID = id
	}

	// --- First issue → active version ---
	m.runIssueOperation(ctx, firstOpID)
	pub, err := m.GetManagedCertificate(ctx, certID)
	if err != nil {
		t.Fatal(err)
	}
	if pub.ActiveValidity != store.CertValidityValid || pub.ActiveVersion == nil {
		t.Fatalf("after first issue: validity=%q active=%v", pub.ActiveValidity, pub.ActiveVersion)
	}
	firstActiveID := pub.ActiveVersion.ID

	// --- Desired publish + dual version switch ---
	if _, err := m.UpdateManagedCertificate(ctx, certID, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"v2.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
	}); err != nil {
		t.Fatalf("update desired: %v", err)
	}
	issueOp, err := m.SubmitIssueOperation(ctx, certID)
	if err != nil {
		t.Fatalf("submit issue: %v", err)
	}
	m.runIssueOperation(ctx, issueOp.ID)
	pub, _ = m.GetManagedCertificate(ctx, certID)
	if pub.ActiveVersion == nil || pub.ActiveVersion.ID == firstActiveID {
		t.Fatal("expected new active after desired publish")
	}
	if pub.PreviousVersion == nil || pub.PreviousVersion.ID != firstActiveID {
		t.Fatalf("expected previous = first active, got %v", pub.PreviousVersion)
	}
	secondActiveID := pub.ActiveVersion.ID

	// --- Server assignment + distribution ---
	if _, err := m.UpdateManagedCertificate(ctx, certID, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"v2.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
		ServerIDs:            []string{"srv_1"},
	}); err != nil {
		t.Fatalf("assign server: %v", err)
	}
	list, err := m.ListServerCertificates(ctx, "srv_1")
	if err != nil || len(list) != 1 || !list[0].Available {
		t.Fatalf("list authorized: err=%v list=%+v", err, list)
	}
	bundle, err := m.GetServerCertificateBundle(ctx, "srv_1", certID)
	if err != nil || bundle.VersionID != secondActiveID {
		t.Fatalf("bundle: err=%v version=%q", err, bundle.VersionID)
	}

	// --- Unassign blocks download immediately ---
	if _, err := m.UpdateManagedCertificate(ctx, certID, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"v2.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
		ServerIDs:            []string{},
	}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if _, err := m.GetServerCertificateBundle(ctx, "srv_1", certID); err != ErrCertificateNotAuthorized {
		t.Fatalf("unassigned download err = %v", err)
	}

	// Re-assign for remaining phases.
	if _, err := m.UpdateManagedCertificate(ctx, certID, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"v2.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
		ServerIDs:            []string{"srv_1"},
	}); err != nil {
		t.Fatalf("re-assign: %v", err)
	}

	// --- Expired active still downloadable ---
	expiredNow := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.ActiveVersion.NotBefore = expiredNow.Add(-400 * 24 * time.Hour)
		c.ActiveVersion.NotAfter = expiredNow.Add(-24 * time.Hour)
	})
	m.clock = fixedClock{t: expiredNow}
	expBundle, err := m.GetServerCertificateBundle(ctx, "srv_1", certID)
	if err != nil || expBundle.Validity != store.CertValidityExpired {
		t.Fatalf("expired bundle: err=%v validity=%q", err, expBundle.Validity)
	}

	// --- Auto-renew uses active snapshot (not unpublished desired) ---
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.Domains = []string{"desired-unpublished.example.com"}
		c.ActiveVersion.NotBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		c.ActiveVersion.NotAfter = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		c.AutoRenewEnabled = true
	})
	renewClock := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	m.clock = fixedClock{t: renewClock}
	cert, err := st.GetManagedCertificate(ctx, certID)
	if err != nil {
		t.Fatal(err)
	}
	renewOp, err := m.createPendingRenewOperation(ctx, cert)
	if err != nil {
		t.Fatalf("create renew op: %v", err)
	}
	if renewOp.ConfigSnapshot.Domains[0] != "v2.example.com" {
		t.Fatalf("renew used desired domains %v, want active snapshot", renewOp.ConfigSnapshot.Domains)
	}

	// --- Operation retry after failure; notification failure does not block ---
	m.acme = failIssueACME("transient acme error")
	m.runRenewOperation(ctx, renewOp.ID)
	if st.ops[renewOp.ID].Status != store.CertOpStatusPending {
		t.Fatalf("renew retry status = %q", st.ops[renewOp.ID].Status)
	}
	sentBeforeRecovery := len(rec.Sent)

	m.acme = successIssueACME(t, store.CertKeyTypeECP256, false)
	st.ops[renewOp.ID] = store.CertificateOperation{
		ID:                   renewOp.ID,
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot:       renewOp.ConfigSnapshot,
	}
	m.runRenewOperation(ctx, renewOp.ID)
	notifier.Wait()
	if st.ops[renewOp.ID].Status != store.CertOpStatusSucceeded {
		t.Fatalf("renew success status = %q", st.ops[renewOp.ID].Status)
	}
	if len(rec.Sent) <= sentBeforeRecovery {
		t.Fatal("expected recovery notification after renew success")
	}

	// --- Previous rollback ---
	rollbackActiveID := st.certs[certID].ActiveVersion.ID
	prevID := st.certs[certID].PreviousVersion.ID
	m.clock = fixedClock{t: renewClock}
	if _, err := m.ActivatePreviousVersion(ctx, certID, prevID); err != nil {
		t.Fatalf("activate previous: %v", err)
	}
	if st.certs[certID].ActiveVersion.ID != prevID {
		t.Fatalf("rollback active = %q", st.certs[certID].ActiveVersion.ID)
	}
	if st.certs[certID].PreviousVersion.ID != rollbackActiveID {
		t.Fatalf("rollback previous = %q", st.certs[certID].PreviousVersion.ID)
	}

	// --- Revoke blocks distribution immediately ---
	revokeTarget := st.certs[certID].ActiveVersion.ID
	m.acme = &fakeIssueACME{
		revokeFn: func(context.Context, store.CertificateIssuer, []byte, *uint) error {
			return nil
		},
	}
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		RegistrationStatus: store.IssuerRegistrationReady,
		AccountKeyPEM:      testAccountKeyPEM,
	}
	revokeOp, err := m.SubmitRevokeVersion(ctx, certID, revokeTarget, 0)
	if err != nil {
		t.Fatalf("submit revoke: %v", err)
	}
	if !st.certs[certID].ActiveVersion.RevokePending {
		t.Fatal("expected immediate revoke_pending")
	}
	if _, err := m.GetServerCertificateBundle(ctx, "srv_1", certID); err != ErrBundleNotAvailable {
		t.Fatalf("revoke pending download err = %v", err)
	}
	m.runOperation(ctx, revokeOp.ID)
	if st.certs[certID].ActiveVersion.RevokedAt == nil {
		t.Fatal("expected revoked_at after CA success")
	}

	// --- Delete constraints + local cascade ---
	if err := m.DeleteManagedCertificate(ctx, certID); !errors.Is(err, store.ErrManagedCertificateHasServerAssignments) {
		t.Fatalf("delete with servers: %v", err)
	}
	if _, err := m.UpdateManagedCertificate(ctx, certID, ManagedCertificateInput{
		Name:                 "lifecycle-smoke",
		Domains:              []string{"v2.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            []string{},
	}); err != nil {
		t.Fatalf("clear servers: %v", err)
	}
	if err := m.DeleteManagedCertificate(ctx, certID); err != nil {
		t.Fatalf("delete cascade: %v", err)
	}
	if _, ok := st.certs[certID]; ok {
		t.Fatal("certificate should be deleted locally")
	}
	for id := range st.ops {
		if st.ops[id].ManagedCertificateID == certID {
			t.Fatalf("operation %s should be cascaded", id)
		}
	}
}
