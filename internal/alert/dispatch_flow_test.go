package alert

// Tests in this file exercise the full dispatch flow through the module's
// interface (OnMonitorStatusChange): policy evaluation, throttling, notify
// group resolution and channel fan-out. HTTP never happens — a recording
// sender sits at the channel seam instead of the built-in adapters.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

// recordingSender is a channel adapter that captures every payload it is
// asked to deliver.
type recordingSender struct {
	mu   sync.Mutex
	sent []Payload
}

func (r *recordingSender) send(_ store.AlertChannel, payload Payload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, payload)
	return nil
}

func (r *recordingSender) payloads() []Payload {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Payload, len(r.sent))
	copy(out, r.sent)
	return out
}

// newFlowDispatcher wires a dispatcher whose only channel adapter is the
// recording sender, registered for the webhook kind (the default for an empty
// channel type).
func newFlowDispatcher(st Store) (*Dispatcher, *recordingSender) {
	d := New(st, nil)
	rec := &recordingSender{}
	d.senders = map[string]channelSender{"webhook": rec}
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

func TestDispatchFlowDeliversOnPolicyMatch(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)
	occurred := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "healthy", "down", occurred)
	d.deliveries.Wait()

	got := rec.payloads()
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(got))
	}
	p := got[0]
	if p.PreviousStatus != "Healthy" || p.CurrentStatus != "Down" {
		t.Errorf("statuses not normalized: %q -> %q", p.PreviousStatus, p.CurrentStatus)
	}
	if p.MonitorID != "mon1" || p.ServerID != "srv1" || p.GroupID != "grp1" || p.GroupName != "prod" {
		t.Errorf("payload identity fields wrong: %+v", p)
	}
	if !p.OccurredAt.Equal(occurred) {
		t.Errorf("occurred_at = %v, want %v", p.OccurredAt, occurred)
	}
}

func TestDispatchFlowRespectsPolicyFlags(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: false, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.payloads()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 when OnDown is disabled", n)
	}
}

func TestDispatchFlowSkipsDisabledPolicy(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: false, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.payloads()); n != 0 {
		t.Fatalf("deliveries = %d, want 0 when policy is disabled", n)
	}
}

func TestDispatchFlowSkipsUnchangedStatus(t *testing.T) {
	st := flowStore(store.AlertPolicy{Enabled: true, OnDown: true, NotifyGroupIDs: []string{"ng1"}})
	d, rec := newFlowDispatcher(st)

	// Different spellings of the same status are not a transition.
	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "down", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.payloads()); n != 0 {
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

	// First incident alerts.
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Healthy", "Down", base)
	// A second non-healthy transition inside the window is throttled.
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Critical", "Down", base.Add(time.Minute))
	// Recovery is never throttled and resets the window.
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Down", "Healthy", base.Add(2*time.Minute))
	// Next incident right after recovery alerts immediately despite the window.
	d.OnMonitorStatusChange(ctx, flowMonitor(), "Healthy", "Down", base.Add(3*time.Minute))
	d.deliveries.Wait()

	got := rec.payloads()
	if len(got) != 3 {
		t.Fatalf("deliveries = %d, want 3 (down, recovery, down)", len(got))
	}
	wantCurr := map[string]int{"Down": 2, "Healthy": 1}
	gotCurr := map[string]int{}
	for _, p := range got {
		gotCurr[p.CurrentStatus]++
	}
	for status, want := range wantCurr {
		if gotCurr[status] != want {
			t.Errorf("deliveries with current=%s: got %d, want %d", status, gotCurr[status], want)
		}
	}
}

func TestDispatchFlowSurvivesGroupLoadFailure(t *testing.T) {
	st := &fakeAlertStore{} // no groups at all
	d, rec := newFlowDispatcher(st)

	d.OnMonitorStatusChange(context.Background(), flowMonitor(), "Healthy", "Down", time.Now())
	d.deliveries.Wait()

	if n := len(rec.payloads()); n != 0 {
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

	// ng-missing is skipped; ng1 has 1 channel, ng2 has 2 (empty type falls
	// back to the webhook sender).
	if n := len(rec.payloads()); n != 3 {
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

	if n := len(rec.payloads()); n != 1 {
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

	got := rec.payloads()
	if len(got) != 2 {
		t.Fatalf("deliveries = %d, want 2 (one per group)", len(got))
	}
	names := map[string]bool{}
	for _, p := range got {
		names[p.GroupName] = true
	}
	if !names["prod"] || !names["staging"] {
		t.Errorf("expected deliveries for both groups, got %v", names)
	}
}
