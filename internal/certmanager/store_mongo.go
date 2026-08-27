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
