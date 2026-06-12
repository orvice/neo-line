package mcpserver

import (
	"strings"
	"testing"
)

func TestMCPResourceTypeSSH(t *testing.T) {
	if got := mcpResourceType("ssh_exec"); got != "ssh" {
		t.Fatalf("mcpResourceType(ssh_exec) = %q, want ssh", got)
	}
	if got := mcpResourceType("ssh_test_connection"); got != "ssh" {
		t.Fatalf("mcpResourceType(ssh_test_connection) = %q, want ssh", got)
	}
}

func TestCommandField(t *testing.T) {
	in := sshExecInput{ServerID: "srv1", Command: "uptime"}
	if got := commandField(in); got != "uptime" {
		t.Fatalf("commandField() = %q, want uptime", got)
	}
	if got := commandField(&in); got != "uptime" {
		t.Fatalf("commandField(pointer) = %q, want uptime", got)
	}
	if got := commandField(struct{ ServerID string }{"srv1"}); got != "" {
		t.Fatalf("commandField(no Command) = %q, want empty", got)
	}
	long := strings.Repeat("a", 600)
	got := commandField(sshExecInput{Command: long})
	if !strings.HasSuffix(got, "…") || len(got) > 510 {
		t.Fatalf("commandField(long) not truncated: len=%d", len(got))
	}
}
