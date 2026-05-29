package store

import (
	"testing"
	"time"
)

func TestComputeUptime(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// Results are newest-first, as returned by the store query.
	results := []CheckResult{
		{Status: StatusHealthy, StartedAt: now.Add(-1 * time.Minute), DurationMS: 40},
		{Status: StatusDown, StartedAt: now.Add(-2 * time.Minute), DurationMS: 5000},
		{Status: StatusWarning, StartedAt: now.Add(-30 * time.Minute), DurationMS: 60},
		{Status: StatusHealthy, StartedAt: now.Add(-90 * time.Minute), DurationMS: 50}, // outside 1h
		{Status: StatusDown, StartedAt: now.Add(-23 * time.Hour), DurationMS: 5000},
		{Status: StatusHealthy, StartedAt: now.Add(-26 * time.Hour), DurationMS: 50}, // outside 24h, filtered upstream but ignored here
	}

	got := computeUptime(results, now, 50)

	w1h, ok := got.Windows["1h"]
	if !ok {
		t.Fatal("missing 1h window")
	}
	if w1h.Total != 3 || w1h.Up != 2 || w1h.Down != 1 {
		t.Fatalf("1h window counts = %+v, want total 3 up 2 down 1", w1h)
	}
	if w1h.Uptime < 0.66 || w1h.Uptime > 0.67 {
		t.Fatalf("1h uptime = %v, want ~0.6667", w1h.Uptime)
	}
	if w1h.AvgLatencyMS != 50 { // (40+60)/2
		t.Fatalf("1h avg latency = %v, want 50", w1h.AvgLatencyMS)
	}

	w24h := got.Windows["24h"]
	if w24h.Total != 5 || w24h.Up != 3 || w24h.Down != 2 {
		t.Fatalf("24h window counts = %+v, want total 5 up 3 down 2", w24h)
	}

	// Heartbeats must be oldest-first for left-to-right rendering.
	if len(got.Heartbeats) != len(results) {
		t.Fatalf("heartbeats len = %d, want %d", len(got.Heartbeats), len(results))
	}
	for i := 1; i < len(got.Heartbeats); i++ {
		if got.Heartbeats[i].StartedAt.Before(got.Heartbeats[i-1].StartedAt) {
			t.Fatalf("heartbeats not sorted oldest-first at index %d", i)
		}
	}
}

func TestComputeUptimeEmpty(t *testing.T) {
	now := time.Now().UTC()
	got := computeUptime(nil, now, 50)
	for key, w := range got.Windows {
		if w.Total != 0 || w.Uptime != 0 || w.AvgLatencyMS != 0 {
			t.Fatalf("window %s not zeroed: %+v", key, w)
		}
	}
	if len(got.Heartbeats) != 0 {
		t.Fatalf("heartbeats = %d, want 0", len(got.Heartbeats))
	}
}

func TestComputeUptimeBeatLimit(t *testing.T) {
	now := time.Now().UTC()
	results := make([]CheckResult, 120)
	for i := range results {
		results[i] = CheckResult{Status: StatusHealthy, StartedAt: now.Add(-time.Duration(i) * time.Minute), DurationMS: 10}
	}
	got := computeUptime(results, now, 50)
	if len(got.Heartbeats) != 50 {
		t.Fatalf("heartbeats = %d, want 50", len(got.Heartbeats))
	}
}
