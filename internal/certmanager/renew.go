package certmanager

import (
	"context"
	"sync"

	"github.com/orvice/neo-line/internal/store"
)

var renewInflight sync.Map // opID -> struct{}

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
	for _, opType := range []string{store.CertOpTypeIssue, store.CertOpTypeRenew} {
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
	go m.runRenewOperation(context.Background(), opID)
}

func (m *Manager) runRenewOperation(ctx context.Context, opID string) {
	if _, loaded := renewInflight.LoadOrStore(opID, struct{}{}); loaded {
		return
	}
	defer renewInflight.Delete(opID)

	op, err := m.store.ClaimPendingRenewOperation(ctx, opID)
	if err != nil {
		return
	}

	warning, runErr := m.executeCertificateIssuance(ctx, op)
	if runErr == nil {
		return
	}
	_ = warning
	_ = m.store.FailRenewOperation(ctx, opID, sanitizeIssueError(runErr))
}

func (m *Manager) reconcileAutoRenew(ctx context.Context) {
	certs, err := m.store.ListAutoRenewManagedCertificates(ctx)
	if err != nil {
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
			continue
		}
		if err := m.ensureNoConflictingIssuance(ctx, cert.ID, store.CertOpTypeRenew); err != nil {
			continue
		}
		op, err := m.createPendingRenewOperation(ctx, cert)
		if err != nil {
			continue
		}
		m.triggerRenewOperation(op.ID)
	}
}
