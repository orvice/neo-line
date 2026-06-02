package connectapi

import (
	"context"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListMonitors(ctx context.Context, req *connect.Request[pb.ListMonitorsRequest]) (*connect.Response[pb.ListMonitorsResponse], error) {
	monitors, next, err := s.store.ListMonitors(ctx, req.Msg.GetServerId(), pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListMonitorsResponse{NextPageToken: next}
	for _, m := range monitors {
		out.Monitors = append(out.Monitors, monitorToProto(m))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateMonitor(ctx context.Context, req *connect.Request[pb.CreateMonitorRequest]) (*connect.Response[pb.CreateMonitorResponse], error) {
	created, err := s.store.CreateMonitor(ctx, req.Msg.GetServerId(), monitorFromProto(req.Msg.GetMonitor()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateMonitorResponse{Monitor: monitorToProto(created)}), nil
}

func (s *Service) GetMonitor(ctx context.Context, req *connect.Request[pb.GetMonitorRequest]) (*connect.Response[pb.GetMonitorResponse], error) {
	monitor, err := s.store.GetMonitor(ctx, req.Msg.GetServerId(), req.Msg.GetMonitorId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetMonitorResponse{Monitor: monitorToProto(monitor)}), nil
}

func (s *Service) UpdateMonitor(ctx context.Context, req *connect.Request[pb.UpdateMonitorRequest]) (*connect.Response[pb.UpdateMonitorResponse], error) {
	updated, err := s.store.UpdateMonitor(ctx, req.Msg.GetServerId(), req.Msg.GetMonitorId(), monitorFromProto(req.Msg.GetMonitor()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateMonitorResponse{Monitor: monitorToProto(updated)}), nil
}

func (s *Service) DeleteMonitor(ctx context.Context, req *connect.Request[pb.DeleteMonitorRequest]) (*connect.Response[pb.DeleteMonitorResponse], error) {
	if err := s.store.DeleteMonitor(ctx, req.Msg.GetServerId(), req.Msg.GetMonitorId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteMonitorResponse{}), nil
}

func (s *Service) ListCheckResults(ctx context.Context, req *connect.Request[pb.ListCheckResultsRequest]) (*connect.Response[pb.ListCheckResultsResponse], error) {
	results, next, err := s.store.ListCheckResults(ctx, req.Msg.GetServerId(), req.Msg.GetMonitorId(), pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken(), tsToTimePtr(req.Msg.GetStartTime()), tsToTimePtr(req.Msg.GetEndTime()))
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListCheckResultsResponse{NextPageToken: next}
	for _, r := range results {
		out.Results = append(out.Results, checkResultToProto(r))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) GetMonitorUptime(ctx context.Context, req *connect.Request[pb.GetMonitorUptimeRequest]) (*connect.Response[pb.GetMonitorUptimeResponse], error) {
	uptime, err := s.store.GetMonitorUptime(ctx, req.Msg.GetServerId(), req.Msg.GetMonitorId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetMonitorUptimeResponse{Uptime: uptimeToProto(uptime)}), nil
}
