package statusview

import (
	"context"
	"time"

	"github.com/orvice/neo-line/internal/store"
	"golang.org/x/sync/errgroup"
)

const (
	PageLimit              = 200
	UptimeFetchConcurrency = 16
)

type PublicCertificate struct {
	NotBefore     time.Time `json:"not_before,omitempty"`
	NotAfter      time.Time `json:"not_after,omitempty"`
	DaysRemaining int32     `json:"days_remaining,omitempty"`
}

type Monitor struct {
	ID              string              `json:"id"`
	ServerID        string              `json:"server_id"`
	Name            string              `json:"name"`
	Kind            string              `json:"kind"`
	Status          string              `json:"status"`
	IntervalSeconds uint32              `json:"interval_seconds"`
	LastCheckAt     time.Time           `json:"last_check_at,omitempty"`
	WarningDays     uint32              `json:"warning_days,omitempty"`
	CriticalDays    uint32              `json:"critical_days,omitempty"`
	Certificate     *PublicCertificate  `json:"certificate,omitempty"`
	Uptime          store.MonitorUptime `json:"uptime"`
}

type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Environment string    `json:"environment,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Monitors    []Monitor `json:"monitors"`
}

type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	SortOrder   uint32   `json:"sort_order"`
	Servers     []Server `json:"servers"`
}

type Overview struct {
	Groups []Group `json:"groups"`
}

func Build(ctx context.Context, st store.Store) (Overview, error) {
	groups, _, err := st.ListMonitorGroups(ctx, PageLimit, "")
	if err != nil {
		return Overview{}, err
	}

	serverCache := make(map[string]store.Server)
	out := Overview{Groups: make([]Group, 0, len(groups))}
	for _, group := range groups {
		monitors, _, err := st.ListMonitorsByGroup(ctx, group.ID, PageLimit, "")
		if err != nil {
			return Overview{}, err
		}
		servers, err := buildServers(ctx, st, monitors, serverCache)
		if err != nil {
			return Overview{}, err
		}
		out.Groups = append(out.Groups, Group{
			ID:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			SortOrder:   group.SortOrder,
			Servers:     servers,
		})
	}
	return out, nil
}

func buildServers(ctx context.Context, st store.Store, monitors []store.Monitor, serverCache map[string]store.Server) ([]Server, error) {
	visible := make([]store.Monitor, 0, len(monitors))
	for _, m := range monitors {
		if !m.Enabled {
			continue
		}
		server, err := lookupServer(ctx, st, m.ServerID, serverCache)
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
	g.SetLimit(UptimeFetchConcurrency)
	for i, m := range visible {
		g.Go(func() error {
			uptime, err := st.GetMonitorUptime(gctx, m.ServerID, m.ID)
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
	byServer := make(map[string][]Monitor)
	for i, m := range visible {
		if _, seen := byServer[m.ServerID]; !seen {
			order = append(order, m.ServerID)
		}
		byServer[m.ServerID] = append(byServer[m.ServerID], Monitor{
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

	servers := make([]Server, 0, len(order))
	for _, serverID := range order {
		server, err := lookupServer(ctx, st, serverID, serverCache)
		if err != nil {
			return nil, err
		}
		servers = append(servers, Server{
			ID:          serverID,
			Name:        serverName(server, serverID),
			Environment: server.Environment,
			Tags:        server.Tags,
			Monitors:    byServer[serverID],
		})
	}
	return servers, nil
}

func lookupServer(ctx context.Context, st store.Store, id string, cache map[string]store.Server) (store.Server, error) {
	if cached, ok := cache[id]; ok {
		return cached, nil
	}
	server, err := st.GetServer(ctx, id)
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

func publicCert(cert *store.CertificateInfo) *PublicCertificate {
	if cert == nil {
		return nil
	}
	return &PublicCertificate{
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DaysRemaining: cert.DaysRemaining,
	}
}
