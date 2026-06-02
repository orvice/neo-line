package probe

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
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

func TestRunTreatsTLSKindAsTLSPort(t *testing.T) {
	cert := testTLSCertificate(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	defer listener.Close()

	handshakeDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			handshakeDone <- err
			return
		}
		defer conn.Close()

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			handshakeDone <- errors.New("accepted connection is not TLS")
			return
		}
		handshakeDone <- tlsConn.Handshake()
	}()

	addr := listener.Addr().(*net.TCPAddr)
	result := Run(context.Background(), store.Monitor{
		ID:             "mon_tls_alias",
		ServerID:       "srv_1",
		Kind:           "tls",
		Host:           "127.0.0.1",
		Port:           uint32(addr.Port),
		TLSVerify:      false,
		TimeoutSeconds: 2,
		WarningDays:    30,
		CriticalDays:   7,
	})

	if result.Status != store.StatusHealthy {
		t.Fatalf("Status = %q, want %q; result = %#v", result.Status, store.StatusHealthy, result)
	}
	if result.Certificate == nil {
		t.Fatalf("Certificate is nil; result = %#v", result)
	}
	if result.Certificate.DaysRemaining <= 30 {
		t.Fatalf("DaysRemaining = %d, want > 30", result.Certificate.DaysRemaining)
	}

	select {
	case err := <-handshakeDone:
		if err != nil {
			t.Fatalf("server handshake error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server handshake")
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
		{name: "range upper boundary inclusive", code: 299, expected: "200-299", want: true},
		{name: "range lower boundary inclusive", code: 200, expected: "200-299", want: true},
		{name: "ignores malformed segment", code: 200, expected: "abc, 200", want: true},
		{name: "all-malformed expression rejects", code: 200, expected: "abc, def", want: false},
		{name: "reversed range is ignored", code: 250, expected: "299-200", want: false},
		{name: "whitespace around dash", code: 250, expected: " 200 - 299 ", want: true},
		{name: "trailing comma tolerated", code: 200, expected: "200,", want: true},
		{name: "non-empty expr does not fall back to 200", code: 200, expected: "404", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusCodeAccepted(tt.code, tt.expected); got != tt.want {
				t.Fatalf("statusCodeAccepted(%d, %q) = %v, want %v", tt.code, tt.expected, got, tt.want)
			}
		})
	}
}

func testTLSCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("rand.Int() error = %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair() error = %v", err)
	}
	return cert
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
			mon:  store.Monitor{CriticalDays: store.DefaultTLSCriticalDays, WarningDays: store.DefaultTLSWarningDays},
			want: store.StatusCritical,
		},
		{
			name: "warning threshold",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(20 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 20},
			mon:  store.Monitor{CriticalDays: store.DefaultTLSCriticalDays, WarningDays: store.DefaultTLSWarningDays},
			want: store.StatusWarning,
		},
		{
			name: "healthy outside warning threshold",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(22 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 22},
			mon:  store.Monitor{CriticalDays: store.DefaultTLSCriticalDays, WarningDays: store.DefaultTLSWarningDays},
			want: store.StatusHealthy,
		},
		{
			name: "healthy",
			cert: &x509.Certificate{NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(90 * 24 * time.Hour)},
			info: &store.CertificateInfo{DaysRemaining: 90},
			mon:  store.Monitor{CriticalDays: store.DefaultTLSCriticalDays, WarningDays: store.DefaultTLSWarningDays},
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
