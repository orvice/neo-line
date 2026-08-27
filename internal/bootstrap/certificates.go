// Package bootstrap wires certificate-management runtime components for the
// neo-line server entrypoint.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/orvice/neo-line/internal/certmanager"
	"github.com/orvice/neo-line/internal/certnotify"
	"github.com/orvice/neo-line/internal/store"
)

// EnsureCertificateIndexes creates MongoDB indexes for ACME certificate
// management collections.
func EnsureCertificateIndexes(ctx context.Context, mongoStore *store.MongoStore) error {
	if err := mongoStore.EnsureNotifyGroupIndexes(ctx); err != nil {
		return fmt.Errorf("ensure notify group indexes: %w", err)
	}
	if err := mongoStore.EnsureDNSProviderAccountIndexes(ctx); err != nil {
		return fmt.Errorf("ensure dns provider account indexes: %w", err)
	}
	if err := mongoStore.EnsureCertificateIssuerIndexes(ctx); err != nil {
		return fmt.Errorf("ensure certificate issuer indexes: %w", err)
	}
	if err := mongoStore.EnsureManagedCertificateIndexes(ctx); err != nil {
		return fmt.Errorf("ensure managed certificate indexes: %w", err)
	}
	if err := mongoStore.EnsureCertificateOperationIndexes(ctx); err != nil {
		return fmt.Errorf("ensure certificate operation indexes: %w", err)
	}
	if err := mongoStore.EnsureCertificateAccessTokenIndexes(ctx); err != nil {
		return fmt.Errorf("ensure certificate access token indexes: %w", err)
	}
	return nil
}

// CertificateRuntime holds wired certificate manager and background workers.
type CertificateRuntime struct {
	Manager *certmanager.Manager
}

// InitCertificates constructs the certificate manager and notification dispatcher.
func InitCertificates(mongoStore *store.MongoStore, logger *slog.Logger) *CertificateRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	certMgr := certmanager.NewManagerWithDeps(
		certmanager.NewStore(mongoStore),
		certmanager.NewCloudflareClient(nil),
		certmanager.NewLegoACMEClient(nil),
		certmanager.NewCloudflareDNSFactory(nil),
	)
	certMgr.SetLogger(logger.With("component", "certmanager"))
	certNotifier := certnotify.New(mongoStore, logger.With("component", "certnotify"))
	certMgr.SetCertNotifier(certNotifier)
	return &CertificateRuntime{Manager: certMgr}
}

// StartBackground starts the operation runner and certificate reconciler until
// ctx is canceled. reconcilerDone is closed when the reconciler loop exits.
func (r *CertificateRuntime) StartBackground(ctx context.Context, reconcilerDone chan struct{}) {
	r.Manager.StartOperationRunner(ctx)
	reconciler := certmanager.NewReconciler(r.Manager)
	go func() {
		reconciler.Start(ctx)
		close(reconcilerDone)
	}()
}
