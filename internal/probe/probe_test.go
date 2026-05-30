package probe

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestRunPopulatesCheckResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	monitor := store.Monitor{
		ID:             "mon_1",
		ServerID:       "srv_1",
		Kind:           "tcp",
		Host:           "127.0.0.1",
		Port:           1,
		Retries:        0,
		TimeoutSeconds: 1,
	}

	result := Run(ctx, monitor)

	if result.ServerID != monitor.ServerID {
		t.Fatalf("ServerID = %q, want %q", result.ServerID, monitor.ServerID)
	}
	if result.MonitorID != monitor.ID {
		t.Fatalf("MonitorID = %q, want %q", result.MonitorID, monitor.ID)
	}
	if result.Status != store.StatusDown {
		t.Fatalf("Status = %q, want %q", result.Status, store.StatusDown)
	}
	if result.StartedAt.IsZero() || result.EndedAt.IsZero() {
		t.Fatalf("StartedAt/EndedAt should be populated: %#v", result)
	}
	if result.EndedAt.Before(result.StartedAt) {
		t.Fatalf("EndedAt %v is before StartedAt %v", result.EndedAt, result.StartedAt)
	}
}

func TestStatusCodeAccepted(t *testing.T) {
	tests := []struct {
		name     string
		code     uint32
		expected string
		want     bool
	}{
		{name: "defaults to 200", code: 200, want: true},
		{name: "default rejects non-200", code: 204, want: false},
		{name: "custom accepts listed code", code: 201, expected: "201, 202", want: true},
		{name: "custom rejects missing code", code: 500, expected: "201, 202", want: false},
		{name: "range accepts in-range code", code: 204, expected: "200-299", want: true},
		{name: "range rejects out-of-range code", code: 301, expected: "200-299", want: false},
		{name: "mixed list and range", code: 302, expected: "200-299, 301, 302", want: true},
		{name: "range boundaries inclusive", code: 299, expected: "200-299", want: true},
		{name: "ignores malformed segment", code: 200, expected: "abc, 200", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusCodeAccepted(tt.code, tt.expected); got != tt.want {
				t.Fatalf("statusCodeAccepted(%d, %q) = %v, want %v", tt.code, tt.expected, got, tt.want)
			}
		})
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		isHTTPS bool
		want    string
	}{
		{name: "https certificate error", err: errors.New("x509: certificate signed by unknown authority"), isHTTPS: true, want: StageTLS},
		{name: "https handshake error", err: errors.New("remote error: tls: handshake failure"), isHTTPS: true, want: StageTLS},
		{name: "http keeps tcp classification", err: errors.New("connection refused"), isHTTPS: false, want: StageTCP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyHTTPError(tt.err, tt.isHTTPS)
			if got != tt.want {
				t.Fatalf("classifyHTTPError() stage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyStage(t *testing.T) {
	t.Run("dns", func(t *testing.T) {
		got, _ := classifyStage(&net.DNSError{Err: "no such host", Name: "example.invalid"})
		if got != StageDNS {
			t.Fatalf("classifyStage() = %q, want %q", got, StageDNS)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		got, _ := classifyStage(context.DeadlineExceeded)
		if got != StageTimeout {
			t.Fatalf("classifyStage() = %q, want %q", got, StageTimeout)
		}
	})

	t.Run("tcp fallback", func(t *testing.T) {
		got, _ := classifyStage(errors.New("connection refused"))
		if got != StageTCP {
			t.Fatalf("classifyStage() = %q, want %q", got, StageTCP)
		}
	})
}

func TestCertificateStatus(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		cert *x509.Certificate
		info *store.CertificateInfo
		mon  store.Monitor
		want string
	}{
		{
			name: "expired certificate is down",
			cert: &x509.Certificate{NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: -1},
			want: store.StatusDown,
		},
		{
			name: "not yet valid certificate is down",
			cert: &x509.Certificate{NotBefore: now.Add(24 * time.Hour), NotAfter: now.Add(48 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 2},
			want: store.StatusDown,
		},
		{
			name: "critical threshold",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(5 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 5},
			mon:  store.Monitor{CriticalDays: 7, WarningDays: 30},
			want: store.StatusCritical,
		},
		{
			name: "warning threshold",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(20 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 20},
			mon:  store.Monitor{CriticalDays: 7, WarningDays: 30},
			want: store.StatusWarning,
		},
		{
			name: "healthy",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(90 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 90},
			mon:  store.Monitor{CriticalDays: 7, WarningDays: 30},
			want: store.StatusHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := certificateStatus(tt.cert, tt.mon, tt.info); got != tt.want {
				t.Fatalf("certificateStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsIP(t *testing.T) {
	if !isIP(" 127.0.0.1 ") {
		t.Fatal("isIP() should accept IPv4 addresses with surrounding whitespace")
	}
	if !isIP("::1") {
		t.Fatal("isIP() should accept IPv6 addresses")
	}
	if isIP("example.com") {
		t.Fatal("isIP() should reject hostnames")
	}
}
