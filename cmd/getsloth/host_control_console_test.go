package main

import (
	"bytes"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

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
	for _, want := range []string{"GETSLOTH CONTROL", "LIVE", "Remote", "1 connected", "Phone controls", "https://getsloth.dev/s/example", "host-only-password"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("control console output = %q, want %q", out.String(), want)
		}
	}
}
