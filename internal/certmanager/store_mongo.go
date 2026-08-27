package certmanager

import (
	"context"
	"time"

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

func (s *mongoStore) UpdateManagedCertificate(ctx context.Context, id string, update store.ManagedCertificateUpdate) (store.ManagedCertificate, error) {
	return s.st.UpdateManagedCertificate(ctx, id, update)
}

func (s *mongoStore) DeleteManagedCertificate(ctx context.Context, id string) error {
	return s.st.DeleteManagedCertificate(ctx, id)
}

func (s *mongoStore) MarkVersionRevokePending(ctx context.Context, managedCertID, versionID string) error {
	return s.st.MarkVersionRevokePending(ctx, managedCertID, versionID)
}

func (s *mongoStore) ClearVersionRevokePending(ctx context.Context, managedCertID, versionID string) error {
	return s.st.ClearVersionRevokePending(ctx, managedCertID, versionID)
}

func (s *mongoStore) CompleteRevokeVersion(ctx context.Context, managedCertID, versionID, opID, leaseOwner string, revokedAt time.Time) error {
	return s.st.CompleteRevokeVersion(ctx, managedCertID, versionID, opID, leaseOwner, revokedAt)
}

func (s *mongoStore) CountManagedCertificatesReferencingIssuer(ctx context.Context, issuerID string) (int64, error) {
	return s.st.CountManagedCertificatesReferencingIssuer(ctx, issuerID)
}

func (s *mongoStore) CountManagedCertificatesReferencingDNSAccount(ctx context.Context, dnsID string) (int64, error) {
	return s.st.CountManagedCertificatesReferencingDNSAccount(ctx, dnsID)
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

func (s *mongoStore) HasRunningCertificateOperation(ctx context.Context, managedCertificateID string) (bool, error) {
	return s.st.HasRunningCertificateOperation(ctx, managedCertificateID)
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

func (s *mongoStore) FindClaimableCertificateOperations(ctx context.Context, now time.Time, limit int64) ([]store.CertificateOperation, error) {
	return s.st.FindClaimableCertificateOperations(ctx, now, limit)
}

func (s *mongoStore) TryClaimCertificateOperation(ctx context.Context, p store.CertificateOperationClaimParams) (store.CertificateOperation, error) {
	return s.st.TryClaimCertificateOperation(ctx, p)
}

func (s *mongoStore) RenewCertificateOperationLease(ctx context.Context, opID, owner string, leaseExpires, now time.Time) error {
	return s.st.RenewCertificateOperationLease(ctx, opID, owner, leaseExpires, now)
}

func (s *mongoStore) RecordCertificateOperationPendingTXT(ctx context.Context, opID, owner string, record store.DNSChallengeRecord) error {
	return s.st.RecordCertificateOperationPendingTXT(ctx, opID, owner, record)
}

func (s *mongoStore) ScheduleCertificateOperationRetry(ctx context.Context, opID, owner string, nextAttemptAt time.Time, errorSummary string, consecutiveFailures uint32) error {
	return s.st.ScheduleCertificateOperationRetry(ctx, opID, owner, nextAttemptAt, errorSummary, consecutiveFailures)
}

func (s *mongoStore) MarkCertificateOperationFailed(ctx context.Context, opID, owner, errorSummary string) error {
	return s.st.MarkCertificateOperationFailed(ctx, opID, owner, errorSummary)
}

func (s *mongoStore) FailExpiredCertificateOperations(ctx context.Context, now time.Time) (int64, error) {
	return s.st.FailExpiredCertificateOperations(ctx, now)
}

func (s *mongoStore) ClearCertificateOperationPendingTXT(ctx context.Context, opID string) error {
	return s.st.ClearCertificateOperationPendingTXT(ctx, opID)
}

func (s *mongoStore) ListAutoRenewManagedCertificates(ctx context.Context) ([]store.ManagedCertificate, error) {
	return s.st.ListAutoRenewManagedCertificates(ctx)
}

func (s *mongoStore) ActivateFirstIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, opID, leaseOwner, warning string) error {
	return s.st.ActivateFirstIssueVersion(ctx, managedCertID, version, opID, leaseOwner, warning)
}

func (s *mongoStore) ActivateSubsequentIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, expectedActiveID, opID, leaseOwner, warning string) error {
	return s.st.ActivateSubsequentIssueVersion(ctx, managedCertID, version, expectedActiveID, opID, leaseOwner, warning)
}

func (s *mongoStore) ActivatePreviousVersion(ctx context.Context, managedCertID, versionID string) error {
	return s.st.ActivatePreviousVersion(ctx, managedCertID, versionID)
}

func (s *mongoStore) ListManagedCertificatesForNotifications(ctx context.Context) ([]store.ManagedCertificate, error) {
	return s.st.ListManagedCertificatesForNotifications(ctx)
}

func (s *mongoStore) TryRecordOperationFailureNotification(ctx context.Context, certID string, now time.Time) (bool, error) {
	return s.st.TryRecordOperationFailureNotification(ctx, certID, now)
}

func (s *mongoStore) TryRecordOperationFailureReminder(ctx context.Context, certID string, now time.Time) (bool, error) {
	return s.st.TryRecordOperationFailureReminder(ctx, certID, now)
}

func (s *mongoStore) TryRecordOperationRecovery(ctx context.Context, certID string, now time.Time) (bool, error) {
	return s.st.TryRecordOperationRecovery(ctx, certID, now)
}

func (s *mongoStore) TryRecordSevenDayReminder(ctx context.Context, certID, versionID string, now time.Time) (bool, error) {
	return s.st.TryRecordSevenDayReminder(ctx, certID, versionID, now)
}

func (s *mongoStore) TryRecordExpiredNotification(ctx context.Context, certID, versionID string, now time.Time) (bool, error) {
	return s.st.TryRecordExpiredNotification(ctx, certID, versionID, now)
}

func (s *mongoStore) SetCertificateNotificationWarning(ctx context.Context, certID, warning string, at time.Time) error {
	return s.st.SetCertificateNotificationWarning(ctx, certID, warning, at)
}

func (s *mongoStore) GetNotifyGroup(ctx context.Context, id string) (store.NotifyGroup, error) {
	return s.st.GetNotifyGroup(ctx, id)
}
