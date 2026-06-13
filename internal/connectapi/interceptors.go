package connectapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/store"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
)

// publicProcedures are reachable without authentication. They mirror the
// unauthenticated surface of the legacy REST API: login, the slim public status
// overview, and the read-only site settings the status page renders.
var publicProcedures = map[string]bool{
	neolinev1connect.AuthServiceLoginProcedure:               true,
	neolinev1connect.StatusServiceGetStatusOverviewProcedure: true,
	neolinev1connect.SettingsServiceGetSettingsProcedure:     true,
}

// adminProcedureExempt lists services whose procedures never require the admin
// role even when their method names look mutating (login/logout manage the
// caller's own session).
var adminProcedureExempt = map[string]bool{
	"AuthService": true,
}

// requiresAdmin reports whether a procedure is restricted to admin sessions:
// remote command execution, MCP token management, and every mutating
// (Create/Update/Delete) procedure.
func requiresAdmin(procedure string) bool {
	service, method := splitProcedure(procedure)
	if adminProcedureExempt[service] {
		return false
	}
	switch service {
	case "SshService", "McpTokenService":
		return true
	}
	return strings.HasPrefix(method, "Create") ||
		strings.HasPrefix(method, "Update") ||
		strings.HasPrefix(method, "Delete")
}

// authInterceptor validates the bearer token for non-public procedures,
// attaches the resolved session to the context, and enforces the admin role on
// restricted procedures.
func (s *Service) authInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			if publicProcedures[procedure] {
				return next(ctx, req)
			}
			token := bearerToken(req.Header().Get("Authorization"))
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
			}
			session, err := s.store.GetSession(ctx, token)
			if err != nil {
				if store.IsNotFound(err) {
					return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or expired token"))
				}
				slog.ErrorContext(ctx, "resolve session", "error", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
			}
			if requiresAdmin(procedure) && session.Role != store.RoleAdmin {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin role required"))
			}
			return next(withSession(ctx, session), req)
		}
	}
}

// auditInterceptor records each call to the audit log and structured logger,
// mirroring the metadata captured by the legacy REST audit middleware. It must
// be the outermost interceptor so that requests rejected by the auth
// interceptor are audited too.
func (s *Service) auditInterceptor() connect.UnaryInterceptorFunc {
	logger := slog.Default().With("component", "audit", "source", "api")
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			ctx, holder := withSessionHolder(ctx)
			res, err := next(ctx, req)

			procedure := req.Spec().Procedure
			entry := store.AuditLog{
				Source:       "api",
				Action:       auditAction(procedure),
				ResourceType: auditResourceType(procedure),
				ResourceID:   auditResourceID(req.Any()),
				Method:       "POST",
				Path:         procedure,
				StatusCode:   auditStatusCode(err),
				Success:      err == nil,
				DurationMS:   time.Since(start).Milliseconds(),
				RemoteIP:     req.Peer().Addr,
				UserAgent:    req.Header().Get("User-Agent"),
				OccurredAt:   start.UTC(),
			}
			if holder.session != nil {
				entry.ActorID = holder.session.UserID
				entry.ActorEmail = holder.session.Email
			}
			if err != nil {
				entry.Error = err.Error()
			}

			logger.Info("api request",
				"action", entry.Action,
				"procedure", procedure,
				"status", entry.StatusCode,
				"success", entry.Success,
				"duration_ms", entry.DurationMS,
				"actor_id", entry.ActorID,
				"resource_type", entry.ResourceType,
				"resource_id", entry.ResourceID,
			)

			auditor, ok := s.store.(interface {
				SaveAuditLog(context.Context, store.AuditLog) error
			})
			if ok && auditor != nil {
				auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if saveErr := auditor.SaveAuditLog(auditCtx, entry); saveErr != nil {
					logger.Error("failed to save api audit log", "error", saveErr.Error())
				}
			}

			return res, err
		}
	}
}

func splitProcedure(procedure string) (service, method string) {
	trimmed := strings.TrimPrefix(procedure, "/")
	slash := strings.LastIndex(trimmed, "/")
	if slash < 0 {
		return "", trimmed
	}
	svc := trimmed[:slash]
	if dot := strings.LastIndex(svc, "."); dot >= 0 {
		svc = svc[dot+1:]
	}
	return svc, trimmed[slash+1:]
}

func auditAction(procedure string) string {
	_, method := splitProcedure(procedure)
	switch {
	case strings.HasPrefix(method, "Create"), method == "Login":
		return "create"
	case strings.HasPrefix(method, "Update"):
		return "update"
	case strings.HasPrefix(method, "Delete"), method == "Logout":
		return "delete"
	default:
		return "read"
	}
}

func auditResourceType(procedure string) string {
	service, _ := splitProcedure(procedure)
	switch service {
	case "AuditLogService":
		return "audit_log"
	case "AuthService":
		return "auth"
	case "SettingsService":
		return "settings"
	case "ServerService":
		return "server"
	case "MonitorService":
		return "monitor"
	case "MonitorGroupService":
		return "monitor_group"
	case "NotifyGroupService":
		return "notify_group"
	case "McpTokenService":
		return "mcp_token"
	case "SshService":
		return "ssh"
	default:
		return ""
	}
}

func auditResourceID(msg any) string {
	type monitorID interface{ GetMonitorId() string }
	type id interface{ GetId() string }
	type groupID interface{ GetGroupId() string }
	type notifyGroupID interface{ GetNotifyGroupId() string }
	type serverID interface{ GetServerId() string }
	type tokenID interface{ GetTokenId() string }

	if m, ok := msg.(monitorID); ok && m.GetMonitorId() != "" {
		return m.GetMonitorId()
	}
	if m, ok := msg.(serverID); ok && m.GetServerId() != "" {
		return m.GetServerId()
	}
	if m, ok := msg.(id); ok && m.GetId() != "" {
		return m.GetId()
	}
	if m, ok := msg.(groupID); ok && m.GetGroupId() != "" {
		return m.GetGroupId()
	}
	if m, ok := msg.(notifyGroupID); ok && m.GetNotifyGroupId() != "" {
		return m.GetNotifyGroupId()
	}
	if m, ok := msg.(tokenID); ok && m.GetTokenId() != "" {
		return m.GetTokenId()
	}
	return ""
}

func auditStatusCode(err error) int {
	if err == nil {
		return 200
	}
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument:
		return 400
	case connect.CodeUnauthenticated:
		return 401
	case connect.CodePermissionDenied:
		return 403
	case connect.CodeNotFound:
		return 404
	case connect.CodeAlreadyExists:
		return 409
	case connect.CodeFailedPrecondition:
		return 412
	case connect.CodeResourceExhausted:
		return 429
	case connect.CodeUnavailable:
		return 503
	case connect.CodeDeadlineExceeded:
		return 504
	default:
		return 500
	}
}
