package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestHostSessionStatus_SummarizesPresenceAndRemoteController(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host"},
		{ID: "viewer-1", Role: "viewer", DisplayName: "Phone", IsActiveWriter: true},
	}})
	status.updateControl(protocol.ControlChangedMsg{
		ActiveWriterID:   "viewer-1",
		ActiveWriterRole: "viewer",
	})

	summary := status.summary()
	for _, want := range []string{"LIVE", "Remote", "1 viewer", "Phone controls"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q does not contain %q", summary, want)
		}
	}
	if !strings.Contains(out.String(), "\x1b]0;getsloth") {
		t.Fatalf("terminal title was not updated: %q", out.String())
	}
}

func TestHostSessionStatus_GroupModeExplainsHostControl(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeGroup, &out)
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host", IsActiveWriter: true},
		{ID: "viewer-1", Role: "viewer", DisplayName: "Phone"},
		{ID: "viewer-2", Role: "viewer", DisplayName: "Laptop"},
	}})

	summary := status.summary()
	for _, want := range []string{"Group", "2 viewers", "host controls"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q does not contain %q", summary, want)
		}
	}
}

func TestHostSessionStatus_SnapshotProvidesConsoleData(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)
	status.setInvite("https://getsloth.dev/s/example", "host-only-password")
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host"},
		{ID: "viewer-2", Role: "viewer", DisplayName: "Phone", IsActiveWriter: true},
		{ID: "viewer-1", Role: "viewer", DisplayName: "Laptop"},
	}})

	snapshot := status.snapshot()
	if !snapshot.Live || snapshot.Mode != protocol.SessionModeRemote {
		t.Fatalf("snapshot session = %+v, want a live remote session", snapshot)
	}
	if snapshot.InviteURL != "https://getsloth.dev/s/example" || snapshot.Password != "host-only-password" {
		t.Errorf("snapshot invite = %q/%q", snapshot.InviteURL, snapshot.Password)
	}
	if snapshot.ControllerID != "viewer-2" || snapshot.ControllerRole != "viewer" {
		t.Errorf("snapshot controller = %q/%q, want viewer-2/viewer", snapshot.ControllerID, snapshot.ControllerRole)
	}
	if len(snapshot.Viewers) != 2 {
		t.Fatalf("snapshot viewers = %+v, want two viewers", snapshot.Viewers)
	}
	if snapshot.Viewers[0].ID != "viewer-1" || snapshot.Viewers[0].Name != "Laptop" {
		t.Errorf("first viewer = %+v, want sorted Laptop viewer", snapshot.Viewers[0])
	}
	if !snapshot.Viewers[1].IsController {
		t.Errorf("controller viewer = %+v, want IsController true", snapshot.Viewers[1])
	}
	if len(snapshot.Events) == 0 || snapshot.Events[0] != "Phone joined" {
		t.Errorf("snapshot events = %#v, want Phone joined", snapshot.Events)
	}
}

func TestHostSessionStatus_RecordsControlChangesAsEvents(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host", IsActiveWriter: true},
		{ID: "viewer-1", Role: "viewer", DisplayName: "Phone"},
	}})

	status.updateControl(protocol.ControlChangedMsg{ActiveWriterID: "viewer-1", ActiveWriterRole: "viewer"})
	status.updateControl(protocol.ControlChangedMsg{ActiveWriterID: "host", ActiveWriterRole: "host"})

	events := status.snapshot().Events
	if len(events) < 2 || events[0] != "Host reclaimed control" || events[1] != "Phone took control" {
		t.Errorf("events = %#v", events)
	}
}

func TestHostSessionStatus_PrintWritesReadableStatus(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)

	status.print()

	if !strings.Contains(out.String(), "getsloth: LIVE") {
		t.Fatalf("printed status = %q", out.String())
	}
}

func TestHostSessionStatus_DisconnectedAndSanitizesViewerNames(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "viewer-1", Role: "viewer", DisplayName: "Phone\x1b]0;forged\a", IsActiveWriter: true},
	}})
	status.setDisconnected()

	summary := status.summary()
	if !strings.Contains(summary, "OFFLINE") {
		t.Fatalf("summary = %q, want OFFLINE", summary)
	}
	if strings.Contains(out.String(), "\x1b]0;forged") || strings.Contains(out.String(), "\a\a") {
		t.Fatalf("viewer name injected terminal controls: %q", out.String())
	}
}
