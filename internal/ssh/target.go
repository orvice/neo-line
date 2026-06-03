package ssh

import (
	"context"
	"fmt"

	"github.com/orvice/neo-line/internal/store"
)

type serverGetter interface {
	GetServer(context.Context, string) (store.Server, error)
}

// ResolveTarget resolves a server's SSH target, applying per-server overrides
// over the runner defaults. It errors when SSH is globally disabled or the
// server has not opted in via ssh.enabled.
func ResolveTarget(ctx context.Context, st serverGetter, runner *Runner, serverID string) (Target, error) {
	if runner == nil {
		return Target{}, ErrDisabled
	}
	server, err := st.GetServer(ctx, serverID)
	if err != nil {
		return Target{}, err
	}
	if server.SSH == nil || !server.SSH.Enabled {
		return Target{}, fmt.Errorf("ssh is not enabled for server %q", serverID)
	}
	host := server.SSH.Host
	if host == "" {
		host = server.Host
	}
	return Target{
		Host: host,
		Port: int(server.SSH.Port),
		User: server.SSH.User,
	}, nil
}
