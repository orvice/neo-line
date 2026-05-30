package store

import (
	"testing"
)

func TestApplyMonitorDefaults(t *testing.T) {
	t.Run("tcp defaults", func(t *testing.T) {
		monitor := Monitor{}

		applyMonitorDefaults(&monitor)

		if monitor.Kind != "tcp" {
			t.Fatalf("Kind = %q, want tcp", monitor.Kind)
		}
		if monitor.IntervalSeconds != 60 {
			t.Fatalf("IntervalSeconds = %d, want 60", monitor.IntervalSeconds)
		}
		if monitor.TimeoutSeconds != 5 {
			t.Fatalf("TimeoutSeconds = %d, want 5", monitor.TimeoutSeconds)
		}
		if monitor.Retries != 3 {
			t.Fatalf("Retries = %d, want 3", monitor.Retries)
		}
		if monitor.Status != StatusUnknown {
			t.Fatalf("Status = %q, want %q", monitor.Status, StatusUnknown)
		}
	})

	t.Run("url defaults", func(t *testing.T) {
		monitor := Monitor{Kind: "url"}

		applyMonitorDefaults(&monitor)

		if monitor.Method != "GET" {
			t.Fatalf("Method = %q, want GET", monitor.Method)
		}
		if got := monitor.ExpectedStatusCodes; got != "200" {
			t.Fatalf("ExpectedStatusCodes = %q, want %q", got, "200")
		}
	})

	t.Run("tls port defaults", func(t *testing.T) {
		monitor := Monitor{Kind: "tls_port"}

		applyMonitorDefaults(&monitor)

		if monitor.Port != 443 {
			t.Fatalf("Port = %d, want 443", monitor.Port)
		}
		if monitor.WarningDays != 30 {
			t.Fatalf("WarningDays = %d, want 30", monitor.WarningDays)
		}
		if monitor.CriticalDays != 7 {
			t.Fatalf("CriticalDays = %d, want 7", monitor.CriticalDays)
		}
	})

	t.Run("preserves configured values", func(t *testing.T) {
		monitor := Monitor{
			Kind:                "url",
			Method:              "POST",
			ExpectedStatusCodes: "201, 202",
			IntervalSeconds:     10,
			TimeoutSeconds:      2,
			Retries:             1,
			Status:              StatusHealthy,
		}

		applyMonitorDefaults(&monitor)

		if monitor.Method != "POST" {
			t.Fatalf("Method = %q, want POST", monitor.Method)
		}
		if got := monitor.ExpectedStatusCodes; got != "201, 202" {
			t.Fatalf("ExpectedStatusCodes = %q, want %q", got, "201, 202")
		}
		if monitor.IntervalSeconds != 10 || monitor.TimeoutSeconds != 2 || monitor.Retries != 1 || monitor.Status != StatusHealthy {
			t.Fatalf("defaults overwrote configured values: %#v", monitor)
		}
	})
}

func TestAggregateHealth(t *testing.T) {
	tests := []struct {
		name   string
		health ServerHealth
		want   string
	}{
		{name: "no monitors", health: ServerHealth{}, want: StatusUnknown},
		{name: "down wins", health: ServerHealth{TotalMonitors: 3, HealthyMonitors: 2, DownMonitors: 1}, want: StatusDown},
		{name: "critical before warning", health: ServerHealth{TotalMonitors: 3, HealthyMonitors: 1, WarningMonitors: 1, CriticalMonitors: 1}, want: StatusCritical},
		{name: "warning before unknown", health: ServerHealth{TotalMonitors: 2, WarningMonitors: 1, UnknownMonitors: 1}, want: StatusWarning},
		{name: "unknown before healthy", health: ServerHealth{TotalMonitors: 2, HealthyMonitors: 1, UnknownMonitors: 1}, want: StatusUnknown},
		{name: "all healthy", health: ServerHealth{TotalMonitors: 2, HealthyMonitors: 2}, want: StatusHealthy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregateHealth(tt.health); got != tt.want {
				t.Fatalf("aggregateHealth() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := map[string]string{
		StatusHealthy:            StatusHealthy,
		"healthy":                StatusHealthy,
		"HEALTH_STATUS_HEALTHY":  StatusHealthy,
		"WARNING":                StatusWarning,
		"critical":               StatusCritical,
		"HEALTH_STATUS_CRITICAL": StatusCritical,
		"down":                   StatusDown,
		"unexpected":             StatusUnknown,
		"":                       StatusUnknown,
	}

	for input, want := range tests {
		if got := normalizeStatus(input); got != want {
			t.Fatalf("normalizeStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeLimit(t *testing.T) {
	tests := []struct {
		input int64
		want  int64
	}{
		{input: -1, want: 50},
		{input: 0, want: 50},
		{input: 1, want: 1},
		{input: 200, want: 200},
		{input: 201, want: 200},
	}

	for _, tt := range tests {
		if got := normalizeLimit(tt.input); got != tt.want {
			t.Fatalf("normalizeLimit(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParsePageToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    int64
		wantErr bool
	}{
		{name: "empty", token: "", want: 0},
		{name: "valid", token: "25", want: 25},
		{name: "negative", token: "-1", wantErr: true},
		{name: "not number", token: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePageToken(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parsePageToken() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePageToken() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parsePageToken() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  Admin@Example.COM  "); got != "admin@example.com" {
		t.Fatalf("normalizeEmail() = %q, want admin@example.com", got)
	}
}
