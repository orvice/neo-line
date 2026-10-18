package alert

// Tests in this file exercise the full dispatch flow through the module's
// interface (OnMonitorStatusChange): policy evaluation, throttling, notify
// group resolution and channel fan-out. HTTP never happens — a recording
// sender sits at the notify seam instead of the built-in adapters.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

type fakeAlertStore struct {
	groups       map[string]store.MonitorGroup
	notifyGroups map[string]store.NotifyGroup
}

func (f *fakeAlertStore) GetMonitorGroup(_ context.Context, id string) (store.MonitorGroup, error) {
	g, ok := f.groups[id]
	if !ok {
		return store.MonitorGroup{}, errors.New("group not found")
	}
	return g, nil
}

func (f *fakeAlertStore) GetNotifyGroup(_ context.Context, id string) (store.NotifyGroup, error) {
	ng, ok := f.notifyGroups[id]
	if !ok {
		return store.NotifyGroup{}, errors.New("notify group not found")
	}
	return ng, nil
}

// newFlowDispatcher wires a dispatcher whose only channel adapter is a
// recording sender registered for the webhook kind (the default for an empty
// channel type).
func newFlowDispatcher(st Store) (*Dispatcher, *notify.RecordingSender) {
	rec := &notify.RecordingSender{}
	d := New(st, nil)
	d.notifier = notify.NewWithSenders(map[string]notify.Sender{"webhook": rec}, nil)
	return d, rec
}

func flowStore(policy store.AlertPolicy) *fakeAlertStore {
	return &fakeAlertStore{
		groups: map[string]store.MonitorGroup{
			"grp1": {ID: "grp1", Name: "prod", AlertPolicy: policy},
		},
		notifyGroups: map[string]store.NotifyGroup{
			"ng1": {ID: "ng1", Channels: []store.AlertChannel{{Type: "webhook", Target: "http://example.test/hook"}}},
		},
	}
}

func flowMonitor() store.Monitor {
	return store.Monitor{ID: "mon1", Name: "api-health", ServerID: "srv1", GroupIDs: []string{"grp1"}}
}

func decodeMonitorPayload(t *testing.T, d notify.Delivery) Payload {
	t.Helper()
	var p Payload
	if err := json.Unmarshal(d.WebhookJSON, &p); err != nil {
		t.Fatalf("unmarshal webhook JSON: %v", err)
	}
	return p
}

func TestDispatchFlowDeliversOnPolicyMatch(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)
	occurred := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "healthy", "down", occurred)
	d.deliveries.Wait()

	got := rec.Deliveries()
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(got))
	}
	p := decodeMonitorPayload(t, got[0])
	if p.PreviousStatus != "Healthy" || p.CurrentStatus != "Down" {
		t.Errorf("statuses not normalized: %q -> %q", p.PreviousStatus, p.CurrentStatus)
	}
	if p.MonitorID != "mon1" || p.ServerID != "srv1" || p.GroupID != "grp1" || p.GroupName != "prod" {
		t.Errorf("payload identity fields wrong: %+v", p)
	}
	if !p.OccurredAt.Equal(occurred) {
		t.Errorf("occurred_at = %v, want %v", p.OccurredAt, occurred)
	}
	if !containsAll(got[0].HumanText, "api-health", "Healthy → Down", "Group: prod", "Server: srv1") {
		t.Errorf("human text missing expected fields: %q", got[0].HumanText)
	}
}

func TestDispatchFlowRespectsPolicyFlags(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: false, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 when OnDown is disabled", n)
	}
}

func TestDispatchFlowSkipsDisabledPolicy(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: false, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 when policy is disabled", n)
	}
}

func TestDispatchFlowSkipsUnchangedStatus(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "down", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 for unchanged status", n)
	}
}

func TestDispatchFlowThrottlesAndRecoveryResets(t *testing.T) {
	policy := store.AlertPolicy{
		Enabled:            true,
		OnDown:             true,
		OnRecover:          true,
		MinIntervalSeconds: 300,
		NotifyGroupIDs:     []string{"ng1"},
	}
	st := flowStore(policy)
	d, rec := newFlowDispatcher(st)
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	d.OnMonitorStatusChange(ctx, flowMonitor(), "Healthy", "Down", base)
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Critical", "Down", base.Add(time.Minute))
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Down", "Healthy", base.Add(2*time.Minute))
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Healthy", "Down", base.Add(3*time.Minute))
	d.deliveries.Wait()

	deliveries := rec.Deliveries()
	if len(deliveries) != 3 {
		t.Fatalf("deliveries = %d, want 3 (down, recovery, down)", len(deliveries))
	}
	wantCurr := map[string]int{"Down": 2, "Healthy": 1}
	gotCurr := map[string]int{}
	for _, d := range deliveries {
		p := decodeMonitorPayload(t, d)
		gotCurr[p.CurrentStatus]++
	}
	for status, want := range wantCurr {
		if gotCurr[status] != want {
			t.Errorf("deliveries with current=%s: got %d, want %d", status, gotCurr[status], want)
		}
	}
}

func TestDispatchFlowSurvivesGroupLoadFailure(t *testing.T) {
	st := &fakeAlertStore{}
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 when group load fails", n)
	}
}

func TestDispatchFlowFlattensNotifyGroupsAndSkipsMissing(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng-missing", "ng1", "ng2"}})
	st.notifyGroups["ng2"] = store.NotifyGroup{ID: "ng2", Channels: []store.AlertChannel{
		{Type: "", Target: "http://example.test/a"},
		{Type: "webhook", Target: "http://example.test/b"},
	}}
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 3 {
		t.Fatalf("deliveries = %d, want 3", n)
	}
}

func TestDispatchFlowIgnoresUnsupportedChannelType(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	st.notifyGroups["ng1"] = store.NotifyGroup{ID: "ng1", Channels: []store.AlertChannel{
		{Type: "sms", Target: "+8613800000000"},
		{Type: "webhook", Target: "http://example.test/hook"},
	}}
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.Deliveries()); n != 1 {
		t.Fatalf("deliveries = %d, want 1 (unsupported type dropped, webhook delivered)", n)
	}
}

func TestDispatchFlowFansOutToMultipleGroups(t *testing.T) {
	policy := store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}}
	st := flowStore(policy)
	st.groups["grp2"] = store.MonitorGroup{ID: "grp2", Name: "staging", AlertPolicy: policy}
	d, rec := newFlowDispatcher(st)

	monitor := flowMonitor()
	monitor.GroupIDs = []string{"grp1", "grp2"}
	d.OnMonitorStatusChange(context.Background(), monitor, "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	deliveries := rec.Deliveries()
	if len(deliveries) != 2 {
		t.Fatalf("deliveries = %d, want 2 (one per group)", len(deliveries))
	}
	names := map[string]bool{}
	for _, d := range deliveries {
		p := decodeMonitorPayload(t, d)
		names[p.GroupName] = true
	}
	if !names["prod"] || !names["staging"] {
		t.Errorf("expected deliveries for both groups, got %v", names)
	}
}

func TestDispatchFlowSurvivesChannelFailure(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d := New(st, nil)
	d.notifier = notify.NewWithSenders(map[string]notify.Sender{
		"webhook": notify.SenderFunc(func(_ store.AlertChannel, _ notify.Delivery) error {
			return errors.New("network down")
		}),
	}, nil)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()
}

type failingThenOKSender struct {
	calls int
}

func (f *failingThenOKSender) Send(_ store.AlertChannel, _ notify.Delivery) error {
	f.calls++
	if f.calls == 1 {
		return errors.New("transient failure")
	}
	return nil
}

func TestDispatchFlowDoesNotBlockOnChannelFailure(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	st.notifyGroups["ng1"] = store.NotifyGroup{ID: "ng1", Channels: []store.AlertChannel{
		{Type: "webhook", Target: "http://example.test/a"},
		{Type: "webhook", Target: "http://example.test/b"},
	}}
	fail := &failingThenOKSender{}
	d := New(st, nil)
	d.notifier = notify.NewWithSenders(map[string]notify.Sender{"webhook": fail}, nil)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if fail.calls != 2 {
		t.Fatalf("sender calls = %d, want 2 (both channels attempted despite first failure)", fail.calls)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
