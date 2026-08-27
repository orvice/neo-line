package certmanager

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

type fakeStore struct {
	noopIssueStore
	accounts map[string]store.DNSProviderAccount
	order    []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{accounts: make(map[string]store.DNSProviderAccount)}
}

func (f *fakeStore) ListDNSProviderAccounts(_ context.Context, _ int64, _ string) ([]store.DNSProviderAccount, string, error) {
	out := make([]store.DNSProviderAccount, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, f.accounts[id])
	}
	return out, "", nil
}

func (f *fakeStore) CreateDNSProviderAccount(_ context.Context, account store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	if account.ID == "" {
		account.ID = "dns_test"
	}
	for _, a := range f.accounts {
		if a.Name == account.Name {
			return store.DNSProviderAccount{}, store.ErrDNSProviderAccountNameTaken
		}
	}
	f.accounts[account.ID] = account
	f.order = append(f.order, account.ID)
	return account, nil
}

func (f *fakeStore) GetDNSProviderAccount(_ context.Context, id string) (store.DNSProviderAccount, error) {
	a, ok := f.accounts[id]
	if !ok {
		return store.DNSProviderAccount{}, errors.New("not found")
	}
	return a, nil
}

func (f *fakeStore) UpdateDNSProviderAccount(_ context.Context, id string, account store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	if _, ok := f.accounts[id]; !ok {
		return store.DNSProviderAccount{}, errors.New("not found")
	}
	account.ID = id
	f.accounts[id] = account
	return account, nil
}

func (f *fakeStore) DeleteDNSProviderAccount(_ context.Context, id string) error {
	if _, ok := f.accounts[id]; !ok {
		return errors.New("not found")
	}
	delete(f.accounts, id)
	return nil
}

func (f *fakeStore) ListCertificateIssuers(context.Context, int64, string) ([]store.CertificateIssuer, string, error) {
	return nil, "", nil
}
func (f *fakeStore) CreateCertificateIssuer(context.Context, store.CertificateIssuer) (store.CertificateIssuer, error) {
	return store.CertificateIssuer{}, errors.New("not implemented")
}
func (f *fakeStore) GetCertificateIssuer(context.Context, string) (store.CertificateIssuer, error) {
	return store.CertificateIssuer{}, errors.New("not implemented")
}
func (f *fakeStore) UpdateCertificateIssuer(context.Context, string, store.CertificateIssuer) (store.CertificateIssuer, error) {
	return store.CertificateIssuer{}, errors.New("not implemented")
}
func (f *fakeStore) DeleteCertificateIssuer(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeStore) ListManagedCertificates(context.Context, int64, string) ([]store.ManagedCertificate, string, error) {
	return nil, "", nil
}
func (f *fakeStore) ListManagedCertificatesByServer(context.Context, string) ([]store.ManagedCertificate, error) {
	return nil, nil
}
func (f *fakeStore) CreateManagedCertificate(context.Context, store.ManagedCertificate) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeStore) GetManagedCertificate(context.Context, string) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeStore) UpdateManagedCertificate(context.Context, string, store.ManagedCertificate) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeStore) CreateCertificateOperation(context.Context, store.CertificateOperation) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeStore) GetCertificateOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeStore) FindRunningCertificateOperation(context.Context, string, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeStore) ListCertificateOperationsByCertificate(context.Context, string, int64) ([]store.CertificateOperation, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeStore) LatestCertificateOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeStore) ValidateNotifyGroupIDs(context.Context, []string) error {
	return nil
}
func (f *fakeStore) ValidateServerIDs(context.Context, []string) error {
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestManagerCreateVerifyFailDoesNotSave(t *testing.T) {
	st := newFakeStore()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/tokens/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bad-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Invalid API Token"}]}`))
	}))
	defer srv.Close()

	client := NewCloudflareClient(srv.Client())
	client.baseURL = srv.URL

	m := NewManager(st, client)
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}

	_, err := m.CreateDNSProviderAccount(context.Background(), DNSAccountInput{
		Name:     "prod",
		Provider: store.DNSProviderCloudflare,
		APIToken: "bad-token",
	})
	if err == nil {
		t.Fatal("expected verify error")
	}
	if !errors.Is(err, ErrCloudflareTokenInvalid) {
		t.Fatalf("expected ErrCloudflareTokenInvalid, got %v", err)
	}
	if len(st.accounts) != 0 {
		t.Fatalf("expected no saved account, got %d", len(st.accounts))
	}
}

func TestManagerCreateVerifySuccessSaves(t *testing.T) {
	st := newFakeStore()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	client := NewCloudflareClient(srv.Client())
	client.baseURL = srv.URL

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	m := NewManager(st, client)
	m.clock = fixedClock{t: now}

	got, err := m.CreateDNSProviderAccount(context.Background(), DNSAccountInput{
		Name:     "prod",
		Provider: store.DNSProviderCloudflare,
		APIToken: "good-token",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !got.TokenConfigured {
		t.Fatal("expected token_configured true")
	}
	if got.TokenLastVerifiedAt == nil || !got.TokenLastVerifiedAt.Equal(now) {
		t.Fatalf("token_last_verified_at = %v, want %v", got.TokenLastVerifiedAt, now)
	}
	raw, _ := st.GetDNSProviderAccount(context.Background(), got.ID)
	if raw.APIToken != "good-token" {
		t.Fatalf("stored token = %q", raw.APIToken)
	}
}

func TestManagerRotateVerifyFailKeepsOldToken(t *testing.T) {
	st := newFakeStore()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	verified := now.Add(-time.Hour)
	st.accounts["dns_1"] = store.DNSProviderAccount{
		ID:                        "dns_1",
		Name:                      "prod",
		Provider:                  store.DNSProviderCloudflare,
		PropagationTimeoutSeconds: 120,
		APIToken:                  "old-token",
		TokenLastVerifiedAt:       &verified,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	st.order = []string{"dns_1"}

	verifyCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyCalls++
		if strings.Contains(r.Header.Get("Authorization"), "new-bad") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"Invalid API Token"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	client := NewCloudflareClient(srv.Client())
	client.baseURL = srv.URL
	m := NewManager(st, client)
	m.clock = fixedClock{t: now}

	_, err := m.UpdateDNSProviderAccount(context.Background(), "dns_1", DNSAccountInput{
		Name:     "prod-renamed",
		Provider: store.DNSProviderCloudflare,
		APIToken: "new-bad",
	})
	if err == nil {
		t.Fatal("expected rotate verify error")
	}
	raw, _ := st.GetDNSProviderAccount(context.Background(), "dns_1")
	if raw.APIToken != "old-token" {
		t.Fatalf("token after failed rotate = %q, want old-token", raw.APIToken)
	}
	if raw.Name != "prod" {
		t.Fatalf("name after failed rotate = %q, want prod unchanged", raw.Name)
	}
	if verifyCalls != 1 {
		t.Fatalf("verify calls = %d, want 1", verifyCalls)
	}
}

func TestManagerPublicResponsesOmitToken(t *testing.T) {
	st := newFakeStore()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	st.accounts["dns_1"] = store.DNSProviderAccount{
		ID:                        "dns_1",
		Name:                      "prod",
		Provider:                  store.DNSProviderCloudflare,
		PropagationTimeoutSeconds: 120,
		APIToken:                  "secret-token-value",
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	st.order = []string{"dns_1"}

	m := NewManager(st, &CloudflareClient{client: http.DefaultClient})

	got, err := m.GetDNSProviderAccount(context.Background(), "dns_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(got.Name, "secret-token-value") {
		t.Fatal("response leaked token in name")
	}
	if !got.TokenConfigured {
		t.Fatal("expected token_configured true")
	}

	list, _, err := m.ListDNSProviderAccounts(context.Background(), 50, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !list[0].TokenConfigured {
		t.Fatalf("list = %+v", list)
	}
}

func TestCloudflareClientVerifySuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"Invalid API Token"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	client := NewCloudflareClient(srv.Client())
	client.baseURL = srv.URL

	if err := client.VerifyCloudflareToken(context.Background(), "ok"); err != nil {
		t.Fatalf("verify ok: %v", err)
	}
	if err := client.VerifyCloudflareToken(context.Background(), "bad"); err == nil {
		t.Fatal("expected verify failure")
	} else if !errors.Is(err, ErrCloudflareTokenInvalid) {
		t.Fatalf("expected ErrCloudflareTokenInvalid, got %v", err)
	}
}
