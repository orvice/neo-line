package mcpserver

import (
	"context"
	"errors"
	"testing"

	nlssh "github.com/orvice/neo-line/internal/ssh"
	"github.com/orvice/neo-line/internal/store"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type sshTargetStore struct {
	store.Store
	server store.Server
	err    error
}

func (s sshTargetStore) GetServer(context.Context, string) (store.Server, error) {
	return s.server, s.err
}

func TestSSHTargetResolvesOverrides(t *testing.T) {
	tls := &tools{
		store: sshTargetStore{
			server: store.Server{
				ID:   "srv_1",
				Host: "10.0.0.10",
				SSH:  &store.ServerSSH{Enabled: true, Host: "10.0.0.11", Port: 2222, User: "ops"},
			},
		},
		ssh: &nlssh.Runner{},
	}

	target, err := tls.sshTarget(t.Context(), "srv_1")
	if err != nil {
		t.Fatalf("sshTarget() error = %v", err)
	}
	if target.Host != "10.0.0.11" || target.Port != 2222 || target.User != "ops" {
		t.Fatalf("sshTarget() = %#v", target)
	}
}

func TestSSHTargetFallsBackToServerHost(t *testing.T) {
	tls := &tools{
		store: sshTargetStore{
			server: store.Server{
				ID:   "srv_1",
				Host: "10.0.0.10",
				SSH:  &store.ServerSSH{Enabled: true},
			},
		},
		ssh: &nlssh.Runner{},
	}

	target, err := tls.sshTarget(t.Context(), "srv_1")
	if err != nil {
		t.Fatalf("sshTarget() error = %v", err)
	}
	if target.Host != "10.0.0.10" || target.Port != 0 || target.User != "" {
		t.Fatalf("sshTarget() = %#v", target)
	}
}

func TestSSHTargetRequiresRunnerAndServerOptIn(t *testing.T) {
	t.Run("runner disabled", func(t *testing.T) {
		tls := &tools{store: sshTargetStore{}}
		_, err := tls.sshTarget(t.Context(), "srv_1")
		if !errors.Is(err, nlssh.ErrDisabled) {
			t.Fatalf("sshTarget() error = %v, want ErrDisabled", err)
		}
	})

	t.Run("server disabled", func(t *testing.T) {
		tls := &tools{
			store: sshTargetStore{server: store.Server{ID: "srv_1", Host: "10.0.0.10"}},
			ssh:   &nlssh.Runner{},
		}
		_, err := tls.sshTarget(t.Context(), "srv_1")
		if err == nil || err.Error() != `ssh is not enabled for server "srv_1"` {
			t.Fatalf("sshTarget() error = %v, want disabled server", err)
		}
	})
}

func TestSSHTargetMapsStoreErrors(t *testing.T) {
	tls := &tools{
		store: sshTargetStore{err: mongo.ErrNoDocuments},
		ssh:   &nlssh.Runner{},
	}
	_, err := tls.sshTarget(t.Context(), "missing")
	if err == nil {
		t.Fatal("sshTarget() error is nil")
	}
}
