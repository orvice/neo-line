package connectapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	nlssh "github.com/orvice/neo-line/internal/ssh"
	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) Exec(ctx context.Context, req *connect.Request[pb.SshServiceExecRequest]) (*connect.Response[pb.SshServiceExecResponse], error) {
	command := req.Msg.GetCommand()
	if strings.TrimSpace(command) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("command is required"))
	}
	target, err := s.sshTarget(ctx, req.Msg.GetServerId())
	if err != nil {
		return nil, err
	}
	res, err := s.ssh.Exec(ctx, target, command, time.Duration(req.Msg.GetTimeoutSeconds())*time.Second)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.SshServiceExecResponse{
		ServerId: req.Msg.GetServerId(),
		Host:     target.Host,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: int32(res.ExitCode),
	}), nil
}

func (s *Service) TestConnection(ctx context.Context, req *connect.Request[pb.SshServiceTestConnectionRequest]) (*connect.Response[pb.SshServiceTestConnectionResponse], error) {
	target, err := s.sshTarget(ctx, req.Msg.GetServerId())
	if err != nil {
		return nil, err
	}
	res, err := s.ssh.Exec(ctx, target, "echo neo-line-ssh-ok", 15*time.Second)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.SshServiceTestConnectionResponse{
		ServerId: req.Msg.GetServerId(),
		Host:     target.Host,
		Ok:       res.ExitCode == 0,
		Output:   res.Stdout,
	}), nil
}

func (s *Service) sshTarget(ctx context.Context, serverID string) (nlssh.Target, error) {
	target, err := nlssh.ResolveTarget(ctx, s.store, s.ssh, serverID)
	if err == nil {
		return target, nil
	}
	switch {
	case errors.Is(err, nlssh.ErrDisabled):
		return nlssh.Target{}, connect.NewError(connect.CodeFailedPrecondition, err)
	case store.IsNotFound(err):
		return nlssh.Target{}, toConnectError(err)
	case strings.HasPrefix(err.Error(), "ssh is not enabled for server"):
		return nlssh.Target{}, connect.NewError(connect.CodePermissionDenied, err)
	default:
		return nlssh.Target{}, connect.NewError(connect.CodeInternal, err)
	}
}
