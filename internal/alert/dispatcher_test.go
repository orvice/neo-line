package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestShouldFire(t *testing.T) {
	tests := []struct {
		name   string
		policy store.AlertPolicy
		prev   string
		curr   string
		want   bool
	}{
		{
			name:   "down enabled, transitions to down",
			policy: store.AlertPolicy{OnDown: true},
			prev:   "Healthy",
			curr:   "Down",
			want:   true,
		},
		{
			name:   "down disabled",
			policy: store.AlertPolicy{OnDown: false},
			prev:   "Healthy",
			curr:   "Down",
			want:   false,
		},
		{
			name:   "recover from down",
			policy: store.AlertPolicy{OnRecover: true},
			prev:   "Down",
			curr:   "Healthy",
			want:   true,
		},
		{
			name:   "recover ignored when empty prev (first probe)",
			policy: store.AlertPolicy{OnRecover: true},
			prev:   "",
			curr:   "Healthy",
			want:   false,
		},
		{
			name:   "recover ignored when prev is Unknown (first probe of new monitor)",
			policy: store.AlertPolicy{OnRecover: true},
			prev:   "Unknown",
			curr:   "Healthy",
			want:   false,
		},
		{
			name:   "recover not triggered when staying healthy",
			policy: store.AlertPolicy{OnRecover: true},
			prev:   "Healthy",
			curr:   "Healthy",
			want:   false,
		},
		{
			name:   "warning enabled",
			policy: store.AlertPolicy{OnWarning: true},
			prev:   "Healthy",
			curr:   "Warning",
			want:   true,
		},
		{
			name:   "critical enabled",
			policy: store.AlertPolicy{OnCritical: true},
			prev:   "Warning",
			curr:   "Critical",
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldFire(tt.policy, tt.prev, tt.curr); got != tt.want {
				t.Fatalf("shouldFire() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowThrottle(t *testing.T) {
	d := New(nil, nil)
	now := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)

	if !d.allowThrottle("grp1", "mon1", 60, now) {
		t.Fatal("first call should be allowed")
	}
	if d.allowThrottle("grp1", "mon1", 60, now.Add(30*time.Second)) {
		t.Fatal("second call within window should be throttled")
	}
	if !d.allowThrottle("grp1", "mon1", 60, now.Add(2*time.Minute)) {
		t.Fatal("call after window should be allowed")
	}
	if !d.allowThrottle("grp2", "mon1", 60, now.Add(30*time.Second)) {
		t.Fatal("different group should not be throttled by other group")
	}
	if !d.allowThrottle("grp1", "mon1", 0, now.Add(1*time.Second)) {
		t.Fatal("zero interval disables throttling")
	}
}

func TestResetThrottleClearsWindow(t *testing.T) {
	d := New(nil, nil)
	now := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)

	if !d.allowThrottle("grp1", "mon1", 300, now) {
		t.Fatal("first call should be allowed")
	}
	if d.allowThrottle("grp1", "mon1", 300, now.Add(time.Minute)) {
		t.Fatal("second call within window should be throttled")
	}
	d.resetThrottle("grp1", "mon1")
	if !d.allowThrottle("grp1", "mon1", 300, now.Add(2*time.Minute)) {
		t.Fatal("call after reset should be allowed")
	}
}

func samplePayload() Payload {
	return Payload{
		MonitorID:      "mon1",
		MonitorName:    "api-health",
		ServerID:       "srv1",
		PreviousStatus: "Healthy",
		CurrentStatus:  "Down",
		OccurredAt:     time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC),
		GroupID:        "grp1",
		GroupName:      "prod",
	}
}

func TestFormatMessage(t *testing.T) {
	msg := formatMessage(samplePayload())
	for _, want := range []string{"api-health", "Healthy → Down", "Group: prod", "Server: srv1", "2026-05-30T10:00:00Z"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("formatMessage() missing %q in:\n%s", want, msg)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"Healthy":                "Healthy",
		"healthy":                "Healthy",
		"HEALTH_STATUS_HEALTHY":  "Healthy",
		"WARNING":                "Warning",
		"critical":               "Critical",
		"HEALTH_STATUS_CRITICAL": "Critical",
		"down":                   "Down",
		"":                       "",
		"banana":                 "Unknown",
	}
	for input, want := range tests {
		if got := normalize(input); got != want {
			t.Fatalf("normalize(%q) = %q, want %q", input, got, want)
		}
	}
}
