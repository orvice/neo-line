package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) Login(ctx context.Context, req *connect.Request[pb.LoginRequest]) (*connect.Response[pb.LoginResponse], error) {
	email := req.Msg.GetEmail()
	password := req.Msg.GetPassword()
	if email == "" || password == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email and password are required"))
	}
	user, err := s.store.Authenticate(ctx, email, password)
	if err != nil {
		return nil, toConnectError(err)
	}
	session, err := s.store.CreateSession(ctx, user)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.LoginResponse{
		Token:     session.Token,
		ExpiresAt: timeToTS(session.ExpiresAt),
		User:      userToProto(user.ID, user.Email, user.Role),
	}), nil
}

func (s *Service) Logout(ctx context.Context, _ *connect.Request[pb.LogoutRequest]) (*connect.Response[pb.LogoutResponse], error) {
	session, ok := sessionFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	if err := s.store.DeleteSession(ctx, session.Token); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.LogoutResponse{}), nil
}

func (s *Service) GetCurrentUser(ctx context.Context, _ *connect.Request[pb.GetCurrentUserRequest]) (*connect.Response[pb.GetCurrentUserResponse], error) {
	session, ok := sessionFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	return connect.NewResponse(&pb.GetCurrentUserResponse{
		User: userToProto(session.UserID, session.Email, session.Role),
	}), nil
}
