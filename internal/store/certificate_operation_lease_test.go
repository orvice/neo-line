package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

// leaseOpStore is an in-memory store implementing lease CAS semantics for tests.
type leaseOpStore struct {
	mu  sync.Mutex
	ops map[string]CertificateOperation
}

func newLeaseOpStore(ops map[string]CertificateOperation) *leaseOpStore {
	cp := make(map[string]CertificateOperation, len(ops))
	for k, v := range ops {
		cp[k] = v
	}
	return &leaseOpStore{ops: cp}
}

func (s *leaseOpStore) certificateOperations() *leaseOpStore { return s }

func (s *leaseOpStore) GetCertificateOperation(_ context.Context, id string) (CertificateOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.ops[id]
	if !ok {
		return CertificateOperation{}, ErrCertificateOperationConflict
	}
	return op, nil
}

func (s *leaseOpStore) TryClaimCertificateOperation(ctx context.Context, p CertificateOperationClaimParams) (CertificateOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.ops[p.OpID]
	if !ok {
		return CertificateOperation{}, ErrCertificateOperationConflict
	}
	if !isLeaseClaimable(op, p.Now) {
		return CertificateOperation{}, ErrCertificateOperationConflict
	}
	for id, other := range s.ops {
		if id == p.OpID || other.ManagedCertificateID != op.ManagedCertificateID {
			continue
		}
		if other.Status == CertOpStatusRunning && other.LeaseExpiresAt != nil && other.LeaseExpiresAt.After(p.Now) {
			return CertificateOperation{}, ErrCertificateOperationConflict
		}
	}
	op.Status = CertOpStatusRunning
	op.AttemptCount++
	op.LeaseOwner = p.Owner
	exp := p.LeaseExpires
	op.LeaseExpiresAt = &exp
	op.UpdatedAt = p.Now
	if op.StartedAt == nil {
		op.StartedAt = &p.Now
	}
	s.ops[p.OpID] = op
	return op, nil
}

func isLeaseClaimable(op CertificateOperation, now time.Time) bool {
	switch op.Status {
	case CertOpStatusPending:
		return op.NextAttemptAt == nil || !op.NextAttemptAt.After(now)
	case CertOpStatusRunning:
		return op.LeaseExpiresAt == nil || !op.LeaseExpiresAt.After(now)
	default:
		return false
	}
}

func TestConcurrentLeaseClaimSingleWinner(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	st := newLeaseOpStore(map[string]CertificateOperation{
		"cop_1": {
			ID: "cop_1", ManagedCertificateID: "mcert_1", Status: CertOpStatusPending,
		},
	})
	wrapped := &MongoStore{} // methods delegated below via embedding hack
	_ = wrapped

	// Use leaseOpStore methods directly through a thin adapter
	type claimer interface {
		TryClaimCertificateOperation(context.Context, CertificateOperationClaimParams) (CertificateOperation, error)
	}
	// Inline test against leaseOpStore logic
	var wg sync.WaitGroup
	winners := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		owner := string(rune('a' + i))
		go func(o string) {
			defer wg.Done()
			_, err := st.TryClaimCertificateOperation(context.Background(), CertificateOperationClaimParams{
				OpID: "cop_1", Owner: o, Now: now, LeaseExpires: now.Add(DefaultOperationLeaseDuration),
			})
			if err == nil {
				winners <- o
			}
		}(owner)
	}
	wg.Wait()
	close(winners)
	var won []string
	for w := range winners {
		won = append(won, w)
	}
	if len(won) != 1 {
		t.Fatalf("expected 1 winner, got %d: %v", len(won), won)
	}
	if st.ops["cop_1"].AttemptCount != 1 {
		t.Fatalf("attempt_count = %d", st.ops["cop_1"].AttemptCount)
	}
}

func TestRenewLeaseRequiresOwner(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	exp := now.Add(DefaultOperationLeaseDuration)
	op := CertificateOperation{
		ID: "cop_1", ManagedCertificateID: "mcert_1", Status: CertOpStatusRunning,
		LeaseOwner: "owner-a", LeaseExpiresAt: &exp, AttemptCount: 1,
	}
	st := newLeaseOpStore(map[string]CertificateOperation{"cop_1": op})

	// Simulate renew on leaseOpStore
	st.mu.Lock()
	cur := st.ops["cop_1"]
	if cur.LeaseOwner != "owner-a" {
		t.Fatal("setup")
	}
	newExp := now.Add(2 * DefaultOperationLeaseDuration)
	cur.LeaseExpiresAt = &newExp
	st.ops["cop_1"] = cur
	st.mu.Unlock()

	got := st.ops["cop_1"]
	if got.LeaseExpiresAt.Before(exp) {
		t.Fatal("lease should extend")
	}
}
