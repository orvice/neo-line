package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

type fakeIssuerStore struct {
	mu      sync.Mutex
	issuers map[string]store.CertificateIssuer
	order   []string
}

func newFakeIssuerStore() *fakeIssuerStore {
	return &fakeIssuerStore{issuers: make(map[string]store.CertificateIssuer)}
}

func (f *fakeIssuerStore) ListDNSProviderAccounts(context.Context, int64, string) ([]store.DNSProviderAccount, string, error) {
	return nil, "", nil
}
func (f *fakeIssuerStore) CreateDNSProviderAccount(context.Context, store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return store.DNSProviderAccount{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) GetDNSProviderAccount(context.Context, string) (store.DNSProviderAccount, error) {
	return store.DNSProviderAccount{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) UpdateDNSProviderAccount(context.Context, string, store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return store.DNSProviderAccount{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) DeleteDNSProviderAccount(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *fakeIssuerStore) ListCertificateIssuers(_ context.Context, _ int64, _ string) ([]store.CertificateIssuer, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.CertificateIssuer, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, f.issuers[id])
	}
	return out, "", nil
}

func (f *fakeIssuerStore) CreateCertificateIssuer(_ context.Context, issuer store.CertificateIssuer) (store.CertificateIssuer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if issuer.ID == "" {
		issuer.ID = "iss_test"
	}
	for _, existing := range f.issuers {
		if existing.Name == issuer.Name {
			return store.CertificateIssuer{}, store.ErrCertificateIssuerNameTaken
		}
	}
	f.issuers[issuer.ID] = issuer
	f.order = append(f.order, issuer.ID)
	return issuer, nil
}

func (f *fakeIssuerStore) GetCertificateIssuer(_ context.Context, id string) (store.CertificateIssuer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.issuers[id]
	if !ok {
		return store.CertificateIssuer{}, errors.New("not found")
	}
	return i, nil
}

func (f *fakeIssuerStore) UpdateCertificateIssuer(_ context.Context, id string, issuer store.CertificateIssuer) (store.CertificateIssuer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.issuers[id]; !ok {
		return store.CertificateIssuer{}, errors.New("not found")
	}
	issuer.ID = id
	f.issuers[id] = issuer
	return issuer, nil
}

func (f *fakeIssuerStore) DeleteCertificateIssuer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.issuers[id]; !ok {
		return errors.New("not found")
	}
	delete(f.issuers, id)
	return nil
}

func (f *fakeIssuerStore) ListManagedCertificates(context.Context, int64, string) ([]store.ManagedCertificate, string, error) {
	return nil, "", nil
}
func (f *fakeIssuerStore) CreateManagedCertificate(context.Context, store.ManagedCertificate) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) GetManagedCertificate(context.Context, string) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) UpdateManagedCertificate(context.Context, string, store.ManagedCertificate) (store.ManagedCertificate, error) {
	return store.ManagedCertificate{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) CreateCertificateOperation(context.Context, store.CertificateOperation) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) GetCertificateOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not implemented")
}
func (f *fakeIssuerStore) FindRunningCertificateOperation(context.Context, string, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not found")
}
func (f *fakeIssuerStore) ListCertificateOperationsByCertificate(context.Context, string, int64) ([]store.CertificateOperation, error) {
	return nil, nil
}
func (f *fakeIssuerStore) LatestCertificateOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, errors.New("not found")
}
func (f *fakeIssuerStore) ValidateNotifyGroupIDs(context.Context, []string) error { return nil }
func (f *fakeIssuerStore) ValidateServerIDs(context.Context, []string) error      { return nil }

func startFakeACMEServer(t *testing.T, failRegister bool) *httptest.Server {
	t.Helper()
	var nonceCounter int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/new-nonce"):
			nonceCounter++
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonceCounter))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/directory"):
			base := "https://" + r.Host
			_, _ = w.Write([]byte(`{
				"newNonce":"` + base + `/new-nonce",
				"newAccount":"` + base + `/new-account",
				"newOrder":"` + base + `/new-order",
				"meta":{"termsOfService":"https://example.com/tos"}
			}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/new-account"):
			if failRegister {
				http.Error(w, `{"type":"urn:ietf:params:acme:error:malformed","detail":"bad eab"}`, http.StatusBadRequest)
				return
			}
			nonceCounter++
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonceCounter))
			w.Header().Set("Location", "https://"+r.Host+"/account/1")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"valid","contact":["mailto:test@example.com"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testACMEClient(t *testing.T, srv *httptest.Server) *LegoACMEClient {
	t.Helper()
	client := NewLegoACMEClient(srv.Client())
	client.httpClient = srv.Client()
	return client
}

func waitForIssuerStatus(t *testing.T, st *fakeIssuerStore, id, want string) store.CertificateIssuer {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		i, err := st.GetCertificateIssuer(context.Background(), id)
		if err == nil && i.RegistrationStatus == want {
			return i
		}
		time.Sleep(10 * time.Millisecond)
	}
	i, _ := st.GetCertificateIssuer(context.Background(), id)
	t.Fatalf("issuer %q status = %q, want %q", id, i.RegistrationStatus, want)
	return i
}

func TestIssuerCreateRequiresTOSAgreement(t *testing.T) {
	st := newFakeIssuerStore()
	srv := startFakeACMEServer(t, false)
	m := NewManagerWithACME(st, nil, testACMEClient(t, srv))
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}

	_, err := m.CreateCertificateIssuer(context.Background(), IssuerInput{
		Name:                 "prod-le",
		CAType:               store.CATypeCustom,
		CustomDirectoryURL:   srv.URL + "/directory",
		Email:                "admin@example.com",
		TermsOfServiceAgreed: false,
	})
	if !errors.Is(err, ErrTermsOfServiceRequired) {
		t.Fatalf("expected ErrTermsOfServiceRequired, got %v", err)
	}
}

func TestIssuerRegistrationSuccessBecomesReady(t *testing.T) {
	st := newFakeIssuerStore()
	srv := startFakeACMEServer(t, false)
	m := NewManagerWithACME(st, nil, testACMEClient(t, srv))
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}

	got, err := m.CreateCertificateIssuer(context.Background(), IssuerInput{
		Name:                 "prod-le",
		CAType:               store.CATypeCustom,
		CustomDirectoryURL:   srv.URL + "/directory",
		Email:                "admin@example.com",
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.RegistrationStatus != store.IssuerRegistrationPending {
		t.Fatalf("initial status = %q, want Pending", got.RegistrationStatus)
	}
	final := waitForIssuerStatus(t, st, got.ID, store.IssuerRegistrationReady)
	if final.RegistrationError != "" {
		t.Fatalf("registration_error = %q", final.RegistrationError)
	}
	if final.TermsOfServiceURL == "" {
		t.Fatal("expected terms_of_service_url persisted")
	}
}

func TestIssuerRegistrationFailureAndRetry(t *testing.T) {
	st := newFakeIssuerStore()
	srv := startFakeACMEServer(t, true)
	m := NewManagerWithACME(st, nil, testACMEClient(t, srv))
	m.clock = fixedClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}

	got, err := m.CreateCertificateIssuer(context.Background(), IssuerInput{
		Name:                 "fail-le",
		CAType:               store.CATypeCustom,
		CustomDirectoryURL:   srv.URL + "/directory",
		Email:                "admin@example.com",
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	failed := waitForIssuerStatus(t, st, got.ID, store.IssuerRegistrationFailed)
	if failed.RegistrationError == "" {
		t.Fatal("expected registration_error")
	}

	// Switch server to succeed and retry.
	srv.Close()
	srvOK := startFakeACMEServer(t, false)
	failed.DirectoryURL = srvOK.URL + "/directory"
	_, _ = st.UpdateCertificateIssuer(context.Background(), got.ID, failed)

	retried, err := m.RetryCertificateIssuerRegistration(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.RegistrationStatus != store.IssuerRegistrationPending {
		t.Fatalf("retry status = %q", retried.RegistrationStatus)
	}
	waitForIssuerStatus(t, st, got.ID, store.IssuerRegistrationReady)
}

func TestIssuerReadyOnlyAllowsNameUpdate(t *testing.T) {
	st := newFakeIssuerStore()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		Name:               "ready",
		CAType:             store.CATypeLetsEncryptProduction,
		DirectoryURL:       letsEncryptProductionDirectory,
		Email:              "admin@example.com",
		RegistrationStatus: store.IssuerRegistrationReady,
		AccountKeyPEM:      mustGenerateKeyPEM(t),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	st.order = []string{"iss_1"}

	m := NewManagerWithACME(st, nil, NewLegoACMEClient(nil))
	_, err := m.UpdateCertificateIssuer(context.Background(), "iss_1", IssuerInput{
		Name:  "renamed",
		Email: "other@example.com",
	})
	if !errors.Is(err, store.ErrCertificateIssuerImmutable) {
		t.Fatalf("expected immutable error, got %v", err)
	}
	updated, err := m.UpdateCertificateIssuer(context.Background(), "iss_1", IssuerInput{Name: "renamed"})
	if err != nil {
		t.Fatalf("update name: %v", err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name = %q", updated.Name)
	}
}

func TestIssuerPublicResponsesOmitSecrets(t *testing.T) {
	st := newFakeIssuerStore()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	st.issuers["iss_1"] = store.CertificateIssuer{
		ID:                 "iss_1",
		Name:               "prod",
		CAType:             store.CATypeZeroSSL,
		DirectoryURL:       zeroSSLDirectory,
		Email:              "admin@example.com",
		RegistrationStatus: store.IssuerRegistrationReady,
		AccountKeyPEM:      "secret-key-pem",
		EABKid:             "kid-secret",
		EABHMAC:            "hmac-secret",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	st.order = []string{"iss_1"}

	m := NewManagerWithACME(st, nil, NewLegoACMEClient(nil))
	got, err := m.GetCertificateIssuer(context.Background(), "iss_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(got.Name, "secret") {
		t.Fatal("response leaked secret")
	}
	if !got.AccountKeyConfigured || !got.EABConfigured {
		t.Fatalf("configured flags: key=%v eab=%v", got.AccountKeyConfigured, got.EABConfigured)
	}
}

func TestIssuerPresetRequiresEAB(t *testing.T) {
	st := newFakeIssuerStore()
	srv := startFakeACMEServer(t, false)
	m := NewManagerWithACME(st, nil, testACMEClient(t, srv))

	_, err := m.CreateCertificateIssuer(context.Background(), IssuerInput{
		Name:                 "zerossl",
		CAType:               store.CATypeZeroSSL,
		Email:                "admin@example.com",
		TermsOfServiceAgreed: true,
	})
	if !errors.Is(err, ErrEABRequired) {
		t.Fatalf("expected ErrEABRequired, got %v", err)
	}
}

func TestIssuerCustomDirectoryMustBeHTTPS(t *testing.T) {
	st := newFakeIssuerStore()
	m := NewManagerWithACME(st, nil, NewLegoACMEClient(nil))

	_, err := m.CreateCertificateIssuer(context.Background(), IssuerInput{
		Name:                 "bad",
		CAType:               store.CATypeCustom,
		CustomDirectoryURL:   "http://acme.example/directory",
		Email:                "admin@example.com",
		TermsOfServiceAgreed: true,
	})
	if !errors.Is(err, ErrInvalidDirectoryURL) {
		t.Fatalf("expected ErrInvalidDirectoryURL, got %v", err)
	}
}

func TestIssuerDirectoryPreviewFromFakeACME(t *testing.T) {
	srv := startFakeACMEServer(t, false)
	m := NewManagerWithACME(newFakeIssuerStore(), nil, testACMEClient(t, srv))

	preview, err := m.GetCertificateIssuerDirectoryPreview(context.Background(), store.CATypeCustom, srv.URL+"/directory")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.TermsOfServiceURL == "" {
		t.Fatal("expected terms_of_service_url")
	}
	if preview.DirectoryURL != srv.URL+"/directory" {
		t.Fatalf("directory_url = %q", preview.DirectoryURL)
	}
}

func mustGenerateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

var _ crypto.PrivateKey = (*ecdsa.PrivateKey)(nil)
