package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListCertificateAccessTokens(ctx context.Context, req *connect.Request[pb.ListCertificateAccessTokensRequest]) (*connect.Response[pb.ListCertificateAccessTokensResponse], error) {
	serverID := strings.TrimSpace(req.Msg.GetServerId())
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("server_id is required"))
	}
	tokens, err := s.store.ListCertificateAccessTokensByServer(ctx, serverID)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListCertificateAccessTokensResponse{}
	for _, t := range tokens {
		out.Tokens = append(out.Tokens, certificateAccessTokenToProto(t))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateCertificateAccessToken(ctx context.Context, req *connect.Request[pb.CreateCertificateAccessTokenRequest]) (*connect.Response[pb.CreateCertificateAccessTokenResponse], error) {
	serverID := strings.TrimSpace(req.Msg.GetServerId())
	name := strings.TrimSpace(req.Msg.GetName())
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("server_id is required"))
	}
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	expiresAt := tsToTimePtr(req.Msg.GetExpiresAt())
	token, plaintext, err := s.store.CreateCertificateAccessToken(ctx, serverID, name, expiresAt)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateCertificateAccessTokenResponse{
		Token:  certificateAccessTokenToProto(token),
		Secret: plaintext,
	}), nil
}

func (s *Service) DeleteCertificateAccessToken(ctx context.Context, req *connect.Request[pb.DeleteCertificateAccessTokenRequest]) (*connect.Response[pb.DeleteCertificateAccessTokenResponse], error) {
	serverID := strings.TrimSpace(req.Msg.GetServerId())
	tokenID := strings.TrimSpace(req.Msg.GetTokenId())
	if serverID == "" || tokenID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("server_id and token_id are required"))
	}
	if err := s.store.DeleteCertificateAccessToken(ctx, serverID, tokenID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteCertificateAccessTokenResponse{}), nil
}
