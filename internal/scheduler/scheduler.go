// Package scheduler periodically runs enabled monitors and persists their
// results. MongoDB remains the source of truth: every tick re-reads the set of
// enabled servers and monitors so configuration changes take effect without a
// restart.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/orvice/neo-line/internal/archive"
	"github.com/orvice/neo-line/internal/metric"
	"github.com/orvice/neo-line/internal/probe"
	"github.com/orvice/neo-line/internal/store"
)

// Alerter consumes monitor status transitions and dispatches notifications.
// Provided by internal/alert.Dispatcher in production; nil disables alerting.
type Alerter interface {
	OnMonitorStatusChange(ctx context.Context, monitor store.Monitor, prev, curr string, occurredAt time.Time)
}

const (
	// tickInterval is how often the scheduler re-evaluates which monitors are due.
	tickInterval = 5 * time.Second
	// maxConcurrentProbes bounds simultaneous in-flight probes.
	maxConcurrentProbes = 32
)

// Scheduler drives periodic execution of monitors.
type Scheduler struct {
	store    store.Store
	archiver archive.Archiver
	alerter  Alerter
	logger   *slog.Logger

	mu       sync.Mutex
	lastRun  map[string]time.Time
	inFlight map[string]bool
	sem      chan struct{}
}

// New builds a Scheduler backed by the given store. An optional archiver
// receives a copy of every persisted result; when omitted, results are not
// archived.
func New(st store.Store, archiver ...archive.Archiver) *Scheduler {
	var arch archive.Archiver = archive.Noop{}
	if len(archiver) > 0 && archiver[0] != nil {
		arch = archiver[0]
	}
	return &Scheduler{
		store:    st,
		archiver: arch,
		lastRun:  make(map[string]time.Time),
		inFlight: make(map[string]bool),
		sem:      make(chan struct{}, maxConcurrentProbes),
	}
}

// WithAlerter wires an Alerter into the scheduler. Returns the scheduler for
// chaining at construction time.
func (s *Scheduler) WithAlerter(a Alerter) *Scheduler {
	s.alerter = a
	return s
}

// Start runs the scheduling loop until ctx is cancelled. It blocks, so callers
// typically invoke it in a goroutine. The logger is bound here so it picks up
// the framework-configured slog default.
func (s *Scheduler) Start(ctx context.Context) {
	s.logger = slog.Default().With("component", "scheduler")
	s.logger.Info("scheduler started", "tick", tickInterval.String())
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping")
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

// reconcile loads the current enabled monitors and dispatches any that are due.
func (s *Scheduler) reconcile(ctx context.Context) {
	monitors, err := s.store.ListEnabledMonitors(ctx)
	if err != nil {
		s.logger.Error("failed to list enabled monitors", "error", err.Error())
		return
	}

	metric.ReconcileTotal.Inc()
	metric.EnabledMonitors.Set(float64(len(monitors)))

	now := time.Now()
	due := 0
	for _, m := range monitors {
		if !s.due(m, now) {
			continue
		}
		due++
		s.dispatch(ctx, m)
	}
	s.logger.Debug("reconcile tick", "enabled_monitors", len(monitors), "dispatched", due)
}

func (s *Scheduler) due(m store.Monitor, now time.Time) bool {
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight[m.ID] {
		return false
	}
	last, ok := s.lastRun[m.ID]
	if !ok {
		return true
	}
	return now.Sub(last) >= interval
}

func (s *Scheduler) dispatch(ctx context.Context, m store.Monitor) {
	s.mu.Lock()
	s.inFlight[m.ID] = true
	s.lastRun[m.ID] = time.Now()
	s.mu.Unlock()

	go func() {
		s.sem <- struct{}{}
		defer func() {
			<-s.sem
			s.mu.Lock()
			delete(s.inFlight, m.ID)
			s.mu.Unlock()
		}()

		result := probe.Run(ctx, m)
		s.recordMetrics(m, result)
		prevStatus, err := s.store.SaveCheckResult(ctx, result)
		if err != nil {
			s.logger.Error("failed to save check result", "monitor_id", m.ID, "error", err.Error())
			return
		}
		s.archiver.Enqueue(result)
		if prevStatus != "" && prevStatus != result.Status {
			metric.StatusChangesTotal.WithLabelValues(m.Kind, prevStatus, result.Status).Inc()
			s.logger.Info("monitor status changed",
				"monitor_id", m.ID,
				"server_id", m.ServerID,
				"kind", m.Kind,
				"prev_status", prevStatus,
				"status", result.Status,
			)
		}
		if s.alerter != nil {
			s.alerter.OnMonitorStatusChange(ctx, m, prevStatus, result.Status, result.EndedAt)
		}
		s.logger.Debug("probe completed",
			"monitor_id", m.ID,
			"server_id", m.ServerID,
			"kind", m.Kind,
			"status", result.Status,
			"duration_ms", result.DurationMS,
		)
	}()
}

// recordMetrics updates Prometheus metrics for a completed probe.
func (s *Scheduler) recordMetrics(m store.Monitor, result store.CheckResult) {
	metric.ProbesTotal.WithLabelValues(m.Kind, result.Status).Inc()
	metric.ProbeDuration.WithLabelValues(m.Kind).Observe(float64(result.DurationMS) / 1000)
	metric.MonitorStatus.WithLabelValues(m.ID, m.ServerID, m.Kind).Set(metric.StatusCode(result.Status))
	if result.ErrorStage != "" && result.ErrorStage != probe.StageNone {
		metric.ProbeErrorsTotal.WithLabelValues(m.Kind, result.ErrorStage).Inc()
	}
	if result.Certificate != nil {
		metric.CertificateDaysRemaining.WithLabelValues(m.ID, m.ServerID).Set(float64(result.Certificate.DaysRemaining))
	}
}
