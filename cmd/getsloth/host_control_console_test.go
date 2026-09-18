package main

import (
	"bytes"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestHostControlKeyAction_MapsVisibleControls(t *testing.T) {
	for key, want := range map[byte]string{'r': "reclaim", 'R': "reclaim", 'k': "kill", 'K': "kill", 'q': "quit", 'i': "snapshot", 'c': "copy", 'C': "copy"} {
		if got := hostControlKeyAction(key); got != want {
			t.Errorf("hostControlKeyAction(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestHostControlArguments_RecognizesWatchMode(t *testing.T) {
	socketPath, action, watch, ok := hostControlArguments([]string{"--socket", "/tmp/getsloth.sock", "--watch"})
	if !ok || socketPath != "/tmp/getsloth.sock" || action != "snapshot" || !watch {
		t.Errorf("hostControlArguments() = %q, %q, %t, %t", socketPath, action, watch, ok)
	}
}

func TestRunHostControlConsole_SendsHostAction(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	called := false
	server.setActions(func() error {
		called = true
		return nil
	}, nil)

	var out bytes.Buffer
	if code := runHostControlConsole([]string{"--socket", server.socketPath, "--action", "reclaim"}, &out); code != 0 {
		t.Fatalf("runHostControlConsole exit code = %d, want 0; output = %q", code, out.String())
	}
	if !called {
		t.Fatal("reclaim action was not sent to the host control server")
	}
}

func TestRunHostControlConsole_PrintsLiveSessionSnapshot(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{
			Live:           true,
			Mode:           protocol.SessionModeRemote,
			InviteURL:      "https://getsloth.dev/s/example",
			Password:       "host-only-password",
			ControllerID:   "viewer-1",
			ControllerRole: "viewer",
			Viewers: []hostControlViewer{{
				ID:           "viewer-1",
				Name:         "Phone",
				IsController: true,
			}},
			Events: []string{"Phone joined", "Phone took control"},
		}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var out bytes.Buffer
	if code := runHostControlConsole([]string{"--socket", server.socketPath}, &out); code != 0 {
		t.Fatalf("runHostControlConsole exit code = %d, want 0; output = %q", code, out.String())
	}
	for _, want := range []string{"[r] RECLAIM", "[k] KILL VIEWERS", "[i] INVITE", "[c] COPY", "[q] CLOSE", "GETSLOTH // HOST CONTROL", "LIVE", "Remote", "Phone", "controlling", "Invite link ready", "Phone joined", "Phone took control"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("control console output = %q, want %q", out.String(), want)
		}
	}
}
