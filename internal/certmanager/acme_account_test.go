package certmanager

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

type acmeProtectedHeader struct {
	KID string          `json:"kid"`
	JWK json.RawMessage `json:"jwk"`
}

type protectedHeaderResult struct {
	header acmeProtectedHeader
	err    error
}

func TestLegoACMEClientUsesPersistedAccountURIForNewOrder(t *testing.T) {
	orderHeader := make(chan protectedHeaderResult, 1)
	var accountURI string
	var nonce atomic.Int64

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/directory":
			baseURL := "https://" + r.Host
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"newNonce":%q,"newAccount":%q,"newOrder":%q}`,
				baseURL+"/new-nonce", baseURL+"/new-account", baseURL+"/new-order")
		case r.Method == http.MethodHead && r.URL.Path == "/new-nonce":
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonce.Add(1)))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/new-account":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonce.Add(1)))
			w.Header().Set("Location", accountURI)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"status":"valid"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/new-order":
			orderHeader <- decodeACMEProtectedHeader(r)
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonce.Add(1)))
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"urn:ietf:params:acme:error:malformed","detail":"test stopped after newOrder"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	accountURI = srv.URL + "/account/1"

	client := testACMEClient(t, srv)
	issuer := store.CertificateIssuer{
		DirectoryURL:  srv.URL + "/directory",
		Email:         "admin@example.com",
		AccountKeyPEM: mustGenerateKeyPEM(t),
	}
	registration, err := client.RegisterAccount(t.Context(), issuer)
	if err != nil {
		t.Fatalf("register account: %v", err)
	}
	if registration.URI != accountURI {
		t.Fatalf("registration URI = %q, want %q", registration.URI, accountURI)
	}

	issuer.AccountURI = registration.URI
	_, err = client.IssueCertificate(t.Context(), IssueRequest{
		Issuer:      issuer,
		Domains:     []string{"example.com"},
		KeyType:     store.CertKeyTypeECP256,
		DNS:         noopDNSProvider{},
		OperationID: "cop_test",
	})
	if err == nil {
		t.Fatal("expected fake newOrder endpoint to stop issuance")
	}

	result := awaitProtectedHeader(t, orderHeader)
	if result.err != nil {
		t.Fatalf("decode newOrder protected header: %v", result.err)
	}
	if result.header.KID != accountURI {
		t.Fatalf("newOrder kid = %q, want %q", result.header.KID, accountURI)
	}
	if len(result.header.JWK) != 0 && string(result.header.JWK) != "null" {
		t.Fatalf("newOrder unexpectedly embedded jwk: %s", result.header.JWK)
	}
}

func TestLegoACMEClientUsesPersistedAccountURIForRevoke(t *testing.T) {
	revokeHeader := make(chan protectedHeaderResult, 1)
	var nonce atomic.Int64

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/directory":
			baseURL := "https://" + r.Host
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"newNonce":%q,"newAccount":%q,"newOrder":%q,"revokeCert":%q}`,
				baseURL+"/new-nonce", baseURL+"/new-account", baseURL+"/new-order", baseURL+"/revoke-cert")
		case r.Method == http.MethodHead && r.URL.Path == "/new-nonce":
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonce.Add(1)))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/revoke-cert":
			revokeHeader <- decodeACMEProtectedHeader(r)
			w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", nonce.Add(1)))
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := testACMEClient(t, srv)
	accountURI := srv.URL + "/account/1"
	issuer := store.CertificateIssuer{
		DirectoryURL:  srv.URL + "/directory",
		Email:         "admin@example.com",
		AccountKeyPEM: mustGenerateKeyPEM(t),
		AccountURI:    accountURI,
	}
	fullchain, _ := generateTestBundle(t, []string{"example.com"}, store.CertKeyTypeECP256)
	if err := client.RevokeCertificate(t.Context(), issuer, fullchain, nil); err != nil {
		t.Fatalf("revoke certificate: %v", err)
	}

	result := awaitProtectedHeader(t, revokeHeader)
	if result.err != nil {
		t.Fatalf("decode revoke protected header: %v", result.err)
	}
	if result.header.KID != accountURI {
		t.Fatalf("revoke kid = %q, want %q", result.header.KID, accountURI)
	}
	if len(result.header.JWK) != 0 && string(result.header.JWK) != "null" {
		t.Fatalf("revoke unexpectedly embedded jwk: %s", result.header.JWK)
	}
}

func awaitProtectedHeader(t *testing.T, results <-chan protectedHeaderResult) protectedHeaderResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ACME protected header")
		return protectedHeaderResult{}
	}
}

func decodeACMEProtectedHeader(r *http.Request) protectedHeaderResult {
	var envelope struct {
		Protected string `json:"protected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		return protectedHeaderResult{err: err}
	}
	protected, err := base64.RawURLEncoding.DecodeString(envelope.Protected)
	if err != nil {
		return protectedHeaderResult{err: err}
	}
	var header acmeProtectedHeader
	if err := json.Unmarshal(protected, &header); err != nil {
		return protectedHeaderResult{err: err}
	}
	return protectedHeaderResult{header: header}
}
