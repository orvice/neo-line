package certnotify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

type fakeCertNotifyStore struct {
	mu     sync.Mutex
	certs  map[string]store.ManagedCertificate
	notify map[string]store.NotifyGroup
	ops    map[string]store.CertificateOperation
}

func newFakeCertNotifyStore() *fakeCertNotifyStore {
	return &fakeCertNotifyStore{
		certs:  make(map[string]store.ManagedCertificate),
		notify: make(map[string]store.NotifyGroup),
		ops:    make(map[string]store.CertificateOperation),
	}
}

func (f *fakeCertNotifyStore) GetManagedCertificate(_ context.Context, id string) (store.ManagedCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.certs[id]
	if !ok {
		return store.ManagedCertificate{}, errors.New("not found")
	}
	return c, nil
}

func (f *fakeCertNotifyStore) GetNotifyGroup(_ context.Context, id string) (store.NotifyGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ng, ok := f.notify[id]
	if !ok {
		return store.NotifyGroup{}, errors.New("not found")
	}
	return ng, nil
}

func (f *fakeCertNotifyStore) ListManagedCertificatesForNotifications(_ context.Context) ([]store.ManagedCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.ManagedCertificate, 0)
	for _, c := range f.certs {
		if len(c.NotifyGroupIDs) > 0 && c.ActiveVersion != nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCertNotifyStore) FindRunningCertificateOperation(_ context.Context, managedCertificateID, opType string) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, op := range f.ops {
		if op.ManagedCertificateID != managedCertificateID || op.Type != opType {
			continue
		}
		if op.Status == store.CertOpStatusPending || op.Status == store.CertOpStatusRunning {
			return op, nil
		}
	}
	return store.CertificateOperation{}, errors.New("not found")
}

func (f *fakeCertNotifyStore) ensureState(cert *store.ManagedCertificate) {
	if cert.NotificationState == nil {
		cert.NotificationState = &store.CertificateNotificationState{}
	}
}

func (f *fakeCertNotifyStore) TryRecordOperationFailureNotification(_ context.Context, certID string, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return false, errors.New("not found")
	}
	f.ensureState(&cert)
	if cert.NotificationState.HadOperationFailure {
		return false, nil
	}
	cert.NotificationState.HadOperationFailure = true
	cert.NotificationState.LastFailNotifiedAt = &now
	f.certs[certID] = cert
	return true, nil
}

func (f *fakeCertNotifyStore) TryRecordOperationFailureReminder(_ context.Context, certID string, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return false, errors.New("not found")
	}
	f.ensureState(&cert)
	if !cert.NotificationState.HadOperationFailure || cert.NotificationState.LastFailNotifiedAt == nil {
		return false, nil
	}
	if now.Sub(*cert.NotificationState.LastFailNotifiedAt) < store.CertNotificationFailReminderInterval {
		return false, nil
	}
	cert.NotificationState.LastFailNotifiedAt = &now
	f.certs[certID] = cert
	return true, nil
}

func (f *fakeCertNotifyStore) TryRecordOperationRecovery(_ context.Context, certID string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return false, errors.New("not found")
	}
	if cert.NotificationState == nil || !cert.NotificationState.HadOperationFailure {
		return false, nil
	}
	cert.NotificationState.HadOperationFailure = false
	cert.NotificationState.LastFailNotifiedAt = nil
	f.certs[certID] = cert
	return true, nil
}

func (f *fakeCertNotifyStore) TryRecordSevenDayReminder(_ context.Context, certID, versionID string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return false, errors.New("not found")
	}
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID != versionID {
		return false, nil
	}
	f.ensureState(&cert)
	if cert.NotificationState.SevenDayReminderVersionID == versionID {
		return false, nil
	}
	cert.NotificationState.SevenDayReminderVersionID = versionID
	f.certs[certID] = cert
	return true, nil
}

func (f *fakeCertNotifyStore) TryRecordExpiredNotification(_ context.Context, certID, versionID string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return false, errors.New("not found")
	}
	if cert.ActiveVersion == nil || cert.ActiveVersion.ID != versionID {
		return false, nil
	}
	f.ensureState(&cert)
	if cert.NotificationState.ExpiredNotifiedVersionID == versionID {
		return false, nil
	}
	cert.NotificationState.ExpiredNotifiedVersionID = versionID
	f.certs[certID] = cert
	return true, nil
}

func (f *fakeCertNotifyStore) SetCertificateNotificationWarning(_ context.Context, certID, warning string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[certID]
	if !ok {
		return errors.New("not found")
	}
	f.ensureState(&cert)
	cert.NotificationState.LastNotificationWarning = warning
	cert.NotificationState.LastNotificationWarningAt = &at
	f.certs[certID] = cert
	return nil
}

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

func newTestDispatcher(st Store) (*Dispatcher, *notify.RecordingSender, *fixedClock) {
	rec := &notify.RecordingSender{}
	clk := &fixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	d := NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{"webhook": rec}, nil), nil)
	d.SetClock(clk)
	return d, rec, clk
}

func seedCert(st *fakeCertNotifyStore, notifyIDs ...string) store.ManagedCertificate {
	cert := store.ManagedCertificate{
		ID:              "mcert_1",
		Name:            "prod",
		Domains:         []string{"example.com"},
		NotifyGroupIDs:  append([]string(nil), notifyIDs...),
		RenewBeforeDays: 30,
	}
	st.certs[cert.ID] = cert
	return cert
}

func seedNotify(st *fakeCertNotifyStore, id string) {
	st.notify[id] = store.NotifyGroup{
		ID: id,
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook"},
		},
	}
}

func decodePayload(t *testing.T, d notify.Delivery) Payload {
	t.Helper()
	var p Payload
	if err := json.Unmarshal(d.WebhookJSON, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return p
}

func TestFirstOperationFailureImmediate(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, clk := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "dns failed")
	d.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(rec.Sent))
	}
	p := decodePayload(t, rec.Sent[0])
	if p.EventType != EventOperationFailed {
		t.Fatalf("event = %q", p.EventType)
	}
	if p.ManagedCertificateID != cert.ID {
		t.Fatalf("cert id = %q", p.ManagedCertificateID)
	}
	if p.OperationType != store.CertOpTypeIssue {
		t.Fatalf("op type = %q", p.OperationType)
	}
	_ = clk
}

func TestRepeatedFailureWithin24hSuppressed(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail 1")
	d.Wait()
	d.OnOperationFailure(context.Background(), cert, op, "fail 2")
	d.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1 immediate only", len(rec.Sent))
	}
}

func TestFailureReminderAfter24h(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, clk := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeRenew}

	d.OnOperationFailure(context.Background(), cert, op, "fail 1")
	d.Wait()
	clk.t = clk.t.Add(25 * time.Hour)
	d.OnOperationFailure(context.Background(), cert, op, "fail 2")
	d.Wait()

	if len(rec.Sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(rec.Sent))
	}
	if decodePayload(t, rec.Sent[1]).EventType != EventOperationFailedReminder {
		t.Fatalf("second event = %q", decodePayload(t, rec.Sent[1]).EventType)
	}
}

func TestRecoveryAfterFailure(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()
	d.OnOperationSuccess(context.Background(), cert, op)
	d.Wait()

	if len(rec.Sent) != 2 {
		t.Fatalf("sent = %d, want fail+recover", len(rec.Sent))
	}
	if decodePayload(t, rec.Sent[1]).EventType != EventOperationRecovered {
		t.Fatalf("recovery event = %q", decodePayload(t, rec.Sent[1]).EventType)
	}
}

func TestFirstSuccessNotRecovery(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationSuccess(context.Background(), cert, op)
	d.Wait()

	if len(rec.Sent) != 0 {
		t.Fatalf("sent = %d, want 0 for first success", len(rec.Sent))
	}
}

func TestRecoveryNotThrottledByFailReminder(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, clk := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()
	clk.t = clk.t.Add(1 * time.Hour)
	d.OnOperationSuccess(context.Background(), cert, op)
	d.Wait()

	if len(rec.Sent) != 2 {
		t.Fatalf("sent = %d, want fail+recover within 24h", len(rec.Sent))
	}
}

func TestSevenDayReminderOnce(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	notAfter := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	cert.ActiveVersion = &store.CertificateVersion{
		ID:        "cver_1",
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  notAfter,
	}
	st.certs[cert.ID] = cert
	d, rec, clk := newTestDispatcher(st)
	clk.t = notAfter.Add(-6 * 24 * time.Hour)

	d.ScanValidityNotifications(context.Background())
	d.Wait()
	d.ScanValidityNotifications(context.Background())
	d.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1 seven-day reminder", len(rec.Sent))
	}
	if decodePayload(t, rec.Sent[0]).EventType != EventExpiringSoon {
		t.Fatalf("event = %q", decodePayload(t, rec.Sent[0]).EventType)
	}
}

func TestSevenDayReminderSkippedWhenRenewInFlight(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	notAfter := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	cert.ActiveVersion = &store.CertificateVersion{
		ID:        "cver_1",
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  notAfter,
	}
	st.certs[cert.ID] = cert
	st.ops["cop_renew"] = store.CertificateOperation{
		ID:                   "cop_renew",
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusRunning,
	}
	d, rec, clk := newTestDispatcher(st)
	clk.t = notAfter.Add(-6 * 24 * time.Hour)

	d.ScanValidityNotifications(context.Background())
	d.Wait()

	if len(rec.Sent) != 0 {
		t.Fatalf("sent = %d, want 0 while renew in flight", len(rec.Sent))
	}
}

func TestSevenDayReminderSkippedWhenReplacingIssueInFlight(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	notAfter := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	cert.ActiveVersion = &store.CertificateVersion{
		ID:        "cver_1",
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  notAfter,
	}
	st.certs[cert.ID] = cert
	st.ops["cop_issue"] = store.CertificateOperation{
		ID:                   "cop_issue",
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
	}
	d, rec, clk := newTestDispatcher(st)
	clk.t = notAfter.Add(-6 * 24 * time.Hour)

	d.ScanValidityNotifications(context.Background())
	d.Wait()

	if len(rec.Sent) != 0 {
		t.Fatalf("sent = %d, want 0 while replacing issue in flight", len(rec.Sent))
	}
}

func TestExpiredNotificationOnce(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	notAfter := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	cert.ActiveVersion = &store.CertificateVersion{
		ID:        "cver_1",
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  notAfter,
	}
	st.certs[cert.ID] = cert
	d, rec, clk := newTestDispatcher(st)
	clk.t = notAfter.Add(time.Hour)

	d.ScanValidityNotifications(context.Background())
	d.Wait()
	d.ScanValidityNotifications(context.Background())
	d.Wait()

	if len(rec.Sent) != 1 {
		t.Fatalf("sent = %d, want 1 expired", len(rec.Sent))
	}
	if decodePayload(t, rec.Sent[0]).EventType != EventExpired {
		t.Fatalf("event = %q", decodePayload(t, rec.Sent[0]).EventType)
	}
}

func TestMultipleNotifyGroupsFanOut(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	seedNotify(st, "ntf_2")
	st.notify["ntf_2"] = store.NotifyGroup{
		ID: "ntf_2",
		Channels: []store.AlertChannel{
			{Type: "webhook", Target: "http://example.test/hook2"},
		},
	}
	cert := seedCert(st, "ntf_1", "ntf_2")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()

	if len(rec.Sent) != 2 {
		t.Fatalf("sent = %d, want 2 channels", len(rec.Sent))
	}
}

func TestDeliveryFailureRecordsWarning(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	failSender := notify.SenderFunc(func(store.AlertChannel, notify.Delivery) error {
		return errors.New("webhook down")
	})
	clk := &fixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	d := NewWithNotifier(st, notify.NewWithSenders(map[string]notify.Sender{"webhook": failSender}, nil), nil)
	d.SetClock(clk)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()

	got, _ := st.GetManagedCertificate(context.Background(), cert.ID)
	if got.NotificationState == nil || got.NotificationState.LastNotificationWarning == "" {
		t.Fatal("expected notification warning persisted")
	}
}

func TestPayloadDoesNotUseMonitorFields(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()

	raw := string(rec.Sent[0].WebhookJSON)
	for _, forbidden := range []string{"monitor_id", "previous_status", "current_status", "group_id", "health"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("webhook JSON must not contain %q", forbidden)
		}
	}
}

func TestPayloadUsesCertificateEventType(t *testing.T) {
	st := newFakeCertNotifyStore()
	seedNotify(st, "ntf_1")
	cert := seedCert(st, "ntf_1")
	d, rec, _ := newTestDispatcher(st)
	op := store.CertificateOperation{ID: "cop_1", Type: store.CertOpTypeIssue}

	d.OnOperationFailure(context.Background(), cert, op, "fail")
	d.Wait()

	p := decodePayload(t, rec.Sent[0])
	if p.EventType != EventOperationFailed {
		t.Fatalf("event_type = %q", p.EventType)
	}
	if p.CertificateName != "prod" {
		t.Fatalf("name = %q", p.CertificateName)
	}
}
