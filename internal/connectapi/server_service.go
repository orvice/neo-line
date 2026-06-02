package connectapi

import (
	"context"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListServers(ctx context.Context, req *connect.Request[pb.ListServersRequest]) (*connect.Response[pb.ListServersResponse], error) {
	servers, next, err := s.store.ListServers(ctx, req.Msg.GetEnvironment(), req.Msg.GetTags(), pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListServersResponse{NextPageToken: next}
	for _, sv := range servers {
		out.Servers = append(out.Servers, serverToProto(sv))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateServer(ctx context.Context, req *connect.Request[pb.CreateServerRequest]) (*connect.Response[pb.CreateServerResponse], error) {
	created, err := s.store.CreateServer(ctx, serverFromProto(req.Msg.GetServer()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateServerResponse{Server: serverToProto(created)}), nil
}

func (s *Service) GetServer(ctx context.Context, req *connect.Request[pb.GetServerRequest]) (*connect.Response[pb.GetServerResponse], error) {
	server, err := s.store.GetServer(ctx, req.Msg.GetId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetServerResponse{Server: serverToProto(server)}), nil
}

func (s *Service) UpdateServer(ctx context.Context, req *connect.Request[pb.UpdateServerRequest]) (*connect.Response[pb.UpdateServerResponse], error) {
	updated, err := s.store.UpdateServer(ctx, req.Msg.GetId(), serverFromProto(req.Msg.GetServer()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateServerResponse{Server: serverToProto(updated)}), nil
}

func (s *Service) DeleteServer(ctx context.Context, req *connect.Request[pb.DeleteServerRequest]) (*connect.Response[pb.DeleteServerResponse], error) {
	if err := s.store.DeleteServer(ctx, req.Msg.GetId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteServerResponse{}), nil
}

func (s *Service) GetServerHealth(ctx context.Context, req *connect.Request[pb.GetServerHealthRequest]) (*connect.Response[pb.GetServerHealthResponse], error) {
	health, err := s.store.GetServerHealth(ctx, req.Msg.GetId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetServerHealthResponse{Health: serverHealthToProto(health)}), nil
}

func (s *Service) ListServerEvents(ctx context.Context, req *connect.Request[pb.ListServerEventsRequest]) (*connect.Response[pb.ListServerEventsResponse], error) {
	events, next, err := s.store.ListServerEvents(ctx, req.Msg.GetId(), pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListServerEventsResponse{NextPageToken: next}
	for _, e := range events {
		out.Events = append(out.Events, serverEventToProto(e))
	}
	return connect.NewResponse(out), nil
}
