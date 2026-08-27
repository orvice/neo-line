package certmanager

import (
	"context"
	"errors"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

type noopIssueStore struct{}

func (noopIssueStore) HasRunningCertificateOperation(context.Context, string) (bool, error) {
	return false, nil
}
func (noopIssueStore) FindClaimableCertificateOperations(context.Context, time.Time, int64) ([]store.CertificateOperation, error) {
	return nil, nil
}
func (noopIssueStore) TryClaimCertificateOperation(context.Context, store.CertificateOperationClaimParams) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (noopIssueStore) RenewCertificateOperationLease(context.Context, string, string, time.Time, time.Time) error {
	return errors.New("not implemented")
}
func (noopIssueStore) RecordCertificateOperationPendingTXT(context.Context, string, string, store.DNSChallengeRecord) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ScheduleCertificateOperationRetry(context.Context, string, string, time.Time, string, uint32) error {
	return errors.New("not implemented")
}
func (noopIssueStore) MarkCertificateOperationFailed(context.Context, string, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) FailExpiredCertificateOperations(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (noopIssueStore) ClearCertificateOperationPendingTXT(context.Context, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ListAutoRenewManagedCertificates(context.Context) ([]store.ManagedCertificate, error) {
	return nil, nil
}
func (noopIssueStore) ActivateFirstIssueVersion(context.Context, string, store.CertificateVersion, string, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ActivateSubsequentIssueVersion(context.Context, string, store.CertificateVersion, string, string, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ActivatePreviousVersion(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ListManagedCertificatesForNotifications(context.Context) ([]store.ManagedCertificate, error) {
	return nil, nil
}
func (noopIssueStore) TryRecordOperationFailureNotification(context.Context, string, time.Time) (bool, error) {
	return false, nil
}
func (noopIssueStore) TryRecordOperationFailureReminder(context.Context, string, time.Time) (bool, error) {
	return false, nil
}
func (noopIssueStore) TryRecordOperationRecovery(context.Context, string, time.Time) (bool, error) {
	return false, nil
}
func (noopIssueStore) TryRecordSevenDayReminder(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}
func (noopIssueStore) TryRecordExpiredNotification(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}
func (noopIssueStore) SetCertificateNotificationWarning(context.Context, string, string, time.Time) error {
	return nil
}
func (noopIssueStore) GetNotifyGroup(context.Context, string) (store.NotifyGroup, error) {
	return store.NotifyGroup{}, errors.New("not implemented")
}
func (noopIssueStore) DeleteManagedCertificate(context.Context, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) MarkVersionRevokePending(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ClearVersionRevokePending(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) CompleteRevokeVersion(context.Context, string, string, string, string, time.Time) error {
	return errors.New("not implemented")
}
func (noopIssueStore) CountManagedCertificatesReferencingIssuer(context.Context, string) (int64, error) {
	return 0, nil
}
func (noopIssueStore) CountManagedCertificatesReferencingDNSAccount(context.Context, string) (int64, error) {
	return 0, nil
}
