package certmanager

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/certnotify"
	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

func TestIssueFailureNotifiesImmediately(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	certID, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	st.notify["ntf_1"] = store.NotifyGroup{
		ID: "ntf_1",
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook"},
		},
	}
	st.certs[certID] = store.ManagedCertificate{
		ID:                   certID,
		Name:                 "test",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              store.CertKeyTypeECP256,
		NotifyGroupIDs:       []string{"ntf_1"},
	}

	rec := &notify.RecordingSender{}
	notifier := certnotify.NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{"webhook": rec}, nil), nil)
	m := newIssueTestManager(t, st, failIssueACME("acme order rejected"), dns)
	m.SetCertNotifier(notifier)

	m.runIssueOperation(context.Background(), opID)
	notifier.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1 failure notification", len(rec.Sent))
	}
	var payload struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(rec.Sent[0].WebhookJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.EventType != certnotify.EventOperationFailed {
		t.Fatalf("event = %q", payload.EventType)
	}
	if st.ops[opID].Status != store.CertOpStatusPending {
		t.Fatalf("op status = %q, notification must not change operation", st.ops[opID].Status)
	}
}

func TestIssueSuccessRecoveryAfterFailure(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	certID, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	st.notify["ntf_1"] = store.NotifyGroup{
		ID: "ntf_1",
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook"},
		},
	}
	st.certs[certID] = store.ManagedCertificate{
		ID:                   certID,
		Name:                 "test",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              store.CertKeyTypeECP256,
		NotifyGroupIDs:       []string{"ntf_1"},
	}

	rec := &notify.RecordingSender{}
	notifier := certnotify.NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{"webhook": rec}, nil), nil)
	failACME := failIssueACME("fail once")
	m := newIssueTestManager(t, st, failACME, dns)
	m.SetCertNotifier(notifier)

	m.runIssueOperation(context.Background(), opID)
	notifier.Wait()

	// Allow retry claim after scheduled backoff.
	st.ops[opID] = store.CertificateOperation{
		ID:                   opID,
		ManagedCertificateID: certID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		ConfigSnapshot:       st.ops[opID].ConfigSnapshot,
	}
	m.acme = successIssueACME(t, store.CertKeyTypeECP256, false)
	m.runIssueOperation(context.Background(), opID)
	notifier.Wait()

	if len(rec.Sent) != 2 {
		t.Fatalf("sent = %d, want fail + recovery", len(rec.Sent))
	}
}

func TestReconcilerSevenDayNotification(t *testing.T) {
	st := newManagedCertFakeStore()
	certID, _ := seedActiveCert(st, []string{"example.com"}, store.CertKeyTypeECP256)
	notAfter := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	setFakeCert(st, certID, func(c *store.ManagedCertificate) {
		c.NotifyGroupIDs = []string{"ntf_1"}
		c.AutoRenewEnabled = false
		c.ActiveVersion.NotAfter = notAfter
	})
	st.notify["ntf_1"] = store.NotifyGroup{
		ID: "ntf_1",
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook"},
		},
	}

	rec := &notify.RecordingSender{}
	notifier := certnotify.NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{"webhook": rec}, nil), nil)
	clk := fixedClock{t: notAfter.Add(-6 * 24 * time.Hour)}
	notifier.SetClock(clk)

	m := NewManager(st, nil)
	m.clock = clk
	m.SetCertNotifier(notifier)
	NewReconciler(m).Reconcile(context.Background())
	notifier.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1 seven-day reminder", len(rec.Sent))
	}
}
