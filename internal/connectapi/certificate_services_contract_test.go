package connectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/certmanager"
	"github.com/orvice/neo-line/internal/store"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type contractAuthStore struct {
	serverCertFakeStore
	sessions map[string]store.Session
}

func newContractAuthStore() *contractAuthStore {
	return &contractAuthStore{
		serverCertFakeStore: *newServerCertFakeStore(),
		sessions:            make(map[string]store.Session),
	}
}

func (s *contractAuthStore) GetSession(_ context.Context, token string) (store.Session, error) {
	sess, ok := s.sessions[token]
	if !ok {
		return store.Session{}, mongo.ErrNoDocuments
	}
	return sess, nil
}

var certServiceProcedures = []struct {
	name      string
	procedure string
}{
	{"DNSProviderAccountService", neolinev1connect.DNSProviderAccountServiceListDNSProviderAccountsProcedure},
	{"CertificateIssuerService", neolinev1connect.CertificateIssuerServiceListCertificateIssuersProcedure},
	{"ManagedCertificateService", neolinev1connect.ManagedCertificateServiceListManagedCertificatesProcedure},
	{"CertificateAccessTokenService", neolinev1connect.CertificateAccessTokenServiceListCertificateAccessTokensProcedure},
	{"ServerCertificateService", neolinev1connect.ServerCertificateServiceListCertificatesProcedure},
}

func TestAllFiveCertificateServicesMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, nil, nil)

	for _, svc := range certServiceProcedures {
		req := httptest.NewRequest(http.MethodPost, BasePath+svc.procedure, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Fatalf("%s not mounted at %s", svc.name, svc.procedure)
		}
	}
}

func TestCertificateAdminServicesRequireAuthentication(t *testing.T) {
	st := newContractAuthStore()
	r := serverCertRouter(st)

	adminProcedures := []string{
		neolinev1connect.DNSProviderAccountServiceListDNSProviderAccountsProcedure,
		neolinev1connect.CertificateIssuerServiceListCertificateIssuersProcedure,
		neolinev1connect.ManagedCertificateServiceListManagedCertificatesProcedure,
		neolinev1connect.CertificateAccessTokenServiceListCertificateAccessTokensProcedure,
		neolinev1connect.ManagedCertificateServiceGetCertificateBundleProcedure,
	}

	for _, procedure := range adminProcedures {
		w := connectPost(t, r, procedure, map[string]any{}, "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without auth: expected 401, got %d", procedure, w.Code)
		}
	}
}

func TestCertificateAdminMutationsRequireAdminRole(t *testing.T) {
	st := newContractAuthStore()
	st.sessions["viewer-token"] = store.Session{
		Token:  "viewer-token",
		Role:   "viewer",
		UserID: "usr_viewer",
		Email:  "viewer@example.com",
	}
	r := serverCertRouter(st)

	mutations := []struct {
		procedure string
		body      map[string]any
	}{
		{neolinev1connect.DNSProviderAccountServiceCreateDNSProviderAccountProcedure, map[string]any{"account": map[string]any{"name": "x", "provider": "cloudflare", "apiToken": "t"}}},
		{neolinev1connect.ManagedCertificateServiceCreateManagedCertificateProcedure, map[string]any{"certificate": map[string]any{"name": "x", "domains": []string{"a.com"}}}},
		{neolinev1connect.CertificateAccessTokenServiceCreateCertificateAccessTokenProcedure, map[string]any{"serverId": "srv_1", "name": "deploy"}},
	}

	for _, m := range mutations {
		w := connectPost(t, r, m.procedure, m.body, "Bearer viewer-token")
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s non-admin: expected 403, got %d body=%s", m.procedure, w.Code, w.Body.String())
		}
	}
}

func TestServerCertificateServiceIsolatedFromAdminSession(t *testing.T) {
	st := newContractAuthStore()
	st.sessions["admin-token"] = store.Session{
		Token:  "admin-token",
		Role:   store.RoleAdmin,
		UserID: "usr_admin",
		Email:  "admin@example.com",
	}
	r := serverCertRouter(st)

	w := connectPost(t, r, neolinev1connect.ServerCertificateServiceListCertificatesProcedure, map[string]any{}, "Bearer admin-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("admin session on server cert service: expected 401, got %d", w.Code)
	}
}

func TestManagedCertificateAdminBundleSetsNoStore(t *testing.T) {
	authSt := newContractAuthStore()
	authSt.sessions["admin-token"] = store.Session{
		Token:  "admin-token",
		Role:   store.RoleAdmin,
		UserID: "usr_admin",
		Email:  "admin@example.com",
	}
	authSt.certs["mcert_1"] = store.ManagedCertificate{
		ID:   "mcert_1",
		Name: "admin-bundle",
		ActiveVersion: &store.CertificateVersion{
			ID:            "cver_1",
			FullchainPEM:  "chain",
			PrivateKeyPEM: "key",
			ConfigSnapshot: store.IssueConfigSnapshot{
				Domains: []string{"example.com"},
			},
			KeyType: store.CertKeyTypeECP256,
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mgr := certmanager.NewManager(certmanager.NewStore(authSt), nil)
	Register(r, authSt, mgr, nil)

	w := connectPost(t, r, neolinev1connect.ManagedCertificateServiceGetCertificateBundleProcedure, map[string]any{
		"managedCertificateId": "mcert_1",
		"versionSlot":          "active",
	}, "Bearer admin-token")
	if w.Code != http.StatusOK {
		t.Fatalf("admin bundle status = %d body=%s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", cc)
	}
	body := w.Body.String()
	for _, secret := range []string{"nlct_", "cf-token"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaked %q", secret)
		}
	}
}
