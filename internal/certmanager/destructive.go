package certmanager

import (
	"context"

	"github.com/orvice/neo-line/internal/store"
)

// DeleteManagedCertificate removes local certificate state when unassigned and idle.
func (m *Manager) DeleteManagedCertificate(ctx context.Context, id string) error {
	return m.store.DeleteManagedCertificate(ctx, id)
}

func (m *Manager) ensureIssuerDeletable(ctx context.Context, issuerID string) error {
	count, err := m.store.CountManagedCertificatesReferencingIssuer(ctx, issuerID)
	if err != nil {
		return err
	}
	if count > 0 {
		return store.ErrCertificateResourceReferenced
	}
	return nil
}

func (m *Manager) ensureDNSAccountDeletable(ctx context.Context, dnsID string) error {
	count, err := m.store.CountManagedCertificatesReferencingDNSAccount(ctx, dnsID)
	if err != nil {
		return err
	}
	if count > 0 {
		return store.ErrCertificateResourceReferenced
	}
	return nil
}
