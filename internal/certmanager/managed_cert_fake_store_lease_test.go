package certmanager

import (
	"context"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func (f *managedCertFakeStore) HasRunningCertificateOperation(_ context.Context, managedCertificateID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.opOrd {
		op := f.ops[id]
		if op.ManagedCertificateID != managedCertificateID {
			continue
		}
		for _, st := range store.CertOpInFlightStatuses {
			if op.Status == st {
				return true, nil
			}
		}
	}
	return false, nil
}

func (f *managedCertFakeStore) FindClaimableCertificateOperations(_ context.Context, now time.Time, limit int64) ([]store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.CertificateOperation
	for _, id := range f.opOrd {
		op := f.ops[id]
		if !isClaimableOp(op, now) {
			continue
		}
		out = append(out, op)
		if limit > 0 && int64(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func isClaimableOp(op store.CertificateOperation, now time.Time) bool {
	switch op.Status {
	case store.CertOpStatusPending:
		if op.NextAttemptAt != nil && op.NextAttemptAt.After(now) {
			return false
		}
		return true
	case store.CertOpStatusRunning:
		if op.LeaseExpiresAt == nil || !op.LeaseExpiresAt.After(now) {
			return true
		}
	}
	return false
}

func (f *managedCertFakeStore) TryClaimCertificateOperation(_ context.Context, p store.CertificateOperationClaimParams) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[p.OpID]
	if !ok || !isClaimableOp(op, p.Now) {
		return store.CertificateOperation{}, store.ErrCertificateOperationConflict
	}
	for _, id := range f.opOrd {
		other := f.ops[id]
		if other.ManagedCertificateID != op.ManagedCertificateID || other.ID == op.ID {
			continue
		}
		if other.Status == store.CertOpStatusRunning && other.LeaseExpiresAt != nil && other.LeaseExpiresAt.After(p.Now) {
			return store.CertificateOperation{}, store.ErrCertificateOperationConflict
		}
	}
	op.Status = store.CertOpStatusRunning
	op.AttemptCount++
	op.LeaseOwner = p.Owner
	exp := p.LeaseExpires
	op.LeaseExpiresAt = &exp
	op.UpdatedAt = p.Now
	op.FinishedAt = nil
	if op.StartedAt == nil {
		op.StartedAt = &p.Now
	}
	f.ops[p.OpID] = op
	return op, nil
}

func (f *managedCertFakeStore) RenewCertificateOperationLease(_ context.Context, opID, owner string, leaseExpires, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning || op.LeaseOwner != owner {
		return store.ErrCertificateOperationConflict
	}
	if op.LeaseExpiresAt == nil || !op.LeaseExpiresAt.After(now) {
		return store.ErrCertificateOperationConflict
	}
	op.LeaseExpiresAt = &leaseExpires
	op.UpdatedAt = now
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) UpdateCertificateOperationPendingTXT(_ context.Context, opID, owner string, records []store.DNSChallengeRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning || op.LeaseOwner != owner {
		return store.ErrCertificateOperationConflict
	}
	op.PendingTXTRecords = append([]store.DNSChallengeRecord(nil), records...)
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) ScheduleCertificateOperationRetry(_ context.Context, opID, owner string, nextAttemptAt time.Time, errorSummary string, consecutiveFailures uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning || op.LeaseOwner != owner {
		return store.ErrCertificateOperationConflict
	}
	op.Status = store.CertOpStatusPending
	op.ErrorSummary = errorSummary
	next := nextAttemptAt
	op.NextAttemptAt = &next
	op.ConsecutiveFailures = consecutiveFailures
	op.LeaseOwner = ""
	op.LeaseExpiresAt = nil
	op.UpdatedAt = time.Now().UTC()
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) MarkCertificateOperationFailed(_ context.Context, opID, owner, errorSummary string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning || op.LeaseOwner != owner {
		return store.ErrCertificateOperationConflict
	}
	now := time.Now().UTC()
	op.Status = store.CertOpStatusFailed
	op.ErrorSummary = errorSummary
	op.FinishedAt = &now
	op.LeaseOwner = ""
	op.LeaseExpiresAt = nil
	op.UpdatedAt = now
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) ClearCertificateOperationPendingTXT(_ context.Context, opID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok {
		return nil
	}
	op.PendingTXTRecords = nil
	f.ops[opID] = op
	return nil
}
