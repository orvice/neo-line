package certmanager

import (
	"context"
	"errors"

	"github.com/orvice/neo-line/internal/store"
)

type noopIssueStore struct{}

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
func (noopIssueStore) ActivateFirstIssueVersion(context.Context, string, store.CertificateVersion, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ActivateSubsequentIssueVersion(context.Context, string, store.CertificateVersion, string, string, string) error {
	return errors.New("not implemented")
}
func (noopIssueStore) ActivatePreviousVersion(context.Context, string, string) error {
	return errors.New("not implemented")
}
