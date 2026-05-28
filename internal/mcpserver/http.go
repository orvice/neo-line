package mcpserver

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orvice/neo-line/internal/store"
)

// Register mounts the MCP streamable HTTP endpoint on the gin engine at /mcp.
// When MCP_AUTH_TOKEN is set, requests must present it via the Authorization
// bearer header or the X-MCP-Token header.
func Register(r *gin.Engine, st store.Store) {
	server := NewServer(st)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	group := r.Group("/mcp")
	group.Use(authRequired())
	group.Any("", gin.WrapH(handler))
	group.Any("/*path", gin.WrapH(handler))
}

// authRequired enforces a static header token when MCP_AUTH_TOKEN is configured.
func authRequired() gin.HandlerFunc {
	token := os.Getenv("MCP_AUTH_TOKEN")
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}
		if requestToken(c) != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing MCP auth token"})
			return
		}
		c.Next()
	}
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
