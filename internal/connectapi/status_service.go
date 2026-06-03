package connectapi

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/statusview"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"google.golang.org/protobuf/proto"
)

const (
	// statusOverviewCacheKey is distinct from the legacy REST cache key because
	// the cached payload here is a marshaled protobuf message, not JSON.
	statusOverviewCacheKey = "status:overview:grpc"
	statusOverviewCacheTTL = 10 * time.Second
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

	overview, err := statusview.Build(ctx, s.store)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := statusOverviewToProto(overview)

	if payload, err := proto.Marshal(out); err != nil {
		slog.WarnContext(ctx, "status overview cache encode failed", "error", err)
	} else if err := s.store.CacheSet(ctx, statusOverviewCacheKey, payload, statusOverviewCacheTTL); err != nil {
		slog.WarnContext(ctx, "status overview cache write failed", "error", err)
	}

	return connect.NewResponse(out), nil
}

func statusOverviewToProto(overview statusview.Overview) *pb.GetStatusOverviewResponse {
	out := &pb.GetStatusOverviewResponse{Groups: make([]*pb.StatusGroup, 0, len(overview.Groups))}
	for _, group := range overview.Groups {
		pg := &pb.StatusGroup{
			Id:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			SortOrder:   group.SortOrder,
			Servers:     make([]*pb.StatusServer, 0, len(group.Servers)),
		}
		for _, server := range group.Servers {
			ps := &pb.StatusServer{
				Id:          server.ID,
				Name:        server.Name,
				Environment: server.Environment,
				Tags:        server.Tags,
				Monitors:    make([]*pb.StatusMonitor, 0, len(server.Monitors)),
			}
			for _, monitor := range server.Monitors {
				ps.Monitors = append(ps.Monitors, &pb.StatusMonitor{
					Id:              monitor.ID,
					ServerId:        monitor.ServerID,
					Name:            monitor.Name,
					Kind:            monitor.Kind,
					Status:          monitor.Status,
					IntervalSeconds: monitor.IntervalSeconds,
					LastCheckAt:     timeToTS(monitor.LastCheckAt),
					WarningDays:     monitor.WarningDays,
					CriticalDays:    monitor.CriticalDays,
					Certificate:     publicCertToProto(monitor.Certificate),
					Uptime:          uptimeToProto(monitor.Uptime),
				})
			}
			pg.Servers = append(pg.Servers, ps)
		}
		out.Groups = append(out.Groups, pg)
	}
	return out
}

func publicCertToProto(cert *statusview.PublicCertificate) *pb.PublicCertificate {
	if cert == nil {
		return nil
	}
	return &pb.PublicCertificate{
		NotBefore:     timeToTS(cert.NotBefore),
		NotAfter:      timeToTS(cert.NotAfter),
		DaysRemaining: cert.DaysRemaining,
	}
}
