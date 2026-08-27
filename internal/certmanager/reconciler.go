package certmanager

import (
	"context"
	"time"
)

const reconcileScanInterval = time.Hour

// Reconciler scans auto-renew-enabled certificates and enqueues Renew operations.
// It is separate from the Monitor scheduler lifecycle.
type Reconciler struct {
	mgr      *Manager
	interval time.Duration
}

func NewReconciler(mgr *Manager) *Reconciler {
	return &Reconciler{mgr: mgr, interval: reconcileScanInterval}
}

// Start runs the hourly reconcile loop until ctx is canceled.
func (r *Reconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	r.Reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Reconcile(ctx)
		}
	}
}

// Reconcile performs one auto-renew scan and validity notification scan.
func (r *Reconciler) Reconcile(ctx context.Context) {
	if r.mgr == nil {
		return
	}
	r.mgr.reconcileAutoRenew(ctx)
	if r.mgr.certNotifier != nil {
		r.mgr.certNotifier.ScanValidityNotifications(ctx)
	}
}
