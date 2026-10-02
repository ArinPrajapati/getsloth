//go:build windows

package main

import (
	"net"
	"testing"
)

func TestDefaultShell_UsesComspec(t *testing.T) {
	t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)
	if got := defaultShell(); got != `C:\Windows\System32\cmd.exe` {
		t.Errorf("defaultShell() = %q", got)
	}
	t.Setenv("COMSPEC", "")
	if got := defaultShell(); got != "cmd.exe" {
		t.Errorf("defaultShell() without COMSPEC = %q, want cmd.exe", got)
	}
}

func TestHostControlServerWindows_RestrictedSocketStillServesCurrentUser(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	defer func() { _ = server.Close() }()

	conn, err := net.Dial("unix", server.socketPath)
	if err != nil {
		t.Fatalf("current user cannot connect to its own control socket: %v", err)
	}
	_ = conn.Close()
}
