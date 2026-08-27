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

	// ServerCertListTotal counts successful ServerCertificateService list calls.
	ServerCertListTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "neoline_server_cert_list_total",
			Help: "Total number of successful Server ListCertificates calls.",
		},
	)

	// ServerCertBundleDownloadTotal counts successful GetCertificateBundle calls.
	ServerCertBundleDownloadTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "neoline_server_cert_bundle_download_total",
			Help: "Total number of successful Server GetCertificateBundle calls.",
		},
	)

	// ManagedCertValidity reports how many ManagedCertificates are in each active
	// validity state. Labels are bounded to the fixed validity vocabulary.
	ManagedCertValidity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "neoline_managed_cert_validity",
			Help: "Count of ManagedCertificates by active validity state.",
		},
		[]string{"validity"},
	)

	// CertOperationTotal counts certificate operation outcomes by type and result.
	CertOperationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "neoline_cert_operation_total",
			Help: "Total certificate operations by type and result.",
		},
		[]string{"op_type", "result"},
	)

	// CertRenewFailuresTotal counts renew operation failures (retry or terminal).
	CertRenewFailuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "neoline_cert_renew_failures_total",
			Help: "Total failed certificate renew operation attempts.",
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
		ServerCertListTotal,
		ServerCertBundleDownloadTotal,
		ManagedCertValidity,
		CertOperationTotal,
		CertRenewFailuresTotal,
	)
}

// RecordCertOperation increments the operation counter for the given type and result.
func RecordCertOperation(opType, result string) {
	CertOperationTotal.WithLabelValues(opType, result).Inc()
}

// RefreshManagedCertValidity sets gauge values for each validity label. Unknown
// validity strings are ignored; labels with zero count are reset to 0.
func RefreshManagedCertValidity(counts map[string]int) {
	for _, v := range []string{"Missing", "Valid", "RenewalDue", "Expired", "Revoked"} {
		ManagedCertValidity.WithLabelValues(v).Set(float64(counts[v]))
	}
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
