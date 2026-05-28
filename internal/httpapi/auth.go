package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
)

const contextSessionKey = "auth_session"

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (api *API) login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
		return
	}
	user, err := api.store.Authenticate(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		respondError(c, err)
		return
	}
	session, err := api.store.CreateSession(c.Request.Context(), user)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":      session.Token,
		"expires_at": session.ExpiresAt,
		"user":       gin.H{"id": user.ID, "email": user.Email, "role": user.Role},
	})
}

func (api *API) logout(c *gin.Context) {
	session := c.MustGet(contextSessionKey).(store.Session)
	if err := api.store.DeleteSession(c.Request.Context(), session.Token); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (api *API) currentUser(c *gin.Context) {
	session := c.MustGet(contextSessionKey).(store.Session)
	c.JSON(http.StatusOK, gin.H{"user": gin.H{"id": session.UserID, "email": session.Email, "role": session.Role}})
}

// authRequired validates the bearer token and aborts unauthenticated requests.
func (api *API) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		session, err := api.store.GetSession(c.Request.Context(), token)
		if err != nil {
			if store.IsNotFound(err) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Set(contextSessionKey, session)
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
