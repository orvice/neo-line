package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
)

type API struct {
	store *store.Store
}

func Register(r *gin.Engine, st *store.Store) {
	api := &API{store: st}
	v1 := r.Group("/v1")
	{
		// Public authentication endpoint.
		v1.POST("/auth/login", api.login)

		// Public read endpoints.
		v1.GET("/servers", api.listServers)
		v1.GET("/servers/:id", api.getServer)
		v1.GET("/servers/:id/health", api.getServerHealth)
		v1.GET("/servers/:id/events", api.listServerEvents)
		v1.GET("/servers/:id/monitors", api.listMonitors)
		v1.GET("/servers/:id/monitors/:monitor_id", api.getMonitor)
		v1.GET("/servers/:id/monitors/:monitor_id/results", api.listCheckResults)

		// Admin endpoints require authentication.
		admin := v1.Group("")
		admin.Use(api.authRequired())
		{
			admin.GET("/auth/me", api.currentUser)
			admin.POST("/auth/logout", api.logout)

			admin.POST("/servers", api.createServer)
			admin.PUT("/servers/:id", api.updateServer)
			admin.DELETE("/servers/:id", api.deleteServer)

			admin.POST("/servers/:id/monitors", api.createMonitor)
			admin.PUT("/servers/:id/monitors/:monitor_id", api.updateMonitor)
			admin.DELETE("/servers/:id/monitors/:monitor_id", api.deleteMonitor)
		}
	}
}

func (api *API) listServers(c *gin.Context) {
	servers, next, err := api.store.ListServers(c.Request.Context(), c.Query("environment"), splitCSV(c.Query("tags")), pageSize(c), c.Query("page_token"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"servers": servers, "next_page_token": next})
}

func (api *API) createServer(c *gin.Context) {
	var server store.Server
	if !bindJSON(c, &server) {
		return
	}
	created, err := api.store.CreateServer(c.Request.Context(), server)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"server": created})
}

func (api *API) getServer(c *gin.Context) {
	server, err := api.store.GetServer(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"server": server})
}

func (api *API) updateServer(c *gin.Context) {
	var server store.Server
	if !bindJSON(c, &server) {
		return
	}
	updated, err := api.store.UpdateServer(c.Request.Context(), c.Param("id"), server)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"server": updated})
}

func (api *API) deleteServer(c *gin.Context) {
	if err := api.store.DeleteServer(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (api *API) getServerHealth(c *gin.Context) {
	health, err := api.store.GetServerHealth(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"health": health})
}

func (api *API) listServerEvents(c *gin.Context) {
	events, next, err := api.store.ListServerEvents(c.Request.Context(), c.Param("id"), pageSize(c), c.Query("page_token"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events, "next_page_token": next})
}

func (api *API) listMonitors(c *gin.Context) {
	monitors, next, err := api.store.ListMonitors(c.Request.Context(), c.Param("id"), pageSize(c), c.Query("page_token"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"monitors": monitors, "next_page_token": next})
}

func (api *API) createMonitor(c *gin.Context) {
	var monitor store.Monitor
	if !bindJSON(c, &monitor) {
		return
	}
	created, err := api.store.CreateMonitor(c.Request.Context(), c.Param("id"), monitor)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"monitor": created})
}

func (api *API) getMonitor(c *gin.Context) {
	monitor, err := api.store.GetMonitor(c.Request.Context(), c.Param("id"), c.Param("monitor_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"monitor": monitor})
}

func (api *API) updateMonitor(c *gin.Context) {
	var monitor store.Monitor
	if !bindJSON(c, &monitor) {
		return
	}
	updated, err := api.store.UpdateMonitor(c.Request.Context(), c.Param("id"), c.Param("monitor_id"), monitor)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"monitor": updated})
}

func (api *API) deleteMonitor(c *gin.Context) {
	if err := api.store.DeleteMonitor(c.Request.Context(), c.Param("id"), c.Param("monitor_id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (api *API) listCheckResults(c *gin.Context) {
	start, ok := parseOptionalTime(c, "start_time")
	if !ok {
		return
	}
	end, ok := parseOptionalTime(c, "end_time")
	if !ok {
		return
	}
	results, next, err := api.store.ListCheckResults(c.Request.Context(), c.Param("id"), c.Param("monitor_id"), pageSize(c), c.Query("page_token"), start, end)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "next_page_token": next})
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body", "detail": err.Error()})
		return false
	}
	return true
}

func respondError(c *gin.Context, err error) {
	if store.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err.Error() == "invalid page_token" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func pageSize(c *gin.Context) int64 {
	if c.Query("page_size") == "" {
		return 50
	}
	value, err := strconv.ParseInt(c.Query("page_size"), 10, 64)
	if err != nil {
		return 50
	}
	return value
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseOptionalTime(c *gin.Context, key string) (*time.Time, bool) {
	value := c.Query(key)
	if value == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + key, "detail": "use RFC3339 time, for example 2026-05-29T01:22:00+08:00"})
		return nil, false
	}
	return &parsed, true
}
