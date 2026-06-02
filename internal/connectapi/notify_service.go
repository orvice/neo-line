package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListNotifyGroups(ctx context.Context, req *connect.Request[pb.ListNotifyGroupsRequest]) (*connect.Response[pb.ListNotifyGroupsResponse], error) {
	groups, next, err := s.store.ListNotifyGroups(ctx, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListNotifyGroupsResponse{NextPageToken: next}
	for _, g := range groups {
		out.Groups = append(out.Groups, notifyGroupToProto(g))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateNotifyGroup(ctx context.Context, req *connect.Request[pb.CreateNotifyGroupRequest]) (*connect.Response[pb.CreateNotifyGroupResponse], error) {
	group := notifyGroupFromProto(req.Msg.GetGroup())
	if strings.TrimSpace(group.Name) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	created, err := s.store.CreateNotifyGroup(ctx, group)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateNotifyGroupResponse{Group: notifyGroupToProto(created)}), nil
}

func (s *Service) GetNotifyGroup(ctx context.Context, req *connect.Request[pb.GetNotifyGroupRequest]) (*connect.Response[pb.GetNotifyGroupResponse], error) {
	group, err := s.store.GetNotifyGroup(ctx, req.Msg.GetNotifyGroupId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetNotifyGroupResponse{Group: notifyGroupToProto(group)}), nil
}

func (s *Service) UpdateNotifyGroup(ctx context.Context, req *connect.Request[pb.UpdateNotifyGroupRequest]) (*connect.Response[pb.UpdateNotifyGroupResponse], error) {
	group := notifyGroupFromProto(req.Msg.GetGroup())
	if strings.TrimSpace(group.Name) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	updated, err := s.store.UpdateNotifyGroup(ctx, req.Msg.GetNotifyGroupId(), group)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateNotifyGroupResponse{Group: notifyGroupToProto(updated)}), nil
}

func (s *Service) DeleteNotifyGroup(ctx context.Context, req *connect.Request[pb.DeleteNotifyGroupRequest]) (*connect.Response[pb.DeleteNotifyGroupResponse], error) {
	if err := s.store.DeleteNotifyGroup(ctx, req.Msg.GetNotifyGroupId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteNotifyGroupResponse{}), nil
}
