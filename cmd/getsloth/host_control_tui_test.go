package main

import (
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderHostControlDashboard_ContainsLongInviteWithinTerminalWidth(t *testing.T) {
	snapshot := hostControlSnapshot{
		Live:           true,
		Mode:           protocol.SessionModeRemote,
		InviteURL:      "https://127.0.0.1:5173/s/very-long-session-id#k=very-long-public-key-that-must-not-break-the-layout",
		Password:       "sloth-demo",
		ControllerRole: "host",
		Viewers:        []hostControlViewer{{ID: "phone", Name: "Phone"}},
		Events:         []string{"Phone joined", "Host reclaimed control"},
	}

	const width = 72
	view := renderHostControlDashboard(snapshot, width)
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line width = %d, want <= %d: %q", got, width, line)
		}
	}
	for _, want := range []string{"GETSLOTH CONTROL", "RECLAIM", "KILL VIEWERS", "Phone joined"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard does not contain %q: %q", want, view)
		}
	}
}
