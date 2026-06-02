package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListMcpTokens(ctx context.Context, _ *connect.Request[pb.ListMcpTokensRequest]) (*connect.Response[pb.ListMcpTokensResponse], error) {
	tokens, err := s.store.ListMcpTokens(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListMcpTokensResponse{}
	for _, t := range tokens {
		out.Tokens = append(out.Tokens, mcpTokenToProto(t))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateMcpToken(ctx context.Context, req *connect.Request[pb.CreateMcpTokenRequest]) (*connect.Response[pb.CreateMcpTokenResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	token, plaintext, err := s.store.CreateMcpToken(ctx, name)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateMcpTokenResponse{
		Token:  mcpTokenToProto(token),
		Secret: plaintext,
	}), nil
}

func (s *Service) DeleteMcpToken(ctx context.Context, req *connect.Request[pb.DeleteMcpTokenRequest]) (*connect.Response[pb.DeleteMcpTokenResponse], error) {
	if err := s.store.DeleteMcpToken(ctx, req.Msg.GetTokenId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteMcpTokenResponse{}), nil
}
