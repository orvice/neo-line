package connectapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterMountsConnectHandlersUnderAPIPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, nil)

	req := httptest.NewRequest(http.MethodGet, BasePath+"/neoline.v1.AuthService/Login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected %s to be routed to Connect handler, got 404", req.URL.Path)
	}
}

func TestRegisterDoesNotExposeLegacyGrpcPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/grpc/neoline.v1.AuthService/Login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected legacy /grpc prefix to be unmounted, got %d", w.Code)
	}
}
