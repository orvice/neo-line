package certmanager

import (
	"context"
	"encoding/pem"
	"fmt"

	"github.com/orvice/neo-line/internal/store"
)

const maxRevokeReason = 10

// SubmitRevokeVersion accepts a revoke for active or previous, immediately blocks
// distribution, and enqueues a persistent Revoke operation.
func (m *Manager) SubmitRevokeVersion(ctx context.Context, managedCertificateID, versionID string, reason uint32) (PublicOperation, error) {
	if reason > maxRevokeReason {
		return PublicOperation{}, ErrInvalidRevocationReason
	}
	if existing, err := m.store.FindRunningCertificateOperation(ctx, managedCertificateID, store.CertOpTypeRevoke); err == nil {
		return publicOperationFromStore(existing), nil
	} else if !store.IsNotFound(err) {
		return PublicOperation{}, err
	}
	if err := m.ensureNoConflictingIssuance(ctx, managedCertificateID, store.CertOpTypeRevoke); err != nil {
		return PublicOperation{}, err
	}
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		return PublicOperation{}, err
	}
	target, err := versionByID(cert, versionID)
	if err != nil {
		return PublicOperation{}, err
	}
	if target.RevokedAt != nil {
		return PublicOperation{}, store.ErrVersionRevoked
	}
	if target.RevokePending {
		return PublicOperation{}, store.ErrVersionRevokePending
	}
	if err := m.store.MarkVersionRevokePending(ctx, managedCertificateID, versionID); err != nil {
		return PublicOperation{}, err
	}
	op, err := m.createPendingRevokeOperation(ctx, cert, versionID, reason, target)
	if err != nil {
		_ = m.store.ClearVersionRevokePending(ctx, managedCertificateID, versionID)
		return PublicOperation{}, err
	}
	m.triggerRevokeOperation(op.ID)
	return publicOperationFromStore(op), nil
}

func versionByID(cert store.ManagedCertificate, versionID string) (store.CertificateVersion, error) {
	if cert.ActiveVersion != nil && cert.ActiveVersion.ID == versionID {
		return *cert.ActiveVersion, nil
	}
	if cert.PreviousVersion != nil && cert.PreviousVersion.ID == versionID {
		return *cert.PreviousVersion, nil
	}
	return store.CertificateVersion{}, store.ErrVersionNotFound
}

func (m *Manager) createPendingRevokeOperation(ctx context.Context, cert store.ManagedCertificate, versionID string, reason uint32, target store.CertificateVersion) (store.CertificateOperation, error) {
	if existing, err := m.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeRevoke); err == nil {
		return existing, nil
	} else if !store.IsNotFound(err) {
		return store.CertificateOperation{}, err
	}
	if running, err := m.store.HasRunningCertificateOperation(ctx, cert.ID); err != nil {
		return store.CertificateOperation{}, err
	} else if running {
		return store.CertificateOperation{}, ErrIssuanceOperationInFlight
	}
	snapshot := target.ConfigSnapshot
	if len(snapshot.Domains) == 0 {
		snapshot = store.IssueConfigSnapshot{
			Domains:              append([]string(nil), cert.Domains...),
			CertificateIssuerID:  cert.CertificateIssuerID,
			DNSProviderAccountID: cert.DNSProviderAccountID,
			KeyType:              cert.KeyType,
		}
	}
	return m.store.CreateCertificateOperation(ctx, store.CertificateOperation{
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeRevoke,
		Status:               store.CertOpStatusPending,
		AttemptCount:         0,
		ConfigSnapshot:       snapshot,
		TargetVersionID:      versionID,
		RevokeReason:         reason,
	})
}

func (m *Manager) triggerRevokeOperation(opID string) {
	m.triggerOperation(opID)
}

func (m *Manager) executeCertificateRevocation(ctx context.Context, op store.CertificateOperation, leaseOwner string) error {
	cert, err := m.store.GetManagedCertificate(ctx, op.ManagedCertificateID)
	if err != nil {
		return err
	}
	target, err := versionByID(cert, op.TargetVersionID)
	if err != nil {
		return err
	}
	if target.RevokedAt != nil {
		return m.store.CompleteRevokeVersion(ctx, op.ManagedCertificateID, op.TargetVersionID, op.ID, leaseOwner, *target.RevokedAt)
	}
	issuerID := target.CertificateIssuerID
	if issuerID == "" {
		issuerID = target.ConfigSnapshot.CertificateIssuerID
	}
	if issuerID == "" {
		issuerID = cert.CertificateIssuerID
	}
	issuer, err := m.store.GetCertificateIssuer(ctx, issuerID)
	if err != nil {
		return fmt.Errorf("resolve issuer: %w", err)
	}
	if issuer.RegistrationStatus != store.IssuerRegistrationReady {
		return ErrIssuerNotReady
	}
	leafPEM, err := leafCertificatePEM([]byte(target.FullchainPEM))
	if err != nil {
		return err
	}
	var reasonPtr *uint
	if op.RevokeReason != 0 {
		r := uint(op.RevokeReason)
		reasonPtr = &r
	}
	if err := m.acme.RevokeCertificate(ctx, issuer, leafPEM, reasonPtr); err != nil {
		return err
	}
	return m.store.CompleteRevokeVersion(ctx, op.ManagedCertificateID, op.TargetVersionID, op.ID, leaseOwner, m.clock.Now())
}

func leafCertificatePEM(fullchain []byte) ([]byte, error) {
	block, _ := pem.Decode(fullchain)
	if block == nil {
		return nil, ErrInvalidCertificatePEM
	}
	return pem.EncodeToMemory(block), nil
}

func sanitizeRevokeError(err error) string {
	return sanitizeIssueError(err)
}
