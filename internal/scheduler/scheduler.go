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

	"github.com/orvice/neo-line/internal/probe"
	"github.com/orvice/neo-line/internal/store"
)

const (
	// tickInterval is how often the scheduler re-evaluates which monitors are due.
	tickInterval = 5 * time.Second
	// maxConcurrentProbes bounds simultaneous in-flight probes.
	maxConcurrentProbes = 32
)

// Scheduler drives periodic execution of monitors.
type Scheduler struct {
	store  store.Store
	logger *slog.Logger

	mu       sync.Mutex
	lastRun  map[string]time.Time
	inFlight map[string]bool
	sem      chan struct{}
}

// New builds a Scheduler backed by the given store.
func New(st store.Store) *Scheduler {
	return &Scheduler{
		store:    st,
		lastRun:  make(map[string]time.Time),
		inFlight: make(map[string]bool),
		sem:      make(chan struct{}, maxConcurrentProbes),
	}
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

	now := time.Now()
	for _, m := range monitors {
		if !s.due(m, now) {
			continue
		}
		s.dispatch(ctx, m)
	}
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
		if err := s.store.SaveCheckResult(ctx, result); err != nil {
			s.logger.Error("failed to save check result", "monitor_id", m.ID, "error", err.Error())
			return
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
