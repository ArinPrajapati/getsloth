package main

import (
	"encoding/json"
	"net"
	"os"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestHostControlServer_RequiresSnapshot(t *testing.T) {
	if _, err := startHostControlServer(nil); err == nil {
		t.Fatal("startHostControlServer(nil) succeeded, want an error")
	}
}

func TestHostControlServer_RejectsUnknownActionsAndCleansUpSocket(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}

	conn, err := net.Dial("unix", server.socketPath)
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}
	if err := json.NewEncoder(conn).Encode(hostControlRequest{Action: "viewer_take_control"}); err != nil {
		t.Fatalf("write unknown request: %v", err)
	}
	var response hostControlResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("read unknown response: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close control client: %v", err)
	}
	if response.Error != "unknown host control action" {
		t.Errorf("response error = %q", response.Error)
	}

	socketPath := server.socketPath
	if err := server.Close(); err != nil {
		t.Fatalf("close control server: %v", err)
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("socket still exists after close, stat error = %v", err)
	}
}

func TestHostControlServer_InvokesOnlyConfiguredHostActions(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var reclaimed, killed int
	server.setActions(
		func() error {
			reclaimed++
			return nil
		},
		func() error {
			killed++
			return nil
		},
	)

	for _, action := range []string{"reclaim", "kill"} {
		conn, err := net.Dial("unix", server.socketPath)
		if err != nil {
			t.Fatalf("dial control socket: %v", err)
		}
		if err := json.NewEncoder(conn).Encode(hostControlRequest{Action: action}); err != nil {
			t.Fatalf("write %s request: %v", action, err)
		}
		var response hostControlResponse
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			t.Fatalf("read %s response: %v", action, err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close %s client: %v", action, err)
		}
		if response.Error != "" {
			t.Errorf("%s response error = %q", action, response.Error)
		}
	}

	if reclaimed != 1 || killed != 1 {
		t.Errorf("actions invoked reclaim=%d kill=%d, want 1 each", reclaimed, killed)
	}
}

func TestHostControlServer_ReturnsLiveSnapshotOverPrivateSocket(t *testing.T) {
	want := hostControlSnapshot{
		Live:           true,
		Mode:           protocol.SessionModeRemote,
		ControllerID:   "viewer-1",
		ControllerRole: "viewer",
		Viewers: []hostControlViewer{{
			ID:           "viewer-1",
			Name:         "Phone",
			IsController: true,
		}},
	}
	server, err := startHostControlServer(func() hostControlSnapshot { return want })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	info, err := os.Stat(server.socketPath)
	if err != nil {
		t.Fatalf("stat control socket: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("control socket permissions = %o, want 600", info.Mode().Perm())
	}

	conn, err := net.Dial("unix", server.socketPath)
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := json.NewEncoder(conn).Encode(hostControlRequest{Action: "snapshot"}); err != nil {
		t.Fatalf("write snapshot request: %v", err)
	}
	var got hostControlResponse
	if err := json.NewDecoder(conn).Decode(&got); err != nil {
		t.Fatalf("read snapshot response: %v", err)
	}
	if got.Error != "" {
		t.Fatalf("control server error = %q", got.Error)
	}
	if got.Snapshot == nil {
		t.Fatal("control response did not include a snapshot")
	}
	if got.Snapshot.ControllerID != want.ControllerID || len(got.Snapshot.Viewers) != 1 || got.Snapshot.Viewers[0].Name != "Phone" {
		t.Errorf("snapshot = %+v, want %+v", got.Snapshot, want)
	}
}
