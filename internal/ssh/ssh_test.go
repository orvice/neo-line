package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestNewDisabledWithoutKeyPath(t *testing.T) {
	runner, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runner != nil {
		t.Fatalf("New() runner = %#v, want nil", runner)
	}
}

func TestNewLoadsDefaultsAndKnownHosts(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	knownHostsPath := filepath.Join(dir, "known_hosts")

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(privateKey, "test@neo-line")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(knownHostsPath, []byte("example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIE9Tg3qPuKySF3H+E5bvvlu38Rbxfr7veUnHJqsBwuKm\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	runner, err := New(Config{KeyPath: keyPath, KnownHostsPath: knownHostsPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runner == nil {
		t.Fatal("New() runner is nil")
	}
	if runner.user != "root" {
		t.Fatalf("runner.user = %q, want root", runner.user)
	}
	if runner.port != 22 {
		t.Fatalf("runner.port = %d, want 22", runner.port)
	}
	if runner.hostKeyCb == nil {
		t.Fatal("runner.hostKeyCb is nil")
	}
}

func TestExecDisabled(t *testing.T) {
	var runner *Runner
	_, err := runner.Exec(t.Context(), Target{Host: "example.com"}, "true", 0)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Exec() error = %v, want ErrDisabled", err)
	}
}

func TestExecRequiresHost(t *testing.T) {
	runner := &Runner{}
	_, err := runner.Exec(t.Context(), Target{}, "true", 0)
	if err == nil || err.Error() != "ssh target host is empty" {
		t.Fatalf("Exec() error = %v, want missing host", err)
	}
}
