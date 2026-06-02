package connectapi

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

const (
	// statusOverviewCacheKey is distinct from the legacy REST cache key because
	// the cached payload here is a marshaled protobuf message, not JSON.
	statusOverviewCacheKey = "status:overview:grpc"
	statusOverviewCacheTTL = 10 * time.Second
	statusPageLimit        = 200
	uptimeFetchConcurrency = 16
)

func (s *Service) GetStatusOverview(ctx context.Context, _ *connect.Request[pb.GetStatusOverviewRequest]) (*connect.Response[pb.GetStatusOverviewResponse], error) {
	if cached, found, err := s.store.CacheGet(ctx, statusOverviewCacheKey); err != nil {
		slog.WarnContext(ctx, "status overview cache read failed", "error", err)
	} else if found {
		out := &pb.GetStatusOverviewResponse{}
		if err := proto.Unmarshal(cached, out); err == nil {
			return connect.NewResponse(out), nil
		}
		slog.WarnContext(ctx, "status overview cache decode failed", "error", err)
	}

	groups, _, err := s.store.ListMonitorGroups(ctx, statusPageLimit, "")
	if err != nil {
		return nil, toConnectError(err)
	}

	serverCache := make(map[string]store.Server)
	out := &pb.GetStatusOverviewResponse{Groups: make([]*pb.StatusGroup, 0, len(groups))}

	for _, group := range groups {
		monitors, _, err := s.store.ListMonitorsByGroup(ctx, group.ID, statusPageLimit, "")
		if err != nil {
			return nil, toConnectError(err)
		}
		servers, err := s.buildStatusServers(ctx, monitors, serverCache)
		if err != nil {
			return nil, toConnectError(err)
		}
		out.Groups = append(out.Groups, &pb.StatusGroup{
			Id:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			SortOrder:   group.SortOrder,
			Servers:     servers,
		})
	}

	if payload, err := proto.Marshal(out); err != nil {
		slog.WarnContext(ctx, "status overview cache encode failed", "error", err)
	} else if err := s.store.CacheSet(ctx, statusOverviewCacheKey, payload, statusOverviewCacheTTL); err != nil {
		slog.WarnContext(ctx, "status overview cache write failed", "error", err)
	}

	return connect.NewResponse(out), nil
}

func (s *Service) buildStatusServers(ctx context.Context, monitors []store.Monitor, serverCache map[string]store.Server) ([]*pb.StatusServer, error) {
	visible := make([]store.Monitor, 0, len(monitors))
	for _, m := range monitors {
		if !m.Enabled {
			continue
		}
		server, err := s.lookupServer(ctx, m.ServerID, serverCache)
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
			uptime, err := s.store.GetMonitorUptime(gctx, m.ServerID, m.ID)
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
	byServer := make(map[string][]*pb.StatusMonitor)
	for i, m := range visible {
		if _, seen := byServer[m.ServerID]; !seen {
			order = append(order, m.ServerID)
		}
		byServer[m.ServerID] = append(byServer[m.ServerID], &pb.StatusMonitor{
			Id:              m.ID,
			ServerId:        m.ServerID,
			Name:            m.Name,
			Kind:            m.Kind,
			Status:          m.Status,
			IntervalSeconds: m.IntervalSeconds,
			LastCheckAt:     timeToTS(m.LastCheckAt),
			WarningDays:     m.WarningDays,
			CriticalDays:    m.CriticalDays,
			Certificate:     publicCertToProto(m.Certificate),
			Uptime:          uptimeToProto(uptimes[i]),
		})
	}

	servers := make([]*pb.StatusServer, 0, len(order))
	for _, serverID := range order {
		server, err := s.lookupServer(ctx, serverID, serverCache)
		if err != nil {
			return nil, err
		}
		servers = append(servers, &pb.StatusServer{
			Id:          serverID,
			Name:        statusServerName(server, serverID),
			Environment: server.Environment,
			Tags:        server.Tags,
			Monitors:    byServer[serverID],
		})
	}
	return servers, nil
}

func (s *Service) lookupServer(ctx context.Context, id string, cache map[string]store.Server) (store.Server, error) {
	if cached, ok := cache[id]; ok {
		return cached, nil
	}
	server, err := s.store.GetServer(ctx, id)
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

func statusServerName(server store.Server, fallback string) string {
	if server.Name != "" {
		return server.Name
	}
	return fallback
}

func publicCertToProto(cert *store.CertificateInfo) *pb.PublicCertificate {
	if cert == nil {
		return nil
	}
	return &pb.PublicCertificate{
		NotBefore:     timeToTS(cert.NotBefore),
		NotAfter:      timeToTS(cert.NotAfter),
		DaysRemaining: cert.DaysRemaining,
	}
}
