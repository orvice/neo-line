package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
)

// fakeTokenStore embeds store.Store so it satisfies the interface while only
// implementing the token methods the MCP auth middleware exercises.
type fakeTokenStore struct {
	store.Store
	valid map[string]bool
	count int64
}

func (f *fakeTokenStore) ValidateMcpToken(_ context.Context, plaintext string) (bool, error) {
	return f.valid[plaintext], nil
}

func (f *fakeTokenStore) CountMcpTokens(_ context.Context) (int64, error) {
	return f.count, nil
}

func newRouterWithStore(st store.Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, st)
	return r
}

func TestMCPAcceptsStoredToken(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "")
	st := &fakeTokenStore{valid: map[string]bool{"mcp_abc": true}, count: 1}
	r := newRouterWithStore(st)

	req := mcpRequest(initBody)
	req.Header.Set("Authorization", "Bearer mcp_abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected stored token to authorize, got 401")
	}
}

func TestMCPRejectsUnknownTokenWhenStoreHasTokens(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "")
	st := &fakeTokenStore{valid: map[string]bool{"mcp_abc": true}, count: 1}
	r := newRouterWithStore(st)

	req := mcpRequest(initBody)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown token, got %d", w.Code)
	}
}

func TestMCPOpenWhenNoTokensConfigured(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "")
	st := &fakeTokenStore{count: 0}
	r := newRouterWithStore(st)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, mcpRequest(initBody))

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected open access with no tokens configured, got 401")
	}
}

func TestMCPEnvTokenStillWorksAlongsideStore(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "secret")
	st := &fakeTokenStore{count: 0}
	r := newRouterWithStore(st)

	req := mcpRequest(initBody)
	req.Header.Set("X-MCP-Token", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected env token to authorize, got 401")
	}
}
