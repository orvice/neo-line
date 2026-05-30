package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// probeTLSPort opens a TCP connection, performs a TLS handshake without sending
// any application data, and evaluates the peer certificate's validity window
// against the monitor's warning/critical day thresholds.
func probeTLSPort(ctx context.Context, m store.Monitor, _ time.Duration, logger *slog.Logger) outcome {
	address := net.JoinHostPort(m.Host, strconv.FormatUint(uint64(m.Port), 10))
	logger.Debug("tls dial", "address", address)

	var dialer net.Dialer
	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		stage, msg := classifyStage(err)
		logger.Debug("tls dial failed", "address", address, "stage", stage, "error", msg)
		return outcome{status: store.StatusDown, stage: stage, errMsg: msg, port: m.Port}
	}
	defer rawConn.Close()
	remote := rawConn.RemoteAddr().String()

	if deadline, ok := ctx.Deadline(); ok {
		_ = rawConn.SetDeadline(deadline)
	}

	serverName := m.SNIName
	if serverName == "" && !isIP(m.Host) {
		serverName = m.Host
	}
	tlsConfig := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: !m.TLSVerify,
	}

	logger.Debug("tls handshake", "remote_address", remote, "server_name", serverName, "verify", m.TLSVerify)
	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		logger.Debug("tls handshake failed", "remote_address", remote, "error", err.Error())
		return outcome{status: store.StatusDown, stage: StageTLS, errMsg: err.Error(), remoteAddress: remote, port: m.Port}
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return outcome{status: store.StatusDown, stage: StageTLS, errMsg: "no peer certificate presented", remoteAddress: remote, port: m.Port}
	}
	leaf := certs[0]
	info := certificateInfo(leaf)
	logger.Debug("tls certificate read",
		"subject", info.Subject, "issuer", info.Issuer, "days_remaining", info.DaysRemaining)

	out := outcome{
		remoteAddress: remote,
		port:          m.Port,
		certificate:   info,
		stage:         StageNone,
	}
	out.status = certificateStatus(leaf, m, info)
	if out.status == store.StatusDown {
		out.stage = StageTLS
		out.errMsg = certificateDownReason(leaf)
	}
	return out
}

// certificateStatus derives a health status from the certificate validity
// window and the monitor's warning/critical day thresholds.
func certificateStatus(cert *x509.Certificate, m store.Monitor, info *store.CertificateInfo) string {
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return store.StatusDown
	}
	warning := m.WarningDays
	if warning == 0 {
		warning = 30
	}
	critical := m.CriticalDays
	if critical == 0 {
		critical = 7
	}
	days := info.DaysRemaining
	switch {
	case days <= int32(critical):
		return store.StatusCritical
	case days <= int32(warning):
		return store.StatusWarning
	default:
		return store.StatusHealthy
	}
}

func certificateDownReason(cert *x509.Certificate) string {
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return "certificate is not yet valid"
	}
	if now.After(cert.NotAfter) {
		return "certificate has expired"
	}
	return "certificate validation failed"
}

// certificateInfo extracts the metadata neo-line records for a peer certificate.
func certificateInfo(cert *x509.Certificate) *store.CertificateInfo {
	daysRemaining := int32(time.Until(cert.NotAfter).Hours() / 24)
	return &store.CertificateInfo{
		Subject:       cert.Subject.String(),
		Issuer:        cert.Issuer.String(),
		DNSNames:      cert.DNSNames,
		SerialNumber:  cert.SerialNumber.String(),
		NotBefore:     cert.NotBefore.UTC(),
		NotAfter:      cert.NotAfter.UTC(),
		DaysRemaining: daysRemaining,
	}
}

func isIP(host string) bool {
	return net.ParseIP(strings.TrimSpace(host)) != nil
}
