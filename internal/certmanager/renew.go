package certmanager

import (
	"context"
	"errors"

	"github.com/orvice/neo-line/internal/store"
)

// SubmitRenewOperation returns an existing running Renew operation or creates a
// new Pending Renew using the active version issuance snapshot.
func (m *Manager) SubmitRenewOperation(ctx context.Context, managedCertificateID string) (PublicOperation, error) {
	if existing, err := m.store.FindRunningCertificateOperation(ctx, managedCertificateID, store.CertOpTypeRenew); err == nil {
		return publicOperationFromStore(existing), nil
	} else if !store.IsNotFound(err) {
		return PublicOperation{}, err
	}
	if err := m.ensureNoConflictingIssuance(ctx, managedCertificateID, store.CertOpTypeRenew); err != nil {
		return PublicOperation{}, err
	}
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		return PublicOperation{}, err
	}
	if cert.ActiveVersion == nil {
		return PublicOperation{}, ErrNoActiveVersion
	}
	op, err := m.createPendingRenewOperation(ctx, cert)
	if err != nil {
		return PublicOperation{}, err
	}
	m.triggerRenewOperation(op.ID)
	return publicOperationFromStore(op), nil
}

func (m *Manager) ensureNoConflictingIssuance(ctx context.Context, managedCertificateID, selfType string) error {
	running, err := m.store.HasRunningCertificateOperation(ctx, managedCertificateID)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	for _, opType := range []string{store.CertOpTypeIssue, store.CertOpTypeRenew, store.CertOpTypeRevoke} {
		if opType == selfType {
			continue
		}
		if _, err := m.store.FindRunningCertificateOperation(ctx, managedCertificateID, opType); err == nil {
			return ErrIssuanceOperationInFlight
		} else if !store.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (m *Manager) createPendingRenewOperation(ctx context.Context, cert store.ManagedCertificate) (store.CertificateOperation, error) {
	if existing, err := m.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeRenew); err == nil {
		return existing, nil
	} else if !store.IsNotFound(err) {
		return store.CertificateOperation{}, err
	}
	if running, err := m.store.HasRunningCertificateOperation(ctx, cert.ID); err != nil {
		return store.CertificateOperation{}, err
	} else if running {
		return store.CertificateOperation{}, ErrIssuanceOperationInFlight
	}
	if cert.ActiveVersion == nil {
		return store.CertificateOperation{}, ErrNoActiveVersion
	}
	snapshot := cert.ActiveVersion.ConfigSnapshot
	return m.store.CreateCertificateOperation(ctx, store.CertificateOperation{
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeRenew,
		Status:               store.CertOpStatusPending,
		AttemptCount:         0,
		ConfigSnapshot: store.IssueConfigSnapshot{
			Domains:              append([]string(nil), snapshot.Domains...),
			CertificateIssuerID:  snapshot.CertificateIssuerID,
			DNSProviderAccountID: snapshot.DNSProviderAccountID,
			KeyType:              snapshot.KeyType,
		},
	})
}

func (m *Manager) triggerRenewOperation(opID string) {
	m.triggerOperation(opID)
}

func (m *Manager) runRenewOperation(ctx context.Context, opID string) {
	m.runOperation(ctx, opID)
}

func (m *Manager) reconcileAutoRenew(ctx context.Context) {
	certs, err := m.store.ListAutoRenewManagedCertificates(ctx)
	if err != nil {
		m.logger.WarnContext(ctx, "list auto-renew certificates", "error", err)
		return
	}
	now := m.clock.Now()
	for _, cert := range certs {
		if cert.ActiveVersion == nil || !cert.AutoRenewEnabled {
			continue
		}
		validity, _ := computeValidity(cert, now)
		if validity != store.CertValidityRenewalDue {
			continue
		}
		if _, err := m.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeRenew); err == nil {
			continue
		} else if !store.IsNotFound(err) {
			m.logger.WarnContext(ctx, "find running renew operation", "certificate_id", cert.ID, "error", err)
			continue
		}
		if err := m.ensureNoConflictingIssuance(ctx, cert.ID, store.CertOpTypeRenew); err != nil {
			if !errors.Is(err, ErrIssuanceOperationInFlight) {
				m.logger.WarnContext(ctx, "check conflicting certificate operation", "certificate_id", cert.ID, "error", err)
			}
			continue
		}
		op, err := m.createPendingRenewOperation(ctx, cert)
		if err != nil {
			m.logger.WarnContext(ctx, "create pending renew operation", "certificate_id", cert.ID, "error", err)
			continue
		}
		m.triggerRenewOperation(op.ID)
	}
}
