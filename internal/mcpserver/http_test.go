package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil)
	return r
}

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

func mcpRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

func TestMCPNoAuthByDefault(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "")
	r := newRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, mcpRequest(initBody))

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected request to pass without token, got 401")
	}
}

func TestMCPRejectsMissingToken(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "secret")
	r := newRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, mcpRequest(initBody))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestMCPAcceptsBearerToken(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "secret")
	r := newRouter()

	req := mcpRequest(initBody)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected bearer token to authorize, got 401")
	}
}

func TestMCPAcceptsHeaderToken(t *testing.T) {
	t.Setenv("MCP_AUTH_TOKEN", "secret")
	r := newRouter()

	req := mcpRequest(initBody)
	req.Header.Set("X-MCP-Token", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected X-MCP-Token to authorize, got 401")
	}
}
