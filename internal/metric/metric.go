// Package metric defines and registers the Prometheus metrics exposed by
// neo-line. Metrics are registered against the Butterfly-managed Prometheus
// registry, which the framework serves at /metrics on its dedicated port.
package metric

import (
	"butterfly.orx.me/core/observe/otel"
	"github.com/orvice/neo-line/internal/store"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ProbesTotal counts probe executions partitioned by monitor kind and
	// resulting health status.
	ProbesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "neoline_probe_total",
			Help: "Total number of monitor probes executed.",
		},
		[]string{"kind", "status"},
	)

	// ProbeDuration tracks probe wall-clock latency in seconds per monitor kind.
	ProbeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "neoline_probe_duration_seconds",
			Help:    "Monitor probe duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"kind"},
	)

	// ProbeErrorsTotal counts failed probes partitioned by kind and the stage
	// (dns/tcp/tls/http/timeout) at which the failure was classified.
	ProbeErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "neoline_probe_errors_total",
			Help: "Total number of failed monitor probes by error stage.",
		},
		[]string{"kind", "stage"},
	)

	// StatusChangesTotal counts monitor status transitions.
	StatusChangesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "neoline_monitor_status_changes_total",
			Help: "Total number of monitor status transitions.",
		},
		[]string{"kind", "previous_status", "status"},
	)

	// MonitorStatus reports the current health status of each monitor as a
	// numeric code (see StatusCode).
	MonitorStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "neoline_monitor_status",
			Help: "Current monitor health status (0=Unknown,1=Healthy,2=Warning,3=Critical,4=Down).",
		},
		[]string{"monitor_id", "server_id", "kind"},
	)

	// CertificateDaysRemaining reports days until certificate expiry for
	// monitors that observe a peer certificate (url/tls_port).
	CertificateDaysRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "neoline_certificate_days_remaining",
			Help: "Days remaining until the observed TLS certificate expires.",
		},
		[]string{"monitor_id", "server_id"},
	)

	// EnabledMonitors reports how many monitors are currently enabled.
	EnabledMonitors = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "neoline_enabled_monitors",
			Help: "Number of currently enabled monitors.",
		},
	)

	// ReconcileTotal counts scheduler reconcile ticks.
	ReconcileTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "neoline_scheduler_reconcile_total",
			Help: "Total number of scheduler reconcile ticks.",
		},
	)
)

func init() {
	otel.PrometheusRegistry().MustRegister(
		ProbesTotal,
		ProbeDuration,
		ProbeErrorsTotal,
		StatusChangesTotal,
		MonitorStatus,
		CertificateDaysRemaining,
		EnabledMonitors,
		ReconcileTotal,
	)
}

// StatusCode maps a health status string to the numeric code exported by the
// MonitorStatus gauge.
func StatusCode(status string) float64 {
	switch status {
	case store.StatusHealthy:
		return 1
	case store.StatusWarning:
		return 2
	case store.StatusCritical:
		return 3
	case store.StatusDown:
		return 4
	default:
		return 0
	}
}
