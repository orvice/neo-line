package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// fakeStore is a non-Mongo store.Store used to prove the scheduler depends only
// on the interface. The embedded store.Store leaves unused methods unimplemented
// (they panic if ever called), so the test documents exactly what the scheduler
// touches: ListEnabledMonitors to load work and SaveCheckResult to persist it.
type fakeStore struct {
	store.Store
	monitors []store.Monitor
	saved    chan store.CheckResult
}

func (f *fakeStore) ListEnabledMonitors(context.Context) ([]store.Monitor, error) {
	return f.monitors, nil
}

func (f *fakeStore) SaveCheckResult(_ context.Context, result store.CheckResult) (string, error) {
	f.saved <- result
	return "", nil
}

func TestReconcileDispatchesDueMonitors(t *testing.T) {
	fake := &fakeStore{
		monitors: []store.Monitor{{
			ID:              "mon_1",
			ServerID:        "srv_1",
			Kind:            "tcp",
			Host:            "127.0.0.1",
			Port:            1, // closed port: connection refused, fast and local
			IntervalSeconds: 1,
			TimeoutSeconds:  1,
			Retries:         0,
		}},
		saved: make(chan store.CheckResult, 1),
	}

	s := New(fake)
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	s.reconcile(context.Background())

	select {
	case result := <-fake.saved:
		if result.MonitorID != "mon_1" {
			t.Fatalf("SaveCheckResult MonitorID = %q, want mon_1", result.MonitorID)
		}
		if result.Status != store.StatusDown {
			t.Fatalf("Status = %q, want %q", result.Status, store.StatusDown)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected scheduler to dispatch the due monitor and call SaveCheckResult")
	}
}
