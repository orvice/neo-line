package store

import (
	"testing"
	"time"
)

func TestBuildWindow(t *testing.T) {
	// 3 checks: 2 up (latency 40 + 60), 1 down.
	win := buildWindow(3600, windowAccumulator{Total: 3, Down: 1, LatencySum: 100})
	if win.WindowSeconds != 3600 {
		t.Fatalf("window seconds = %d, want 3600", win.WindowSeconds)
	}
	if win.Total != 3 || win.Up != 2 || win.Down != 1 {
		t.Fatalf("counts = %+v, want total 3 up 2 down 1", win)
	}
	if win.Uptime < 0.66 || win.Uptime > 0.67 {
		t.Fatalf("uptime = %v, want ~0.6667", win.Uptime)
	}
	if win.AvgLatencyMS != 50 { // 100 / 2
		t.Fatalf("avg latency = %v, want 50", win.AvgLatencyMS)
	}
}

func TestBuildWindowEmpty(t *testing.T) {
	win := buildWindow(60, windowAccumulator{})
	if win.Total != 0 || win.Up != 0 || win.Down != 0 || win.Uptime != 0 || win.AvgLatencyMS != 0 {
		t.Fatalf("empty window not zeroed: %+v", win)
	}
	if win.WindowSeconds != 60 {
		t.Fatalf("window seconds = %d, want 60", win.WindowSeconds)
	}
}

func TestBuildWindowAllDown(t *testing.T) {
	win := buildWindow(60, windowAccumulator{Total: 4, Down: 4, LatencySum: 0})
	if win.Up != 0 || win.Uptime != 0 {
		t.Fatalf("all-down window = %+v, want up 0 uptime 0", win)
	}
	if win.AvgLatencyMS != 0 {
		t.Fatalf("avg latency = %v, want 0 when no up checks", win.AvgLatencyMS)
	}
}

func TestToHeartbeats(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// Newest-first, as returned by the store query.
	results := []CheckResult{
		{Status: StatusHealthy, StartedAt: now.Add(-1 * time.Minute), DurationMS: 40},
		{Status: "DOWN", StartedAt: now.Add(-2 * time.Minute), DurationMS: 5000},
		{Status: StatusWarning, StartedAt: now.Add(-3 * time.Minute), DurationMS: 60},
	}
	beats := toHeartbeats(results)
	if len(beats) != 3 {
		t.Fatalf("heartbeats len = %d, want 3", len(beats))
	}
	// Oldest-first for left-to-right rendering.
	for i := 1; i < len(beats); i++ {
		if beats[i].StartedAt.Before(beats[i-1].StartedAt) {
			t.Fatalf("heartbeats not oldest-first at index %d", i)
		}
	}
	// Status is normalized.
	if beats[1].Status != StatusDown {
		t.Fatalf("status[1] = %q, want %q (normalized)", beats[1].Status, StatusDown)
	}
}

func TestToHeartbeatsEmpty(t *testing.T) {
	if beats := toHeartbeats(nil); len(beats) != 0 {
		t.Fatalf("heartbeats = %d, want 0", len(beats))
	}
}
