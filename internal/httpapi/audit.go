package httpapi

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
)

func (api *API) auditMiddleware() gin.HandlerFunc {
	logger := slog.Default().With("component", "audit", "source", "api")
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		entry := store.AuditLog{
			Source:       "api",
			Action:       auditAPIAction(c),
			ResourceType: auditAPIResourceType(c),
			ResourceID:   auditAPIResourceID(c),
			Method:       c.Request.Method,
			Path:         c.FullPath(),
			StatusCode:   c.Writer.Status(),
			Success:      c.Writer.Status() < 400,
			DurationMS:   time.Since(start).Milliseconds(),
			RemoteIP:     c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			OccurredAt:   start.UTC(),
		}
		if entry.Path == "" {
			entry.Path = c.Request.URL.Path
		}
		if sessionValue, ok := c.Get(contextSessionKey); ok {
			if session, ok := sessionValue.(store.Session); ok {
				entry.ActorID = session.UserID
				entry.ActorEmail = session.Email
			}
		}
		if len(c.Errors) > 0 {
			entry.Error = c.Errors.Last().Error()
		}

		logger.Info("api request",
			"action", entry.Action,
			"method", entry.Method,
			"path", entry.Path,
			"status", entry.StatusCode,
			"success", entry.Success,
			"duration_ms", entry.DurationMS,
			"actor_id", entry.ActorID,
			"resource_type", entry.ResourceType,
			"resource_id", entry.ResourceID,
		)
		auditor, ok := api.store.(interface {
			SaveAuditLog(context.Context, store.AuditLog) error
		})
		if !ok || auditor == nil {
			return
		}
		auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := auditor.SaveAuditLog(auditCtx, entry); err != nil {
			logger.Error("failed to save api audit log", "error", err.Error())
		}
	}
}

func auditAPIAction(c *gin.Context) string {
	switch c.Request.Method {
	case "GET":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return c.Request.Method
	}
}

func auditAPIResourceType(c *gin.Context) string {
	switch {
	case c.FullPath() == "/api/v1/auth/login":
		return "auth"
	case c.FullPath() == "/api/v1/auth/logout", c.FullPath() == "/api/v1/auth/me":
		return "auth"
	case c.FullPath() == "/api/v1/settings":
		return "settings"
	case c.Param("token_id") != "":
		return "mcp_token"
	case c.Param("notify_group_id") != "":
		return "notify_group"
	case c.Param("group_id") != "":
		return "monitor_group"
	case c.Param("monitor_id") != "":
		return "monitor"
	case c.Param("id") != "":
		return "server"
	default:
		switch c.FullPath() {
		case "/api/v1/servers":
			return "server"
		case "/api/v1/monitor-groups":
			return "monitor_group"
		case "/api/v1/notify-groups":
			return "notify_group"
		case "/api/v1/mcp-tokens":
			return "mcp_token"
		default:
			return ""
		}
	}
}

func auditAPIResourceID(c *gin.Context) string {
	for _, key := range []string{"monitor_id", "id", "group_id", "notify_group_id", "token_id"} {
		if value := c.Param(key); value != "" {
			return value
		}
	}
	return ""
}
