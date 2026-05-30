// Package ssh provides a thin SSH command runner backed by a single local
// private key loaded from the runtime configuration. It is used by the MCP
// server to execute commands on monitored hosts.
package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ErrDisabled is returned by Runner methods when SSH is not configured.
var ErrDisabled = errors.New("ssh is not configured")

// Config is the global SSH configuration sourced from runtime config.
type Config struct {
	// KeyPath is the path to the local private key. Empty disables SSH.
	KeyPath string
	// User is the default SSH user when a server does not override it.
	User string
	// Port is the default SSH port when a server does not override it.
	Port int
	// KnownHostsPath enables host key verification. When empty, host keys are
	// not verified (unrestricted mode).
	KnownHostsPath string
}

// Target identifies a single SSH endpoint. Empty fields fall back to the
// Runner defaults; Host has no default and must be set.
type Target struct {
	Host string
	Port int
	User string
}

// Result is the outcome of a single remote command execution.
type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Runner executes commands over SSH using one preloaded signer.
type Runner struct {
	signer    ssh.Signer
	user      string
	port      int
	hostKeyCb ssh.HostKeyCallback
}

// New builds a Runner from the global config. When cfg.KeyPath is empty it
// returns (nil, nil): SSH features are disabled and callers should treat a nil
// Runner as "not configured".
func New(cfg Config) (*Runner, error) {
	if cfg.KeyPath == "" {
		return nil, nil
	}
	keyBytes, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh key %q: %w", cfg.KeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key %q: %w", cfg.KeyPath, err)
	}

	hostKeyCb := ssh.InsecureIgnoreHostKey()
	if cfg.KnownHostsPath != "" {
		hostKeyCb, err = knownhosts.New(cfg.KnownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("load known_hosts %q: %w", cfg.KnownHostsPath, err)
		}
	}

	port := cfg.Port
	if port == 0 {
		port = 22
	}
	user := cfg.User
	if user == "" {
		user = "root"
	}
	return &Runner{signer: signer, user: user, port: port, hostKeyCb: hostKeyCb}, nil
}

// Exec dials the target and runs command, returning its stdout, stderr, and
// exit code. A non-zero exit code is reported in Result, not as an error; an
// error is returned only for connection or protocol failures.
func (r *Runner) Exec(ctx context.Context, target Target, command string, timeout time.Duration) (Result, error) {
	if r == nil {
		return Result{}, ErrDisabled
	}
	if target.Host == "" {
		return Result{}, errors.New("ssh target host is empty")
	}
	user := target.User
	if user == "" {
		user = r.user
	}
	port := target.Port
	if port == 0 {
		port = r.port
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(r.signer)},
		HostKeyCallback: r.hostKeyCb,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(target.Host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Result{}, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientCfg)
	if err != nil {
		return Result{}, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}, ctx.Err()
	case runErr := <-done:
		res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitStatus()
			return res, nil
		}
		if runErr != nil {
			return res, runErr
		}
		return res, nil
	}
}
