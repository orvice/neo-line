package certmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/orvice/neo-line/internal/metric"
	"github.com/orvice/neo-line/internal/store"
)

const (
	operationPollInterval      = 5 * time.Second
	operationLeaseDuration     = store.DefaultOperationLeaseDuration
	operationHeartbeatInterval = operationLeaseDuration / 3
	// OperationAttemptGracePeriod bounds in-flight attempt cleanup after shutdown.
	OperationAttemptGracePeriod = 2 * time.Minute
	operationRetryInitial       = 15 * time.Minute
	operationRetryMax           = 12 * time.Hour
)

// JitterFunc adds bounded jitter to retry delays; tests inject deterministic values.
type JitterFunc func(base time.Duration) time.Duration

func defaultJitter(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0
	}
	n := uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 | uint64(buf[3])<<32 |
		uint64(buf[4])<<24 | uint64(buf[5])<<16 | uint64(buf[6])<<8 | uint64(buf[7])
	frac := float64(n%1000) / 1000.0
	return time.Duration(float64(base) * 0.1 * frac)
}

func computeRetryDelay(consecutiveFailures uint32, jitter JitterFunc) time.Duration {
	if consecutiveFailures == 0 {
		consecutiveFailures = 1
	}
	delay := operationRetryInitial
	for i := uint32(1); i < consecutiveFailures; i++ {
		if delay >= operationRetryMax {
			delay = operationRetryMax
			break
		}
		delay *= 2
	}
	if delay > operationRetryMax {
		delay = operationRetryMax
	}
	if jitter == nil {
		jitter = defaultJitter
	}
	return delay + jitter(delay)
}

func newReplicaID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "neo-line"
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return host + "-" + hex.EncodeToString(buf[:])
}

// StartOperationRunner polls for claimable operations and executes them with leases.
func (m *Manager) StartOperationRunner(ctx context.Context) {
	if m.replicaID == "" {
		m.replicaID = newReplicaID()
	}
	if m.jitter == nil {
		m.jitter = defaultJitter
	}
	m.runnerCtxMu.Lock()
	m.runnerCtx = ctx
	m.runnerCtxMu.Unlock()
	m.claimingLeases.Store(true)
	if m.logger != nil {
		m.logger.InfoContext(ctx, "certificate operation runner started",
			"replica_id", m.replicaID,
			"poll_interval", operationPollInterval.String(),
			"lease_duration", operationLeaseDuration.String(),
		)
	}
	if !m.launchBackgroundTask(func() { m.runOperationPollLoop(ctx) }) {
		m.claimingLeases.Store(false)
		if m.logger != nil {
			m.logger.ErrorContext(ctx, "certificate operation runner failed to start", "replica_id", m.replicaID)
		}
	}
}

func (m *Manager) runOperationPollLoop(ctx context.Context) {
	ticker := time.NewTicker(operationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.claimingLeases.Store(false)
			if m.logger != nil {
				m.logger.InfoContext(ctx, "certificate operation runner stopped", "replica_id", m.replicaID)
			}
			return
		case <-ticker.C:
			if !m.claimingLeases.Load() {
				continue
			}
			m.pollClaimableOperations(ctx)
		}
	}
}

func (m *Manager) pollClaimableOperations(ctx context.Context) {
	if !m.claimingLeases.Load() {
		return
	}
	now := m.clock.Now()
	expired, err := m.store.FailExpiredCertificateOperations(ctx, now)
	if err != nil {
		if m.logger != nil {
			attrs := []any{"error_stage", "fail_expired_operations"}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "fail expired certificate operations failed", attrs...)
		}
	} else if expired > 0 && m.logger != nil {
		m.logger.WarnContext(ctx, "expired certificate operations marked failed", "count", expired)
	}
	ops, err := m.store.FindClaimableCertificateOperations(ctx, now, 20)
	if err != nil {
		if m.logger != nil {
			attrs := []any{"error_stage", "find_claimable_operations"}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "find claimable certificate operations failed", attrs...)
		}
		return
	}
	for _, op := range ops {
		if operationDeadlineExceeded(op, now) {
			if m.logger != nil {
				attrs := operationLogAttrs(op)
				attrs = append(attrs, "error_stage", "operation_deadline")
				m.logger.WarnContext(ctx, "claimable certificate operation exceeded deadline", attrs...)
			}
			continue
		}
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			if op.NextAttemptAt != nil {
				attrs = append(attrs, "claimable_next_attempt_at", *op.NextAttemptAt)
			}
			m.logger.InfoContext(ctx, "claimable certificate operation scheduled", attrs...)
		}
		m.triggerOperation(op.ID)
	}
}

func (m *Manager) triggerOperation(opID string) {
	if !m.claimingLeases.Load() {
		if m.logger != nil {
			m.logger.Warn("certificate operation trigger skipped", "operation_id", opID, "error_stage", "trigger_leases_disabled")
		}
		return
	}
	ctx, ok := m.runnerContext()
	if !ok {
		if m.logger != nil {
			m.logger.Error("certificate operation trigger skipped", "operation_id", opID, "error_stage", "runner_context_unavailable")
		}
		return
	}
	if _, loaded := m.opInflight.LoadOrStore(opID, struct{}{}); loaded {
		return
	}
	if !m.launchBackgroundTask(func() {
		defer m.opInflight.Delete(opID)
		m.runOperation(ctx, opID)
	}) {
		m.opInflight.Delete(opID)
		if m.logger != nil {
			m.logger.Error("certificate operation trigger failed", "operation_id", opID, "error_stage", "background_task_rejected")
		}
	}
}

func (m *Manager) runOperation(ctx context.Context, opID string) {
	operationStarted := time.Now()
	now := m.clock.Now()
	op, err := m.store.TryClaimCertificateOperation(ctx, store.CertificateOperationClaimParams{
		OpID:         opID,
		Owner:        m.replicaID,
		Now:          now,
		LeaseExpires: now.Add(operationLeaseDuration),
	})
	if err != nil {
		if m.logger != nil {
			attrs := []any{
				"operation_id", opID,
				"replica_id", m.replicaID,
				"duration_ms", time.Since(operationStarted).Milliseconds(),
			}
			attrs = append(attrs, safeErrorAttrs(err)...)
			if errors.Is(err, store.ErrCertificateOperationConflict) {
				m.logger.InfoContext(ctx, "certificate operation claim skipped", attrs...)
			} else {
				m.logger.ErrorContext(ctx, "certificate operation claim failed", attrs...)
			}
		}
		return
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "replica_id", m.replicaID, "claim_duration_ms", time.Since(operationStarted).Milliseconds())
		m.logger.InfoContext(ctx, "certificate operation claimed", attrs...)
	}
	if operationDeadlineExceeded(op, now) {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "operation_deadline")
			m.logger.WarnContext(ctx, "certificate operation deadline exceeded before execution", attrs...)
		}
		m.failOperationDeadlineExceeded(ctx, op)
		return
	}

	takeover := op.AttemptCount > 1 && len(op.PendingTXTRecords) > 0
	if takeover {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "takeover", true)
			m.logger.WarnContext(ctx, "certificate operation taking over expired lease", attrs...)
		}
		m.cleanupPendingTXT(ctx, op)
	}

	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	defer cancelAttempt()
	if !m.claimingLeases.Load() {
		var graceCancel context.CancelFunc
		attemptCtx, graceCancel = context.WithTimeout(attemptCtx, OperationAttemptGracePeriod)
		defer graceCancel()
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "grace_period", OperationAttemptGracePeriod.String())
			m.logger.WarnContext(ctx, "certificate operation running during shutdown grace period", attrs...)
		}
	}

	heartbeatDone := make(chan struct{})
	go m.heartbeatOperationLease(attemptCtx, opID, heartbeatDone)
	defer close(heartbeatDone)

	warning, runErr := m.executeOperation(attemptCtx, op, m.replicaID)
	if runErr == nil {
		metric.RecordCertOperation(certOpMetricType(op.Type), "succeeded")
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs,
				"duration_ms", time.Since(operationStarted).Milliseconds(),
				"warning_present", warning != "",
			)
			m.logger.InfoContext(ctx, "certificate operation succeeded", attrs...)
		}
		m.notifyOperationSuccess(ctx, op)
		return
	}
	if m.logger != nil {
		stage := operationStageOf(runErr)
		if stage == "" {
			stage = "operation"
		}
		attrs := operationLogAttrs(op)
		attrs = append(attrs,
			"duration_ms", time.Since(operationStarted).Milliseconds(),
			"error_stage", stage,
		)
		attrs = append(attrs, safeErrorAttrs(runErr)...)
		m.logger.ErrorContext(ctx, "certificate operation failed", attrs...)
	}
	_ = warning
	m.handleOperationFailure(ctx, op, runErr)
}

func (m *Manager) notifyOperationSuccess(ctx context.Context, op store.CertificateOperation) {
	if m.certNotifier == nil {
		return
	}
	cert, err := m.store.GetManagedCertificate(ctx, op.ManagedCertificateID)
	if err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "load_notification_certificate")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.WarnContext(ctx, "load certificate for success notification failed", attrs...)
		}
		return
	}
	m.certNotifier.OnOperationSuccess(ctx, cert, op)
}

func (m *Manager) notifyOperationFailure(ctx context.Context, op store.CertificateOperation, errorSummary string) {
	if m.certNotifier == nil {
		return
	}
	cert, err := m.store.GetManagedCertificate(ctx, op.ManagedCertificateID)
	if err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "load_notification_certificate")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.WarnContext(ctx, "load certificate for failure notification failed", attrs...)
		}
		return
	}
	m.certNotifier.OnOperationFailure(ctx, cert, op, errorSummary)
}

func (m *Manager) executeOperation(ctx context.Context, op store.CertificateOperation, leaseOwner string) (warning string, err error) {
	switch op.Type {
	case store.CertOpTypeRevoke:
		return "", m.executeCertificateRevocation(ctx, op, leaseOwner)
	default:
		return m.executeCertificateIssuance(ctx, op, leaseOwner)
	}
}

func (m *Manager) heartbeatOperationLease(ctx context.Context, opID string, done <-chan struct{}) {
	ticker := time.NewTicker(operationHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			now := m.clock.Now()
			nextExpiry := now.Add(operationLeaseDuration)
			if err := m.store.RenewCertificateOperationLease(ctx, opID, m.replicaID, nextExpiry, now); err != nil {
				if m.logger != nil {
					attrs := []any{
						"operation_id", opID,
						"replica_id", m.replicaID,
						"error_stage", "renew_lease",
					}
					attrs = append(attrs, safeErrorAttrs(err)...)
					m.logger.ErrorContext(ctx, "certificate operation lease renewal failed", attrs...)
				}
			} else if m.logger != nil {
				m.logger.InfoContext(ctx, "certificate operation lease renewed",
					"operation_id", opID,
					"replica_id", m.replicaID,
					"lease_expires_at", nextExpiry,
				)
			}
		}
	}
}

func (m *Manager) cleanupPendingTXT(ctx context.Context, op store.CertificateOperation) {
	if len(op.PendingTXTRecords) == 0 {
		return
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "error_stage", "cleanup_pending_txt")
		m.logger.InfoContext(ctx, "cleanup pending DNS challenges started", attrs...)
	}
	dnsAccount, err := m.store.GetDNSProviderAccount(ctx, op.ConfigSnapshot.DNSProviderAccountID)
	if err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "cleanup_resolve_dns_account")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "resolve DNS account for pending challenge cleanup failed", attrs...)
		}
		return
	}
	if m.dnsFactory == nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "cleanup_create_dns_provider")
			m.logger.ErrorContext(ctx, "DNS provider factory unavailable for pending challenge cleanup", attrs...)
		}
		return
	}
	provider, err := m.dnsFactory.NewProvider(dnsAccount)
	if err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "cleanup_create_dns_provider")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "create DNS provider for pending challenge cleanup failed", attrs...)
		}
		return
	}
	for _, rec := range op.PendingTXTRecords {
		if err := provider.CleanUp(rec.Domain, rec.Token, rec.KeyAuth); err != nil && m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "domain", rec.Domain, "error_stage", "cleanup_pending_txt_record")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.WarnContext(ctx, "pending DNS challenge cleanup failed", attrs...)
		}
	}
	if err := m.store.ClearCertificateOperationPendingTXT(ctx, op.ID); err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "clear_pending_txt")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "clear pending DNS challenges failed", attrs...)
		}
		return
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		m.logger.InfoContext(ctx, "cleanup pending DNS challenges completed", attrs...)
	}
}

func (m *Manager) handleOperationFailure(ctx context.Context, op store.CertificateOperation, runErr error) {
	summary := sanitizeIssueError(runErr)
	if op.Type == store.CertOpTypeRevoke {
		summary = sanitizeRevokeError(runErr)
	}
	if op.Type == store.CertOpTypeRenew {
		metric.CertRenewFailuresTotal.Inc()
	}
	deadlineExceeded := operationDeadlineExceeded(op, m.clock.Now())
	if isPermanentOperationError(runErr) || deadlineExceeded {
		if deadlineExceeded {
			summary = store.OperationTotalTimeoutSummary
		}
		metric.RecordCertOperation(certOpMetricType(op.Type), "failed")
		if err := m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, summary); err != nil {
			if m.logger != nil {
				attrs := operationLogAttrs(op)
				attrs = append(attrs, "error_stage", "mark_operation_failed")
				attrs = append(attrs, safeErrorAttrs(err)...)
				m.logger.ErrorContext(ctx, "persist terminal certificate operation failure failed", attrs...)
			}
		} else if m.logger != nil {
			failureKind := "permanent_error"
			if deadlineExceeded {
				failureKind = "deadline_exceeded"
			}
			attrs := operationLogAttrs(op)
			attrs = append(attrs,
				"final_status", store.CertOpStatusFailed,
				"failure_kind", failureKind,
			)
			m.logger.WarnContext(ctx, "certificate operation marked failed", attrs...)
		}
		m.notifyOperationFailure(ctx, op, summary)
		return
	}
	metric.RecordCertOperation(certOpMetricType(op.Type), "retry_scheduled")
	failures := op.ConsecutiveFailures + 1
	nextAt := m.clock.Now().Add(computeRetryDelay(failures, m.jitter))
	if op.DeadlineAt != nil && !nextAt.Before(*op.DeadlineAt) {
		metric.RecordCertOperation(certOpMetricType(op.Type), "failed")
		if err := m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, store.OperationTotalTimeoutSummary); err != nil {
			if m.logger != nil {
				attrs := operationLogAttrs(op)
				attrs = append(attrs, "error_stage", "mark_operation_failed_deadline")
				attrs = append(attrs, safeErrorAttrs(err)...)
				m.logger.ErrorContext(ctx, "persist deadline certificate operation failure failed", attrs...)
			}
		} else if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs,
				"final_status", store.CertOpStatusFailed,
				"failure_kind", "retry_would_exceed_deadline",
			)
			m.logger.WarnContext(ctx, "certificate operation retry would exceed deadline", attrs...)
		}
		m.notifyOperationFailure(ctx, op, store.OperationTotalTimeoutSummary)
		return
	}
	if err := m.store.ScheduleCertificateOperationRetry(ctx, op.ID, m.replicaID, nextAt, summary, failures); err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "schedule_retry")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "schedule certificate operation retry failed", attrs...)
		}
	} else if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs,
			"next_attempt_at", nextAt,
			"next_consecutive_failures", failures,
			"final_status", store.CertOpStatusPending,
		)
		m.logger.WarnContext(ctx, "certificate operation retry scheduled", attrs...)
	}
	m.notifyOperationFailure(ctx, op, summary)
}

func certOpMetricType(opType string) string {
	switch opType {
	case store.CertOpTypeIssue:
		return "issue"
	case store.CertOpTypeRenew:
		return "renew"
	case store.CertOpTypeRevoke:
		return "revoke"
	default:
		return "unknown"
	}
}

func isPermanentOperationError(err error) bool {
	return errors.Is(err, ErrIssuerNotReady)
}

func operationDeadlineExceeded(op store.CertificateOperation, now time.Time) bool {
	return op.DeadlineAt != nil && !now.Before(*op.DeadlineAt)
}

func (m *Manager) failOperationDeadlineExceeded(ctx context.Context, op store.CertificateOperation) {
	metric.RecordCertOperation(certOpMetricType(op.Type), "failed")
	if err := m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, store.OperationTotalTimeoutSummary); err != nil {
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			attrs = append(attrs, "error_stage", "mark_operation_failed_deadline")
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "persist operation deadline failure failed", attrs...)
		}
	}
	m.notifyOperationFailure(ctx, op, store.OperationTotalTimeoutSummary)
}

// WaitForInflightOperations stops task admission and waits for operation,
// poll-loop, and issuer-registration goroutines before MongoDB is closed.
func (m *Manager) WaitForInflightOperations(ctx context.Context) {
	m.taskMu.Lock()
	m.tasksClosed = true
	m.taskMu.Unlock()

	done := make(chan struct{})
	go func() {
		m.taskWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// SetReplicaID configures the replica identity used for lease ownership (tests).
func (m *Manager) SetReplicaID(id string) {
	m.replicaID = id
}

// SetJitter configures retry jitter (tests).
func (m *Manager) SetJitter(j JitterFunc) {
	m.jitter = j
}

// SetClaimingLeases controls whether this replica claims new leases (tests/shutdown).
func (m *Manager) SetClaimingLeases(v bool) {
	m.claimingLeases.Store(v)
}
