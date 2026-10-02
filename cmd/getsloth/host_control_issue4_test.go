package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestIssue4_HostControlStateUpdatesReachSnapshot(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeRemote, &bytes.Buffer{})
	status.setInvite("https://getsloth.dev/s/session", "host-password")
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host", IsActiveWriter: true},
		{ID: "phone", Role: "viewer", DisplayName: "Phone"},
	}})
	status.updateControl(protocol.ControlChangedMsg{
		ActiveWriterID:   "phone",
		ActiveWriterRole: "viewer",
	})

	snapshot := status.snapshot()
	if !snapshot.Live || snapshot.Mode != protocol.SessionModeRemote {
		t.Fatalf("session state = %+v, want live remote session", snapshot)
	}
	if snapshot.InviteURL != "https://getsloth.dev/s/session" || snapshot.Password != "host-password" {
		t.Fatalf("invite data = %q/%q, want configured invite", snapshot.InviteURL, snapshot.Password)
	}
	if snapshot.ControllerID != "phone" || snapshot.ControllerRole != "viewer" {
		t.Fatalf("controller = %q/%q, want phone/viewer", snapshot.ControllerID, snapshot.ControllerRole)
	}
	if len(snapshot.Viewers) != 1 || snapshot.Viewers[0].ID != "phone" || !snapshot.Viewers[0].IsController {
		t.Fatalf("viewers = %+v, want controlling phone viewer", snapshot.Viewers)
	}

	status.updateControl(protocol.ControlChangedMsg{ActiveWriterID: "host", ActiveWriterRole: "host"})
	snapshot = status.snapshot()
	if snapshot.ControllerID != "host" || snapshot.ControllerRole != "host" {
		t.Fatalf("reclaimed controller = %q/%q, want host/host", snapshot.ControllerID, snapshot.ControllerRole)
	}
	if len(snapshot.Events) < 2 || snapshot.Events[0] != "Host reclaimed control" || snapshot.Events[1] != "Phone took control" {
		t.Fatalf("state transition events = %#v, want reclaim after viewer takeover", snapshot.Events)
	}
}

func TestIssue4_ConsoleReclaimAndKillActionsUpdateHostState(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeRemote, &bytes.Buffer{})
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host"},
		{ID: "phone", Role: "viewer", DisplayName: "Phone", IsActiveWriter: true},
	}})
	status.updateControl(protocol.ControlChangedMsg{ActiveWriterID: "phone", ActiveWriterRole: "viewer"})

	server, err := startHostControlServer(status.snapshot)
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	server.setActions(
		func() error {
			status.updateControl(protocol.ControlChangedMsg{ActiveWriterID: "host", ActiveWriterRole: "host"})
			return nil
		},
		func() error {
			status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
				{ID: "host", Role: "host", IsActiveWriter: true},
			}})
			status.noteKillSwitch()
			return nil
		},
	)

	var out bytes.Buffer
	if code := runHostControlConsole([]string{"--socket", server.socketPath, "--action", "reclaim"}, &out); code != 0 {
		t.Fatalf("reclaim console exit code = %d, output = %q", code, out.String())
	}
	if snapshot := status.snapshot(); snapshot.ControllerRole != "host" || snapshot.ControllerID != "host" {
		t.Fatalf("after reclaim, controller = %q/%q, want host/host", snapshot.ControllerID, snapshot.ControllerRole)
	}

	out.Reset()
	if code := runHostControlConsole([]string{"--socket", server.socketPath, "--action", "kill"}, &out); code != 0 {
		t.Fatalf("kill console exit code = %d, output = %q", code, out.String())
	}
	snapshot := status.snapshot()
	if len(snapshot.Viewers) != 0 {
		t.Fatalf("after kill, viewers = %+v, want none", snapshot.Viewers)
	}
	if len(snapshot.Events) == 0 || !strings.Contains(snapshot.Events[0], "kill switch") {
		t.Fatalf("after kill, events = %#v, want kill-switch event", snapshot.Events)
	}
}

func TestIssue4_InviteSnapshotIsAvailableThroughLocalChannel(t *testing.T) {
	want := hostControlSnapshot{
		Live:      true,
		Mode:      protocol.SessionModeGroup,
		InviteURL: "https://getsloth.dev/s/group-session",
		Password:  "invite-password",
	}
	server, err := startHostControlServer(func() hostControlSnapshot { return want })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	got, err := requestHostControlSnapshot(server.socketPath, "snapshot")
	if err != nil {
		t.Fatalf("requestHostControlSnapshot: %v", err)
	}
	if got.InviteURL != want.InviteURL || got.Password != want.Password {
		t.Fatalf("invite snapshot = %q/%q, want %q/%q", got.InviteURL, got.Password, want.InviteURL, want.Password)
	}
}

func TestIssue4_ConsoleDisconnectDoesNotEndHostSession(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{Live: true, Mode: protocol.SessionModeRemote}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}

	var out bytes.Buffer
	if code := runHostControlConsole([]string{"--socket", server.socketPath}, &out); code != 0 {
		_ = server.Close()
		t.Fatalf("console exit code = %d, output = %q", code, out.String())
	}

	snapshot, err := requestHostControlSnapshot(server.socketPath, "snapshot")
	if err != nil {
		_ = server.Close()
		t.Fatalf("snapshot after console disconnect: %v", err)
	}
	if !snapshot.Live {
		_ = server.Close()
		t.Fatal("host session became inactive when console disconnected")
	}

	if err := server.Close(); err != nil {
		t.Fatalf("close host control server: %v", err)
	}
	if _, err := requestHostControlSnapshot(server.socketPath, "snapshot"); err == nil {
		t.Fatal("snapshot succeeded after host control channel closed")
	}
}

func TestIssue4_ClosingTUIOnlyQuitsConsole(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{Live: true, Mode: protocol.SessionModeRemote}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	model := newHostControlTUI(server.socketPath, &bytes.Buffer{}, hostControlSnapshot{Live: true})
	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("q")}))
	if updated.(hostControlTUI).socketPath != server.socketPath {
		t.Fatal("closing console changed the host control channel")
	}
	if cmd == nil {
		t.Fatal("closing console did not return a quit command")
	}
	quitMessage := cmd()
	if _, ok := quitMessage.(tea.QuitMsg); !ok {
		t.Fatalf("closing console command returned %T, want tea.QuitMsg", quitMessage)
	}
	if _, err := requestHostControlSnapshot(server.socketPath, "snapshot"); err != nil {
		t.Fatalf("host control channel unavailable after console quit: %v", err)
	}
}

func TestIssue4_UnavailableActionIsReportedOverLocalChannel(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{Live: true} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	_, err = requestHostControlSnapshot(server.socketPath, "reclaim")
	var controlErr *hostControlError
	if !errors.As(err, &controlErr) {
		t.Fatalf("request unavailable action error = %v, want hostControlError", err)
	}
	if controlErr.Error() != "host control action is unavailable" {
		t.Fatalf("request unavailable action error = %q", controlErr.Error())
	}
}
