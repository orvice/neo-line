// Package connectapi serves the neo-line API over Connect (gRPC, gRPC-Web, and
// the Connect protocol) backed by the same store layer the dashboard uses. It
// is mounted on the existing Gin engine under the /api/grpc path prefix.
package connectapi

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	nlssh "github.com/orvice/neo-line/internal/ssh"
	"github.com/orvice/neo-line/internal/store"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
)

// BasePath is the URL prefix the Connect handlers are mounted under. Browser
// clients point their transport baseUrl here.
const BasePath = "/api/grpc"

// Service implements every neoline.v1 Connect handler against the store.
type Service struct {
	store        store.Store
	ssh          *nlssh.Runner
	loginLimiter *loginLimiter
}

func New(st store.Store, ssh *nlssh.Runner) *Service {
	return &Service{store: st, ssh: ssh, loginLimiter: newLoginLimiter()}
}

// Register mounts the Connect handlers on the Gin engine under BasePath.
func Register(r *gin.Engine, st store.Store, ssh *nlssh.Runner) {
	svc := New(st, ssh)
	// Audit must wrap auth so rejected (unauthenticated/forbidden) calls are
	// recorded too; auth fills the session holder for actor attribution.
	opts := connect.WithInterceptors(svc.auditInterceptor(), svc.authInterceptor())

	mux := http.NewServeMux()
	mux.Handle(neolinev1connect.NewAuthServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewAuditLogServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewSettingsServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewStatusServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewServerServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewMonitorServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewMonitorGroupServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewNotifyGroupServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewMcpTokenServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewSshServiceHandler(svc, opts))

	handler := http.StripPrefix(BasePath, mux)
	r.Any(BasePath+"/*any", gin.WrapH(handler))
}

func pageLimit(size uint32) int64 {
	if size == 0 {
		return 50
	}
	return int64(size)
}
