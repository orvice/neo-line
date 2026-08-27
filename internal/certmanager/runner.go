package certmanager

import (
	"context"
	"time"
)

const issuePollInterval = 5 * time.Second

// StartIssueRunner polls for Pending Issue operations and executes them.
// Full certificate reconciler behavior (#22) builds on this loop.
func (m *Manager) StartIssueRunner(ctx context.Context) {
	go m.runIssuePollLoop(ctx)
}

func (m *Manager) runIssuePollLoop(ctx context.Context) {
	ticker := time.NewTicker(issuePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollPendingIssues(ctx)
		}
	}
}

func (m *Manager) pollPendingIssues(ctx context.Context) {
	ops, err := m.store.FindPendingIssueOperations(ctx, 20)
	if err != nil {
		return
	}
	for _, op := range ops {
		m.triggerIssueOperation(op.ID)
	}
}
