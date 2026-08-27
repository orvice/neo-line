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

var operationAttemptGracePeriod = OperationAttemptGracePeriod

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
	go m.runOperationPollLoop(ctx)
}

// StartIssueRunner is deprecated; use StartOperationRunner.
func (m *Manager) StartIssueRunner(ctx context.Context) {
	m.StartOperationRunner(ctx)
}

func (m *Manager) runOperationPollLoop(ctx context.Context) {
	ticker := time.NewTicker(operationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.claimingLeases.Store(false)
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
	now := m.clock.Now()
	_, _ = m.store.FailExpiredCertificateOperations(ctx, now)
	ops, err := m.store.FindClaimableCertificateOperations(ctx, now, 20)
	if err != nil {
		return
	}
	for _, op := range ops {
		if operationDeadlineExceeded(op, now) {
			continue
		}
		m.triggerOperation(op.ID)
	}
}

func (m *Manager) triggerOperation(opID string) {
	m.runnerCtxMu.RLock()
	ctx := m.runnerCtx
	m.runnerCtxMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	go m.runOperation(ctx, opID)
}

func (m *Manager) runOperation(ctx context.Context, opID string) {
	if _, loaded := m.opInflight.LoadOrStore(opID, struct{}{}); loaded {
		return
	}
	m.opInflightWG.Add(1)
	defer func() {
		m.opInflight.Delete(opID)
		m.opInflightWG.Done()
	}()

	now := m.clock.Now()
	op, err := m.store.TryClaimCertificateOperation(ctx, store.CertificateOperationClaimParams{
		OpID:         opID,
		Owner:        m.replicaID,
		Now:          now,
		LeaseExpires: now.Add(operationLeaseDuration),
	})
	if err != nil {
		return
	}
	if operationDeadlineExceeded(op, now) {
		m.failOperationDeadlineExceeded(ctx, op)
		return
	}

	takeover := op.AttemptCount > 1 && len(op.PendingTXTRecords) > 0
	if takeover {
		m.cleanupPendingTXT(ctx, op)
	}

	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	defer cancelAttempt()
	if !m.claimingLeases.Load() {
		var graceCancel context.CancelFunc
		attemptCtx, graceCancel = context.WithTimeout(attemptCtx, operationAttemptGracePeriod)
		defer graceCancel()
	}

	heartbeatDone := make(chan struct{})
	go m.heartbeatOperationLease(attemptCtx, opID, heartbeatDone)
	defer close(heartbeatDone)

	warning, runErr := m.executeOperation(attemptCtx, op, m.replicaID)
	if runErr == nil {
		metric.RecordCertOperation(certOpMetricType(op.Type), "succeeded")
		m.notifyOperationSuccess(ctx, op)
		return
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
			_ = m.store.RenewCertificateOperationLease(ctx, opID, m.replicaID, now.Add(operationLeaseDuration), now)
		}
	}
}

func (m *Manager) cleanupPendingTXT(ctx context.Context, op store.CertificateOperation) {
	if len(op.PendingTXTRecords) == 0 {
		return
	}
	dnsAccount, err := m.store.GetDNSProviderAccount(ctx, op.ConfigSnapshot.DNSProviderAccountID)
	if err != nil {
		return
	}
	if m.dnsFactory == nil {
		return
	}
	provider, err := m.dnsFactory.NewProvider(dnsAccount)
	if err != nil {
		return
	}
	for _, rec := range op.PendingTXTRecords {
		_ = provider.CleanUp(rec.Domain, rec.Token, rec.KeyAuth)
	}
	_ = m.store.ClearCertificateOperationPendingTXT(ctx, op.ID)
}

func (m *Manager) handleOperationFailure(ctx context.Context, op store.CertificateOperation, runErr error) {
	summary := sanitizeIssueError(runErr)
	if op.Type == store.CertOpTypeRevoke {
		summary = sanitizeRevokeError(runErr)
	}
	if op.Type == store.CertOpTypeRenew {
		metric.CertRenewFailuresTotal.Inc()
	}
	if isPermanentOperationError(runErr) || operationDeadlineExceeded(op, m.clock.Now()) {
		if operationDeadlineExceeded(op, m.clock.Now()) {
			summary = store.OperationTotalTimeoutSummary
		}
		metric.RecordCertOperation(certOpMetricType(op.Type), "failed")
		_ = m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, summary)
		m.notifyOperationFailure(ctx, op, summary)
		return
	}
	metric.RecordCertOperation(certOpMetricType(op.Type), "retry_scheduled")
	failures := op.ConsecutiveFailures + 1
	nextAt := m.clock.Now().Add(computeRetryDelay(failures, m.jitter))
	if op.DeadlineAt != nil && !nextAt.Before(*op.DeadlineAt) {
		metric.RecordCertOperation(certOpMetricType(op.Type), "failed")
		_ = m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, store.OperationTotalTimeoutSummary)
		m.notifyOperationFailure(ctx, op, store.OperationTotalTimeoutSummary)
		return
	}
	_ = m.store.ScheduleCertificateOperationRetry(ctx, op.ID, m.replicaID, nextAt, summary, failures)
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
	_ = m.store.MarkCertificateOperationFailed(ctx, op.ID, m.replicaID, store.OperationTotalTimeoutSummary)
	m.notifyOperationFailure(ctx, op, store.OperationTotalTimeoutSummary)
}

// WaitForInflightOperations blocks until all operation goroutines finish or ctx
// expires. Used during graceful shutdown before closing MongoDB.
func (m *Manager) WaitForInflightOperations(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		m.opInflightWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (m *Manager) persistPendingTXT(ctx context.Context, opID, owner string, records []store.DNSChallengeRecord) {
	if len(records) == 0 {
		return
	}
	_ = m.store.UpdateCertificateOperationPendingTXT(ctx, opID, owner, records)
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
