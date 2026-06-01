package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
	"golang.org/x/sync/errgroup"
)

// uptimeFetchConcurrency bounds the parallel uptime reads the status overview
// issues so a large monitor set does not overwhelm the Mongo connection pool.
const uptimeFetchConcurrency = 16

// statusOverviewCacheKey and statusOverviewCacheTTL back the short-lived cache for
// the public overview. The page polls frequently and monitor data only changes at
// probe intervals, so a brief TTL slashes Mongo load with negligible staleness.
const (
	statusOverviewCacheKey = "status:overview"
	statusOverviewCacheTTL = 10 * time.Second
)

// statusPageLimit bounds how many groups and monitors the public overview reads.
const statusPageLimit = 200

// publicStatus is the anonymous, read-only payload backing the status page. It
// is intentionally narrow: only what is needed to render health and uptime is
// included. Hosts, URLs, ports, headers, SSH config, certificate identity, and
// notification channels are never exposed here.
type publicStatus struct {
	Groups []publicStatusGroup `json:"groups"`
}

type publicStatusGroup struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	SortOrder   uint32               `json:"sort_order"`
	Servers     []publicStatusServer `json:"servers"`
}

type publicStatusServer struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Environment string                `json:"environment,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Monitors    []publicStatusMonitor `json:"monitors"`
}

type publicStatusMonitor struct {
	ID              string              `json:"id"`
	ServerID        string              `json:"server_id"`
	Name            string              `json:"name"`
	Kind            string              `json:"kind"`
	Status          string              `json:"status"`
	IntervalSeconds uint32              `json:"interval_seconds"`
	LastCheckAt     time.Time           `json:"last_check_at,omitempty"`
	WarningDays     uint32              `json:"warning_days,omitempty"`
	CriticalDays    uint32              `json:"critical_days,omitempty"`
	Certificate     *publicCertificate  `json:"certificate,omitempty"`
	Uptime          store.MonitorUptime `json:"uptime"`
}

// publicCertificate carries only validity timing, never the subject, issuer,
// DNS names, or serial that would reveal hostnames and identity.
type publicCertificate struct {
	NotBefore     time.Time `json:"not_before,omitempty"`
	NotAfter      time.Time `json:"not_after,omitempty"`
	DaysRemaining int32     `json:"days_remaining,omitempty"`
}

// getStatusOverview returns the aggregated, slim status payload for anonymous
// callers. It collapses the per-resource calls the status page used to make
// (groups, servers, monitors, uptime) into one response built from non-sensitive
// fields only.
func (api *API) getStatusOverview(c *gin.Context) {
	ctx := c.Request.Context()

	if cached, found, err := api.store.CacheGet(ctx, statusOverviewCacheKey); err != nil {
		slog.WarnContext(ctx, "status overview cache read failed", "error", err)
	} else if found {
		c.Data(http.StatusOK, "application/json; charset=utf-8", cached)
		return
	}

	groups, _, err := api.store.ListMonitorGroups(ctx, statusPageLimit, "")
	if err != nil {
		respondError(c, err)
		return
	}

	serverCache := make(map[string]store.Server)
	out := publicStatus{Groups: make([]publicStatusGroup, 0, len(groups))}

	for _, group := range groups {
		monitors, _, err := api.store.ListMonitorsByGroup(ctx, group.ID, statusPageLimit, "")
		if err != nil {
			respondError(c, err)
			return
		}

		servers, err := api.buildStatusServers(ctx, monitors, serverCache)
		if err != nil {
			respondError(c, err)
			return
		}

		out.Groups = append(out.Groups, publicStatusGroup{
			ID:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			SortOrder:   group.SortOrder,
			Servers:     servers,
		})
	}

	payload, err := json.Marshal(out)
	if err != nil {
		respondError(c, err)
		return
	}
	if err := api.store.CacheSet(ctx, statusOverviewCacheKey, payload, statusOverviewCacheTTL); err != nil {
		slog.WarnContext(ctx, "status overview cache write failed", "error", err)
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
}

// buildStatusServers groups enabled monitors by server (preserving first-seen
// order) and attaches uptime for each monitor. Uptime reads are the dominant
// cost, so they are fetched concurrently with a bounded worker count.
func (api *API) buildStatusServers(ctx context.Context, monitors []store.Monitor, serverCache map[string]store.Server) ([]publicStatusServer, error) {
	visible := make([]store.Monitor, 0, len(monitors))
	for _, m := range monitors {
		if !m.Enabled {
			continue
		}
		server, err := api.lookupServer(ctx, m.ServerID, serverCache)
		if err != nil {
			return nil, err
		}
		if !server.Enabled {
			continue
		}
		visible = append(visible, m)
	}

	uptimes := make([]store.MonitorUptime, len(visible))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(uptimeFetchConcurrency)
	for i, m := range visible {
		g.Go(func() error {
			uptime, err := api.store.GetMonitorUptime(gctx, m.ServerID, m.ID)
			if err != nil {
				return err
			}
			uptimes[i] = uptime
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	order := make([]string, 0)
	byServer := make(map[string][]publicStatusMonitor)
	for i, m := range visible {
		if _, seen := byServer[m.ServerID]; !seen {
			order = append(order, m.ServerID)
		}
		byServer[m.ServerID] = append(byServer[m.ServerID], publicStatusMonitor{
			ID:              m.ID,
			ServerID:        m.ServerID,
			Name:            m.Name,
			Kind:            m.Kind,
			Status:          m.Status,
			IntervalSeconds: m.IntervalSeconds,
			LastCheckAt:     m.LastCheckAt,
			WarningDays:     m.WarningDays,
			CriticalDays:    m.CriticalDays,
			Certificate:     publicCert(m.Certificate),
			Uptime:          uptimes[i],
		})
	}

	servers := make([]publicStatusServer, 0, len(order))
	for _, serverID := range order {
		server, err := api.lookupServer(ctx, serverID, serverCache)
		if err != nil {
			return nil, err
		}
		servers = append(servers, publicStatusServer{
			ID:          serverID,
			Name:        serverName(server, serverID),
			Environment: server.Environment,
			Tags:        server.Tags,
			Monitors:    byServer[serverID],
		})
	}
	return servers, nil
}

// lookupServer fetches a server once and memoizes it. A missing server is cached
// as a zero value so the overview still renders using the server ID.
func (api *API) lookupServer(ctx context.Context, id string, cache map[string]store.Server) (store.Server, error) {
	if cached, ok := cache[id]; ok {
		return cached, nil
	}
	server, err := api.store.GetServer(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			cache[id] = store.Server{}
			return store.Server{}, nil
		}
		return store.Server{}, err
	}
	cache[id] = server
	return server, nil
}

func serverName(server store.Server, fallback string) string {
	if server.Name != "" {
		return server.Name
	}
	return fallback
}

func publicCert(cert *store.CertificateInfo) *publicCertificate {
	if cert == nil {
		return nil
	}
	return &publicCertificate{
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DaysRemaining: cert.DaysRemaining,
	}
}
