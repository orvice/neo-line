package mcpserver

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orvice/neo-line/internal/store"
)

func addAuditedTool[In, Out any](srv *mcp.Server, tool *mcp.Tool, st store.Store, handler mcp.ToolHandlerFor[In, Out]) {
	wrapped := func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		start := time.Now()
		result, output, err := handler(ctx, req, input)

		entry := store.AuditLog{
			Source:       "mcp",
			TokenPrefix:  contextString(ctx, mcpContextTokenPrefixKey),
			Action:       tool.Name,
			ResourceType: mcpResourceType(tool.Name),
			ResourceID:   resourceID(input),
			Success:      err == nil,
			DurationMS:   time.Since(start).Milliseconds(),
			Metadata:     map[string]string{"token_source": contextString(ctx, mcpContextTokenSourceKey)},
			OccurredAt:   start.UTC(),
		}
		if err != nil {
			entry.Error = err.Error()
		}
		logMCPAudit(st, entry)
		return result, output, err
	}
	mcp.AddTool(srv, tool, wrapped)
}

func logMCPAudit(st store.Store, entry store.AuditLog) {
	logger := slog.Default().With("component", "audit", "source", "mcp")
	logger.Info("mcp tool call",
		"tool", entry.Action,
		"success", entry.Success,
		"duration_ms", entry.DurationMS,
		"resource_type", entry.ResourceType,
		"resource_id", entry.ResourceID,
		"token_prefix", entry.TokenPrefix,
	)
	if st == nil {
		return
	}
	auditor, ok := st.(interface {
		SaveAuditLog(context.Context, store.AuditLog) error
	})
	if !ok || auditor == nil {
		return
	}
	auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := auditor.SaveAuditLog(auditCtx, entry); err != nil {
		logger.Error("failed to save mcp audit log", "error", err.Error())
	}
}

func contextString(ctx context.Context, key mcpContextKey) string {
	value, _ := ctx.Value(key).(string)
	return value
}

func mcpResourceType(toolName string) string {
	switch {
	case strings.Contains(toolName, "notify_group"):
		return "notify_group"
	case strings.Contains(toolName, "monitor_group"):
		return "monitor_group"
	case strings.Contains(toolName, "monitor"):
		return "monitor"
	case strings.Contains(toolName, "server"):
		return "server"
	case strings.Contains(toolName, "mcp_token"):
		return "mcp_token"
	default:
		return ""
	}
}

func resourceID(input any) string {
	value := reflect.ValueOf(input)
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range []string{"MonitorID", "ID", "GroupID", "NotifyGroupID", "ServerID"} {
		field := value.FieldByName(name)
		if field.IsValid() && field.Kind() == reflect.String && field.String() != "" {
			return field.String()
		}
	}
	return ""
}
