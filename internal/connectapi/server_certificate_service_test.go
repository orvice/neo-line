package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/certmanager"
	"github.com/orvice/neo-line/internal/store"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type serverCertFakeStore struct {
	store.Store
	mu sync.Mutex

	tokens   map[string]store.CertificateAccessToken
	secrets  map[string]string
	certs    map[string]store.ManagedCertificate
	audits   []store.AuditLog
	rateKeys map[string]int
	rateErr  error
}

func newServerCertFakeStore() *serverCertFakeStore {
	return &serverCertFakeStore{
		tokens:   make(map[string]store.CertificateAccessToken),
		secrets:  make(map[string]string),
		certs:    make(map[string]store.ManagedCertificate),
		rateKeys: make(map[string]int),
	}
}

func (f *serverCertFakeStore) LookupCertificateAccessToken(_ context.Context, plaintext string) (store.CertificateAccessToken, error) {
	if !strings.HasPrefix(plaintext, "nlct_") {
		return store.CertificateAccessToken{}, mongo.ErrNoDocuments
	}
	id, ok := f.secrets[plaintext]
	if !ok {
		return store.CertificateAccessToken{}, mongo.ErrNoDocuments
	}
	tok := f.tokens[id]
	now := time.Now().UTC()
	if tok.ExpiresAt != nil && !tok.ExpiresAt.After(now) {
		return store.CertificateAccessToken{}, mongo.ErrNoDocuments
	}
	return tok, nil
}

func (f *serverCertFakeStore) RateLimitAllow(_ context.Context, key string, limit int, _ time.Duration) (bool, error) {
	if f.rateErr != nil {
		return true, f.rateErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rateKeys[key]++
	return f.rateKeys[key] <= limit, nil
}

func (f *serverCertFakeStore) ListManagedCertificatesByServer(_ context.Context, serverID string) ([]store.ManagedCertificate, error) {
	out := make([]store.ManagedCertificate, 0)
	for _, c := range f.certs {
		for _, sid := range c.ServerIDs {
			if sid == serverID {
				out = append(out, c)
				break
			}
		}
	}
	return out, nil
}

func (f *serverCertFakeStore) GetManagedCertificate(_ context.Context, id string) (store.ManagedCertificate, error) {
	c, ok := f.certs[id]
	if !ok {
		return store.ManagedCertificate{}, mongo.ErrNoDocuments
	}
	return c, nil
}

func (f *serverCertFakeStore) LatestCertificateOperation(context.Context, string) (store.CertificateOperation, error) {
	return store.CertificateOperation{}, mongo.ErrNoDocuments
}

func (f *serverCertFakeStore) SaveAuditLog(_ context.Context, entry store.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, entry)
	return nil
}

func (f *serverCertFakeStore) addToken(serverID, id, prefix, secret string) {
	f.tokens[id] = store.CertificateAccessToken{
		ID:       id,
		ServerID: serverID,
		Prefix:   prefix,
	}
	f.secrets[secret] = id
}

func serverCertRouter(st store.Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, st, certmanager.NewManager(certmanager.NewStore(st), nil), nil)
	return r
}

func connectPost(t *testing.T, r *gin.Engine, procedure string, body any, auth string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, BasePath+procedure, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestServerCertificateRejectsAdminAndMcpTokens(t *testing.T) {
	st := newServerCertFakeStore()
	r := serverCertRouter(st)

	for _, auth := range []string{
		"Bearer admin-session-token",
		"Bearer mcp_abc123",
		"",
	} {
		w := connectPost(t, r, "/neoline.v1.ServerCertificateService/ListCertificates", map[string]any{}, auth)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q: expected 401, got %d", auth, w.Code)
		}
	}
}

func TestServerCertificateListAndDownload(t *testing.T) {
	st := newServerCertFakeStore()
	secret := "nlct_" + strings.Repeat("a", 64)
	st.addToken("srv_1", "cat_1", "nlct_aaaaaaaa", secret)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	st.certs["mcert_1"] = store.ManagedCertificate{
		ID:        "mcert_1",
		Name:      "web",
		ServerIDs: []string{"srv_1"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "-----BEGIN CERTIFICATE-----\nleaf\n-----END CERTIFICATE-----",
			PrivateKeyPEM: "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
			ConfigSnapshot: store.IssueConfigSnapshot{
				Domains: []string{"example.com"},
			},
			KeyType:         store.CertKeyTypeECP256,
			LeafFingerprint: "abc123",
			NotBefore:       now,
			NotAfter:        now.Add(24 * time.Hour),
		},
	}
	r := serverCertRouter(st)
	auth := "Bearer " + secret

	w := connectPost(t, r, "/neoline.v1.ServerCertificateService/ListCertificates", map[string]any{}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", w.Code, w.Body.String())
	}
	if len(st.audits) != 0 {
		t.Fatalf("successful list must not audit, got %d entries", len(st.audits))
	}

	w = connectPost(t, r, "/neoline.v1.ServerCertificateService/GetCertificateBundle", map[string]any{
		"managedCertificateId": "mcert_1",
	}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("bundle status = %d body=%s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q", cc)
	}
	if len(st.audits) != 1 || st.audits[0].Action != "download" || st.audits[0].Source != "server_cert" {
		t.Fatalf("audits = %+v", st.audits)
	}
}

func TestServerCertificateNotFoundEnumeration(t *testing.T) {
	st := newServerCertFakeStore()
	secret := "nlct_" + strings.Repeat("b", 64)
	st.addToken("srv_1", "cat_1", "nlct_bbbbbbbb", secret)
	st.certs["mcert_other"] = store.ManagedCertificate{
		ID:        "mcert_other",
		ServerIDs: []string{"srv_2"},
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
		},
	}
	r := serverCertRouter(st)
	auth := "Bearer " + secret

	for _, id := range []string{"missing", "mcert_other"} {
		w := connectPost(t, r, "/neoline.v1.ServerCertificateService/GetCertificateBundle", map[string]any{
			"managedCertificateId": id,
		}, auth)
		if w.Code != http.StatusNotFound {
			t.Fatalf("id %q status = %d", id, w.Code)
		}
	}
}

func TestServerCertificateFailedPreconditionNoActive(t *testing.T) {
	st := newServerCertFakeStore()
	secret := "nlct_" + strings.Repeat("c", 64)
	st.addToken("srv_1", "cat_1", "nlct_cccccccc", secret)
	st.certs["mcert_1"] = store.ManagedCertificate{
		ID:        "mcert_1",
		ServerIDs: []string{"srv_1"},
	}
	r := serverCertRouter(st)
	w := connectPost(t, r, "/neoline.v1.ServerCertificateService/GetCertificateBundle", map[string]any{
		"managedCertificateId": "mcert_1",
	}, "Bearer "+secret)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestServerCertificateRateLimitFailOpen(t *testing.T) {
	st := newServerCertFakeStore()
	st.rateErr = errors.New("redis down")
	secret := "nlct_" + strings.Repeat("d", 64)
	st.addToken("srv_1", "cat_1", "nlct_dddddddd", secret)
	r := serverCertRouter(st)

	for i := 0; i < 130; i++ {
		w := connectPost(t, r, "/neoline.v1.ServerCertificateService/ListCertificates", map[string]any{}, "Bearer "+secret)
		if w.Code == http.StatusTooManyRequests {
			t.Fatal("redis failure must fail-open")
		}
	}
}

func TestServerCertificateRateLimitExceeded(t *testing.T) {
	st := newServerCertFakeStore()
	secret := "nlct_" + strings.Repeat("e", 64)
	st.addToken("srv_1", "cat_1", "nlct_eeeeeeee", secret)
	r := serverCertRouter(st)
	auth := "Bearer " + secret

	var lastCode int
	for i := 0; i < 121; i++ {
		w := connectPost(t, r, "/neoline.v1.ServerCertificateService/ListCertificates", map[string]any{}, auth)
		lastCode = w.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on 121st request, got %d", lastCode)
	}
	if code := connect.CodeOf(connect.NewError(connect.CodeResourceExhausted, nil)); code != connect.CodeResourceExhausted {
		t.Fatalf("unexpected code mapping")
	}
}

func TestServerCertificateAuthFailureAudited(t *testing.T) {
	st := newServerCertFakeStore()
	r := serverCertRouter(st)
	connectPost(t, r, "/neoline.v1.ServerCertificateService/ListCertificates", map[string]any{}, "Bearer nlct_invalidsecret")
	if len(st.audits) != 1 || st.audits[0].Action != "auth" {
		t.Fatalf("audits = %+v", st.audits)
	}
}

func TestServerCertificateServiceMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, BasePath+"/"+strings.TrimPrefix(neolinev1connect.ServerCertificateServiceListCertificatesProcedure, "/"), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("ServerCertificateService not mounted")
	}
}
