// Package probe executes a single monitor check against its target and reports
// the outcome as a store.CheckResult.
//
// Supported monitor kinds (per docs/monitoring-configuration.md):
//   - tcp:      TCP port reachability
//   - url:      HTTP/HTTPS endpoint availability (scheme decides protocol)
//   - tls_port: TLS handshake + certificate state on a raw TLS port
//
// "http"/"https" are accepted as aliases for url, and "tls"/"tls_certificate"
// as aliases for tls_port.
package probe

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Error stages recorded on a CheckResult when a probe fails.
const (
	StageNone    = "none"
	StageDNS     = "dns"
	StageTCP     = "tcp"
	StageTLS     = "tls"
	StageHTTP    = "http"
	StageTimeout = "timeout"
)

// outcome is the result of a single probe attempt, before timing/identity
// fields are attached.
type outcome struct {
	status        string
	stage         string
	errMsg        string
	remoteAddress string
	port          uint32
	httpCode      uint32
	certificate   *store.CertificateInfo
}

// Run executes the monitor, honoring its retry budget, and returns a fully
// populated CheckResult. A monitor is retried only while it reports Down; a
// definitive Warning/Critical/Healthy result short-circuits remaining retries.
func Run(ctx context.Context, m store.Monitor) store.CheckResult {
	started := time.Now().UTC()
	logger := slog.Default().With(
		"component", "probe",
		"monitor_id", m.ID,
		"server_id", m.ServerID,
		"kind", m.Kind,
		"target", probeTarget(m),
	)
	attempts := int(m.Retries) + 1
	if attempts < 1 {
		attempts = 1
	}
	logger.Debug("probe started", "attempts", attempts)

	var last outcome
	for i := 0; i < attempts; i++ {
		last = runOnce(ctx, m, logger)
		if last.status != store.StatusDown {
			break
		}
		if ctx.Err() != nil {
			logger.Debug("probe aborted, context cancelled", "attempt", i+1, "error", ctx.Err().Error())
			break
		}
		if i < attempts-1 {
			logger.Debug("probe attempt failed, retrying",
				"attempt", i+1, "stage", last.stage, "error", last.errMsg)
		}
	}

	ended := time.Now().UTC()
	durationMS := ended.Sub(started).Milliseconds()
	switch last.status {
	case store.StatusDown:
		logger.Warn("probe failed",
			"status", last.status, "stage", last.stage, "error", last.errMsg,
			"remote_address", last.remoteAddress, "duration_ms", durationMS)
	case store.StatusWarning, store.StatusCritical:
		logger.Info("probe degraded",
			"status", last.status, "stage", last.stage, "duration_ms", durationMS)
	default:
		logger.Debug("probe succeeded",
			"status", last.status, "http_code", last.httpCode,
			"remote_address", last.remoteAddress, "duration_ms", durationMS)
	}

	return store.CheckResult{
		ServerID:       m.ServerID,
		MonitorID:      m.ID,
		Status:         last.status,
		StartedAt:      started,
		EndedAt:        ended,
		DurationMS:     ended.Sub(started).Milliseconds(),
		ErrorStage:     last.stage,
		ErrorMessage:   last.errMsg,
		RemoteAddress:  last.remoteAddress,
		Port:           last.port,
		HTTPStatusCode: last.httpCode,
		Certificate:    last.certificate,
	}
}

func runOnce(ctx context.Context, m store.Monitor, logger *slog.Logger) outcome {
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch m.Kind {
	case "url", "http", "https":
		return probeURL(attemptCtx, m, timeout, logger)
	case "tls", "tls_port", "tls_certificate":
		return probeTLSPort(attemptCtx, m, timeout, logger)
	default: // "tcp" and unknown kinds fall back to TCP reachability.
		return probeTCP(attemptCtx, m, timeout, logger)
	}
}

// probeTarget renders a human-readable target for logging, matching the field
// set each probe kind actually dials.
func probeTarget(m store.Monitor) string {
	switch m.Kind {
	case "url", "http", "https":
		return m.URL
	default:
		return net.JoinHostPort(m.Host, strconv.FormatUint(uint64(m.Port), 10))
	}
}

// classifyStage maps a transport error to the layer it most likely failed at.
func classifyStage(err error) (stage, message string) {
	if err == nil {
		return StageNone, ""
	}
	message = err.Error()

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return StageDNS, message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StageTimeout, message
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StageTimeout, message
	}
	return StageTCP, message
}
