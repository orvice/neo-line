package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListMonitorGroups(ctx context.Context, req *connect.Request[pb.ListMonitorGroupsRequest]) (*connect.Response[pb.ListMonitorGroupsResponse], error) {
	groups, next, err := s.store.ListMonitorGroups(ctx, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListMonitorGroupsResponse{NextPageToken: next}
	for _, g := range groups {
		out.Groups = append(out.Groups, monitorGroupToProto(g))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateMonitorGroup(ctx context.Context, req *connect.Request[pb.CreateMonitorGroupRequest]) (*connect.Response[pb.CreateMonitorGroupResponse], error) {
	group := monitorGroupFromProto(req.Msg.GetGroup())
	if strings.TrimSpace(group.Name) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	created, err := s.store.CreateMonitorGroup(ctx, group)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateMonitorGroupResponse{Group: monitorGroupToProto(created)}), nil
}

func (s *Service) GetMonitorGroup(ctx context.Context, req *connect.Request[pb.GetMonitorGroupRequest]) (*connect.Response[pb.GetMonitorGroupResponse], error) {
	group, err := s.store.GetMonitorGroup(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetMonitorGroupResponse{Group: monitorGroupToProto(group)}), nil
}

func (s *Service) UpdateMonitorGroup(ctx context.Context, req *connect.Request[pb.UpdateMonitorGroupRequest]) (*connect.Response[pb.UpdateMonitorGroupResponse], error) {
	group := monitorGroupFromProto(req.Msg.GetGroup())
	if strings.TrimSpace(group.Name) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	updated, err := s.store.UpdateMonitorGroup(ctx, req.Msg.GetGroupId(), group)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateMonitorGroupResponse{Group: monitorGroupToProto(updated)}), nil
}

func (s *Service) DeleteMonitorGroup(ctx context.Context, req *connect.Request[pb.DeleteMonitorGroupRequest]) (*connect.Response[pb.DeleteMonitorGroupResponse], error) {
	if err := s.store.DeleteMonitorGroup(ctx, req.Msg.GetGroupId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteMonitorGroupResponse{}), nil
}

func (s *Service) ListMonitorsByGroup(ctx context.Context, req *connect.Request[pb.ListMonitorsByGroupRequest]) (*connect.Response[pb.ListMonitorsByGroupResponse], error) {
	monitors, next, err := s.store.ListMonitorsByGroup(ctx, req.Msg.GetGroupId(), pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListMonitorsByGroupResponse{NextPageToken: next}
	for _, m := range monitors {
		out.Monitors = append(out.Monitors, monitorToProto(m))
	}
	return connect.NewResponse(out), nil
}
