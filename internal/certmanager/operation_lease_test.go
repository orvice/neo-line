package certmanager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestComputeRetryDelayBounds(t *testing.T) {
	jitter := func(base time.Duration) time.Duration { return time.Minute }
	first := computeRetryDelay(1, jitter)
	if first != 16*time.Minute {
		t.Fatalf("first delay = %v, want 16m", first)
	}
	capped := computeRetryDelay(20, jitter)
	if capped != 12*time.Hour+time.Minute {
		t.Fatalf("capped delay = %v, want 12h1m", capped)
	}
}

func TestDualReplicaClaimOnlyOneWins(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	acme := successIssueACME(t, store.CertKeyTypeECP256, false)

	m1 := newIssueTestManager(t, st, acme, dns)
	m1.SetReplicaID("replica-a")
	m2 := newIssueTestManager(t, st, acme, dns)
	m2.SetReplicaID("replica-b")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m1.runOperation(context.Background(), opID) }()
	go func() { defer wg.Done(); m2.runOperation(context.Background(), opID) }()
	wg.Wait()

	op := st.ops[opID]
	if op.Status != store.CertOpStatusSucceeded {
		t.Fatalf("status = %q, want Succeeded", op.Status)
	}
	if op.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1", op.AttemptCount)
	}
}

func TestLostLeaseDoesNotCommitActive(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	op, err := st.TryClaimCertificateOperation(context.Background(), store.CertificateOperationClaimParams{
		OpID: opID, Owner: "replica-a", Now: now, LeaseExpires: now.Add(operationLeaseDuration),
	})
	if err != nil {
		t.Fatal(err)
	}

	takeoverAt := now.Add(operationLeaseDuration + time.Second)
	_, err = st.TryClaimCertificateOperation(context.Background(), store.CertificateOperationClaimParams{
		OpID: opID, Owner: "replica-b", Now: takeoverAt, LeaseExpires: takeoverAt.Add(operationLeaseDuration),
	})
	if err != nil {
		t.Fatalf("takeover claim: %v", err)
	}

	version := store.CertificateVersion{ID: "cver_lost"}
	err = st.ActivateFirstIssueVersion(context.Background(), certID, version, opID, op.LeaseOwner, "")
	if !errors.Is(err, store.ErrCertificateOperationConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if st.certs[certID].ActiveVersion != nil {
		t.Fatal("active version must not be set after lost lease")
	}
}

func TestAutoRetrySameOperationIncrementsAttempt(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, failIssueACME("acme order rejected"), dns)
	m.SetJitter(func(time.Duration) time.Duration { return 0 })
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	m.clock = fixedClock{t: base}

	m.runOperation(context.Background(), opID)
	op := st.ops[opID]
	if op.Status != store.CertOpStatusPending {
		t.Fatalf("status = %q", op.Status)
	}
	if op.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d", op.AttemptCount)
	}
	if op.NextAttemptAt == nil {
		t.Fatal("expected next_attempt_at")
	}

	m.clock = fixedClock{t: op.NextAttemptAt.Add(time.Second)}
	m.runOperation(context.Background(), opID)
	op = st.ops[opID]
	if op.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2", op.AttemptCount)
	}
}

func TestManualRetryAfterTerminalCreatesNewOperation(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	cert := store.ManagedCertificate{
		ID: "mcert_1", Name: "test", Domains: []string{"example.com"},
		CertificateIssuerID: "iss_1", DNSProviderAccountID: "dns_1", KeyType: store.CertKeyTypeECP256,
	}
	st.certs[cert.ID] = cert
	st.certOrd = []string{cert.ID}
	failed := store.CertificateOperation{
		ID: "cop_old", ManagedCertificateID: cert.ID, Type: store.CertOpTypeIssue,
		Status: store.CertOpStatusFailed, AttemptCount: 3, ErrorSummary: "gave up",
	}
	st.ops[failed.ID] = failed
	st.opOrd = []string{failed.ID}

	m := NewManager(st, nil)
	op, err := m.SubmitIssueOperation(context.Background(), cert.ID)
	if err != nil {
		t.Fatal(err)
	}
	if op.ID == failed.ID {
		t.Fatal("expected new operation after terminal failure")
	}
	if op.Status != store.CertOpStatusPending {
		t.Fatalf("new op status = %q", op.Status)
	}
}

func TestPollRespectsClaimingLeasesFlag(t *testing.T) {
	st := newManagedCertFakeStore()
	seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := NewManager(st, nil)
	m.SetClaimingLeases(false)
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	m.pollClaimableOperations(context.Background())
	for _, op := range st.ops {
		if op.AttemptCount > 0 {
			t.Fatal("poll should not claim when accepting leases is false")
		}
	}
}

func TestTakeoverCleansPendingTXT(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	exp := now.Add(-time.Second)
	st.mu.Lock()
	op := st.ops[opID]
	op.Status = store.CertOpStatusRunning
	op.AttemptCount = 1
	op.LeaseOwner = "dead-replica"
	op.LeaseExpiresAt = &exp
	op.PendingTXTRecords = []store.DNSChallengeRecord{{Domain: "example.com", Token: "tok", KeyAuth: "auth"}}
	st.ops[opID] = op
	st.mu.Unlock()

	m := newIssueTestManager(t, st, successIssueACME(t, store.CertKeyTypeECP256, false), dns)
	m.SetReplicaID("replica-b")
	m.clock = fixedClock{t: now}
	m.runOperation(context.Background(), opID)
	zone := dns.zones["example.com"]
	if zone == nil {
		t.Fatal("missing zone")
	}
	if len(zone.txt) != 0 {
		t.Fatalf("expected cleanup before retry, txt left: %v", zone.txt)
	}
	if st.ops[opID].Status != store.CertOpStatusSucceeded {
		t.Fatalf("status = %q", st.ops[opID].Status)
	}
}
