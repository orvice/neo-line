package connectapi

import (
	"context"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) GetSettings(ctx context.Context, _ *connect.Request[pb.GetSettingsRequest]) (*connect.Response[pb.GetSettingsResponse], error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetSettingsResponse{Settings: settingsToProto(settings)}), nil
}

func (s *Service) UpdateSettings(ctx context.Context, req *connect.Request[pb.UpdateSettingsRequest]) (*connect.Response[pb.UpdateSettingsResponse], error) {
	updated, err := s.store.UpdateSettings(ctx, settingsFromProto(req.Msg.GetSettings()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateSettingsResponse{Settings: settingsToProto(updated)}), nil
}
