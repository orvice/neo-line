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
func (noopIssueStore) UpdateCertificateOperationPendingTXT(context.Context, string, string, []store.DNSChallengeRecord) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ScheduleCertificateOperationRetry(context.Context, string, string, time.Time, string, uint32) error {
	return errors.New("not implemented")
}
func (noopIssueStore) MarkCertificateOperationFailed(context.Context, string, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ClearCertificateOperationPendingTXT(context.Context, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ClaimPendingRenewOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (noopIssueStore) FailRenewOperation(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) FindPendingRenewOperations(context.Context, int64) ([]store.CertificateOperation, error) {
	return nil, nil
}
func (noopIssueStore) ListAutoRenewManagedCertificates(context.Context) ([]store.ManagedCertificate, error) {
	return nil, nil
}
func (noopIssueStore) ClaimPendingIssueOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (noopIssueStore) FailIssueOperation(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) FindPendingIssueOperations(context.Context, int64) ([]store.CertificateOperation, error) {
	return nil, nil
}
func (noopIssueStore) UpdateCertificateOperation(context.Context, string, store.CertificateOperation) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
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
