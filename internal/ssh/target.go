package ssh

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/orvice/neo-line/internal/store"
)

type serverGetter interface {
	GetServer(context.Context, string) (store.Server, error)
}

// ResolveTarget resolves a server's SSH target, applying per-server overrides
// over the runner defaults. It errors when SSH is globally disabled or the
// server has not opted in via ssh.enabled.
func ResolveTarget(ctx context.Context, st serverGetter, runner *Runner, serverID string) (Target, error) {
	logger := slog.Default().With("component", "ssh", "server_id", serverID)
	if runner == nil {
		logger.DebugContext(ctx, "resolve ssh target: runner disabled")
		return Target{}, ErrDisabled
	}
	server, err := st.GetServer(ctx, serverID)
	if err != nil {
		logger.DebugContext(ctx, "resolve ssh target: get server failed", "error", err.Error())
		return Target{}, err
	}
	if server.SSH == nil || !server.SSH.Enabled {
		logger.DebugContext(ctx, "resolve ssh target: ssh not enabled for server")
		return Target{}, fmt.Errorf("ssh is not enabled for server %q", serverID)
	}
	host := server.SSH.Host
	if host == "" {
		host = server.Host
	}
	target := Target{
		Host: host,
		Port: int(server.SSH.Port),
		User: server.SSH.User,
	}
	logger.DebugContext(ctx, "resolved ssh target", "host", target.Host, "port", target.Port, "user", target.User)
	return target, nil
}
