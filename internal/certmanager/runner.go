package certmanager

import (
	"context"
	"time"
)

const operationPollInterval = 5 * time.Second

// StartOperationRunner polls for Pending Issue and Renew operations and executes them.
func (m *Manager) StartOperationRunner(ctx context.Context) {
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
			return
		case <-ticker.C:
			m.pollPendingOperations(ctx)
		}
	}
}

func (m *Manager) pollPendingOperations(ctx context.Context) {
	issueOps, err := m.store.FindPendingIssueOperations(ctx, 20)
	if err != nil {
		return
	}
	for _, op := range issueOps {
		m.triggerIssueOperation(op.ID)
	}
	renewOps, err := m.store.FindPendingRenewOperations(ctx, 20)
	if err != nil {
		return
	}
	for _, op := range renewOps {
		m.triggerRenewOperation(op.ID)
	}
}
