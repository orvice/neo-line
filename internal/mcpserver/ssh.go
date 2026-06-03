package mcpserver

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	nlssh "github.com/orvice/neo-line/internal/ssh"
)

type sshExecInput struct {
	ServerID       string `json:"server_id" jsonschema:"the server id to run the command on"`
	Command        string `json:"command" jsonschema:"the shell command to execute on the remote host"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"command timeout in seconds, defaults to 30"`
}

type sshExecOutput struct {
	ServerID string `json:"server_id"`
	Host     string `json:"host"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func (t *tools) sshExec(ctx context.Context, _ *mcp.CallToolRequest, in sshExecInput) (*mcp.CallToolResult, sshExecOutput, error) {
	if in.Command == "" {
		return nil, sshExecOutput{}, errors.New("command is required")
	}
	target, err := t.sshTarget(ctx, in.ServerID)
	if err != nil {
		return nil, sshExecOutput{}, err
	}
	timeout := time.Duration(in.TimeoutSeconds) * time.Second
	res, err := t.ssh.Exec(ctx, target, in.Command, timeout)
	if err != nil {
		return nil, sshExecOutput{}, err
	}
	return nil, sshExecOutput{
		ServerID: in.ServerID,
		Host:     target.Host,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
	}, nil
}

type sshTestOutput struct {
	ServerID string `json:"server_id"`
	Host     string `json:"host"`
	OK       bool   `json:"ok"`
	Output   string `json:"output,omitempty"`
}

func (t *tools) sshTestConnection(ctx context.Context, _ *mcp.CallToolRequest, in serverIDInput) (*mcp.CallToolResult, sshTestOutput, error) {
	target, err := t.sshTarget(ctx, in.ID)
	if err != nil {
		return nil, sshTestOutput{}, err
	}
	res, err := t.ssh.Exec(ctx, target, "echo neo-line-ssh-ok", 15*time.Second)
	if err != nil {
		return nil, sshTestOutput{}, err
	}
	return nil, sshTestOutput{ServerID: in.ID, Host: target.Host, OK: res.ExitCode == 0, Output: res.Stdout}, nil
}

// sshTarget resolves a server's SSH target, applying per-server overrides over
// the global defaults. It errors when SSH is disabled for the server.
func (t *tools) sshTarget(ctx context.Context, serverID string) (nlssh.Target, error) {
	target, err := nlssh.ResolveTarget(ctx, t.store, t.ssh, serverID)
	if err != nil && errors.Is(err, nlssh.ErrDisabled) {
		return nlssh.Target{}, err
	}
	if err != nil {
		return nlssh.Target{}, mapErr(err)
	}
	return target, nil
}
