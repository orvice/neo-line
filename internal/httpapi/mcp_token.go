package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type createMcpTokenRequest struct {
	Name string `json:"name"`
}

func (api *API) listMcpTokens(c *gin.Context) {
	tokens, err := api.store.ListMcpTokens(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

// createMcpToken generates a new token and returns the plaintext secret once.
// The secret is not stored and cannot be retrieved later.
func (api *API) createMcpToken(c *gin.Context) {
	var req createMcpTokenRequest
	if !bindJSON(c, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	token, plaintext, err := api.store.CreateMcpToken(c.Request.Context(), name)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "secret": plaintext})
}

func (api *API) deleteMcpToken(c *gin.Context) {
	if err := api.store.DeleteMcpToken(c.Request.Context(), c.Param("token_id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
