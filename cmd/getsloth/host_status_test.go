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
