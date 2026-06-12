package mcpserver

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	nlssh "github.com/orvice/neo-line/internal/ssh"
	"github.com/orvice/neo-line/internal/store"
)

const (
	contextTokenPrefixKey = "mcp_token_prefix"
	contextTokenSourceKey = "mcp_token_source"
)

type mcpContextKey string

const (
	mcpContextTokenPrefixKey mcpContextKey = "mcp_token_prefix"
	mcpContextTokenSourceKey mcpContextKey = "mcp_token_source"
)

// Register mounts the MCP streamable HTTP endpoint on the gin engine at
// /api/mcp. Requests authenticate with a token presented via the Authorization
// bearer header or the X-MCP-Token header. Valid tokens are those stored in the
// mcp_tokens collection, plus the optional static MCP_AUTH_TOKEN env value.
func Register(r *gin.Engine, st store.Store, ssh *nlssh.Runner) {
	server := NewServer(st, ssh)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	group := r.Group("/api/mcp")
	group.Use(authRequired(st))
	group.Any("", gin.WrapH(handler))
	group.Any("/*path", gin.WrapH(handler))
}

// authRequired enforces token auth for the MCP endpoint. A request is allowed
// when its token matches the static MCP_AUTH_TOKEN env value or a token stored
// in MongoDB. Unauthenticated access is refused unless MCP_ALLOW_ANONYMOUS=true
// is set explicitly and no env token or stored token exists (local dev only —
// the MCP endpoint can execute remote SSH commands).
func authRequired(st store.Store) gin.HandlerFunc {
	envToken := os.Getenv("MCP_AUTH_TOKEN")
	allowAnonymous := strings.EqualFold(os.Getenv("MCP_ALLOW_ANONYMOUS"), "true")
	return func(c *gin.Context) {
		reqToken := requestToken(c)
		if envToken != "" && subtle.ConstantTimeCompare([]byte(reqToken), []byte(envToken)) == 1 {
			c.Set(contextTokenSourceKey, "env")
			c.Set(contextTokenPrefixKey, tokenPrefix(reqToken))
			attachMCPContext(c)
			c.Next()
			return
		}
		if reqToken != "" && st != nil {
			ok, err := st.ValidateMcpToken(c.Request.Context(), reqToken)
			if err != nil {
				slog.ErrorContext(c.Request.Context(), "validate mcp token", "error", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
				return
			}
			if ok {
				c.Set(contextTokenSourceKey, "stored")
				c.Set(contextTokenPrefixKey, tokenPrefix(reqToken))
				attachMCPContext(c)
				c.Next()
				return
			}
		}
		if allowAnonymous && envToken == "" && !tokensConfigured(c, st) {
			c.Set(contextTokenSourceKey, "none")
			attachMCPContext(c)
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing MCP auth token"})
	}
}

// tokensConfigured reports whether any MCP token is stored. Errors are treated
// as "configured" so a transient store failure fails closed rather than opening
// the endpoint.
func tokensConfigured(c *gin.Context, st store.Store) bool {
	if st == nil {
		return false
	}
	count, err := st.CountMcpTokens(c.Request.Context())
	if err != nil {
		return true
	}
	return count > 0
}

func requestToken(c *gin.Context) string {
	if header := c.GetHeader("Authorization"); header != "" {
		const prefix = "Bearer "
		if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
			return strings.TrimSpace(header[len(prefix):])
		}
	}
	return strings.TrimSpace(c.GetHeader("X-MCP-Token"))
}

func attachMCPContext(c *gin.Context) {
	ctx := c.Request.Context()
	if value, ok := c.Get(contextTokenPrefixKey); ok {
		ctx = context.WithValue(ctx, mcpContextTokenPrefixKey, value)
	}
	if value, ok := c.Get(contextTokenSourceKey); ok {
		ctx = context.WithValue(ctx, mcpContextTokenSourceKey, value)
	}
	c.Request = c.Request.WithContext(ctx)
}

// tokenPrefix returns a short display prefix that never reveals the whole
// token: at most maxPrefix characters and never more than half the token.
func tokenPrefix(token string) string {
	const maxPrefix = 12
	n := min(len(token)/2, maxPrefix)
	return token[:n]
}
