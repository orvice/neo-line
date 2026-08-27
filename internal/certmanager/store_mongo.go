package certmanager

import (
	"context"

	"github.com/orvice/neo-line/internal/store"
)

// mongoStore adapts the application store to certmanager's narrow Store
// interface.
type mongoStore struct {
	st store.Store
}

func NewStore(st store.Store) Store {
	return &mongoStore{st: st}
}

func (s *mongoStore) ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]store.DNSProviderAccount, string, error) {
	return s.st.ListDNSProviderAccounts(ctx, limit, pageToken)
}

func (s *mongoStore) CreateDNSProviderAccount(ctx context.Context, account store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return s.st.CreateDNSProviderAccount(ctx, account)
}

func (s *mongoStore) GetDNSProviderAccount(ctx context.Context, id string) (store.DNSProviderAccount, error) {
	return s.st.GetDNSProviderAccount(ctx, id)
}

func (s *mongoStore) UpdateDNSProviderAccount(ctx context.Context, id string, account store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return s.st.UpdateDNSProviderAccount(ctx, id, account)
}

func (s *mongoStore) DeleteDNSProviderAccount(ctx context.Context, id string) error {
	return s.st.DeleteDNSProviderAccount(ctx, id)
}

func (s *mongoStore) ListCertificateIssuers(ctx context.Context, limit int64, pageToken string) ([]store.CertificateIssuer, string, error) {
	return s.st.ListCertificateIssuers(ctx, limit, pageToken)
}

func (s *mongoStore) CreateCertificateIssuer(ctx context.Context, issuer store.CertificateIssuer) (store.CertificateIssuer, error) {
	return s.st.CreateCertificateIssuer(ctx, issuer)
}

func (s *mongoStore) GetCertificateIssuer(ctx context.Context, id string) (store.CertificateIssuer, error) {
	return s.st.GetCertificateIssuer(ctx, id)
}

func (s *mongoStore) UpdateCertificateIssuer(ctx context.Context, id string, issuer store.CertificateIssuer) (store.CertificateIssuer, error) {
	return s.st.UpdateCertificateIssuer(ctx, id, issuer)
}

func (s *mongoStore) DeleteCertificateIssuer(ctx context.Context, id string) error {
	return s.st.DeleteCertificateIssuer(ctx, id)
}

func (s *mongoStore) ListManagedCertificates(ctx context.Context, limit int64, pageToken string) ([]store.ManagedCertificate, string, error) {
	return s.st.ListManagedCertificates(ctx, limit, pageToken)
}

func (s *mongoStore) ListManagedCertificatesByServer(ctx context.Context, serverID string) ([]store.ManagedCertificate, error) {
	return s.st.ListManagedCertificatesByServer(ctx, serverID)
}

func (s *mongoStore) CreateManagedCertificate(ctx context.Context, cert store.ManagedCertificate) (store.ManagedCertificate, error) {
	return s.st.CreateManagedCertificate(ctx, cert)
}

func (s *mongoStore) GetManagedCertificate(ctx context.Context, id string) (store.ManagedCertificate, error) {
	return s.st.GetManagedCertificate(ctx, id)
}

func (s *mongoStore) UpdateManagedCertificate(ctx context.Context, id string, cert store.ManagedCertificate) (store.ManagedCertificate, error) {
	return s.st.UpdateManagedCertificate(ctx, id, cert)
}

func (s *mongoStore) CreateCertificateOperation(ctx context.Context, op store.CertificateOperation) (store.CertificateOperation, error) {
	return s.st.CreateCertificateOperation(ctx, op)
}

func (s *mongoStore) GetCertificateOperation(ctx context.Context, id string) (store.CertificateOperation, error) {
	return s.st.GetCertificateOperation(ctx, id)
}

func (s *mongoStore) FindRunningCertificateOperation(ctx context.Context, managedCertificateID, opType string) (store.CertificateOperation, error) {
	return s.st.FindRunningCertificateOperation(ctx, managedCertificateID, opType)
}

func (s *mongoStore) ListCertificateOperationsByCertificate(ctx context.Context, managedCertificateID string, limit int64) ([]store.CertificateOperation, error) {
	return s.st.ListCertificateOperationsByCertificate(ctx, managedCertificateID, limit)
}

func (s *mongoStore) LatestCertificateOperation(ctx context.Context, managedCertificateID string) (store.CertificateOperation, error) {
	return s.st.LatestCertificateOperation(ctx, managedCertificateID)
}

func (s *mongoStore) ValidateNotifyGroupIDs(ctx context.Context, ids []string) error {
	return s.st.ValidateNotifyGroupIDs(ctx, ids)
}

func (s *mongoStore) ValidateServerIDs(ctx context.Context, ids []string) error {
	return s.st.ValidateServerIDs(ctx, ids)
}

func (s *mongoStore) ClaimPendingIssueOperation(ctx context.Context, opID string) (store.CertificateOperation, error) {
	return s.st.ClaimPendingIssueOperation(ctx, opID)
}

func (s *mongoStore) FailIssueOperation(ctx context.Context, opID, errorSummary string) error {
	return s.st.FailIssueOperation(ctx, opID, errorSummary)
}

func (s *mongoStore) FindPendingIssueOperations(ctx context.Context, limit int64) ([]store.CertificateOperation, error) {
	return s.st.FindPendingIssueOperations(ctx, limit)
}

func (s *mongoStore) ClaimPendingRenewOperation(ctx context.Context, opID string) (store.CertificateOperation, error) {
	return s.st.ClaimPendingRenewOperation(ctx, opID)
}

func (s *mongoStore) FailRenewOperation(ctx context.Context, opID, errorSummary string) error {
	return s.st.FailRenewOperation(ctx, opID, errorSummary)
}

func (s *mongoStore) FindPendingRenewOperations(ctx context.Context, limit int64) ([]store.CertificateOperation, error) {
	return s.st.FindPendingRenewOperations(ctx, limit)
}

func (s *mongoStore) ListAutoRenewManagedCertificates(ctx context.Context) ([]store.ManagedCertificate, error) {
	return s.st.ListAutoRenewManagedCertificates(ctx)
}

func (s *mongoStore) UpdateCertificateOperation(ctx context.Context, id string, op store.CertificateOperation) (store.CertificateOperation, error) {
	return s.st.UpdateCertificateOperation(ctx, id, op)
}

func (s *mongoStore) ActivateFirstIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, opID, warning string) error {
	return s.st.ActivateFirstIssueVersion(ctx, managedCertID, version, opID, warning)
}

func (s *mongoStore) ActivateSubsequentIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, expectedActiveID, opID, warning string) error {
	return s.st.ActivateSubsequentIssueVersion(ctx, managedCertID, version, expectedActiveID, opID, warning)
}

func (s *mongoStore) ActivatePreviousVersion(ctx context.Context, managedCertID, versionID string) error {
	return s.st.ActivatePreviousVersion(ctx, managedCertID, versionID)
}
