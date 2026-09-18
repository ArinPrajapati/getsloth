package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHostControlTUI_UsesWindowWidthForResponsiveDashboard(t *testing.T) {
	model := newHostControlTUI("/tmp/getsloth.sock", &bytes.Buffer{}, hostControlSnapshot{Live: true})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	view := updated.(hostControlTUI).View()
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 72 {
			t.Errorf("line width = %d, want <= 72: %q", got, line)
		}
	}
}

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
	view := renderHostControlDashboard(snapshot, width, false)
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line width = %d, want <= %d: %q", got, width, line)
		}
	}
	for _, want := range []string{"GETSLOTH // HOST CONTROL", "SESSION", "LIVE VIEWERS", "ACTIVITY", "RECLAIM", "KILL", "Invite link ready", "Phone joined"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard does not contain %q: %q", want, view)
		}
	}
	if strings.Contains(view, snapshot.Password) {
		t.Errorf("dashboard should not reveal the password until requested: %q", view)
	}
}

func TestRenderHostControlDashboard_RevealsInviteWhenRequested(t *testing.T) {
	snapshot := hostControlSnapshot{
		Live:           true,
		Mode:           protocol.SessionModeRemote,
		InviteURL:      "https://127.0.0.1:5173/s/example",
		Password:       "sloth-demo",
		ControllerRole: "host",
	}

	const width = 72
	view := renderHostControlDashboard(snapshot, width, true)
	for _, want := range []string{snapshot.InviteURL, snapshot.Password} {
		if !strings.Contains(view, want) {
			t.Errorf("revealed dashboard does not contain %q: %q", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line width = %d, want <= %d: %q", got, width, line)
		}
	}
}

func TestHostControlKeybarRegions_AreOrderedAndNonOverlapping(t *testing.T) {
	regions := hostControlKeybarRegions(false)
	wantKeys := []byte{'r', 'k', 'i', 'c', 'q'}
	if len(regions) != len(wantKeys) {
		t.Fatalf("got %d keybar regions, want %d", len(regions), len(wantKeys))
	}
	for idx, region := range regions {
		if region.Key != wantKeys[idx] {
			t.Errorf("region[%d].Key = %q, want %q", idx, region.Key, wantKeys[idx])
		}
		if region.EndCol <= region.StartCol {
			t.Errorf("region[%d] has empty span: %+v", idx, region)
		}
		if idx > 0 && region.StartCol < regions[idx-1].EndCol {
			t.Errorf("region[%d] overlaps region[%d]: %+v, %+v", idx, idx-1, regions[idx-1], region)
		}
	}
}

func TestHostControlTUI_MouseClickTriggersKeybarAction(t *testing.T) {
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

	model := newHostControlTUI(server.socketPath, &bytes.Buffer{}, hostControlSnapshot{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(hostControlTUI)

	regions := hostControlKeybarRegions(model.showInvite)
	var reclaim hostControlButtonRegion
	for _, region := range regions {
		if region.Key == 'r' {
			reclaim = region
		}
	}
	if reclaim.EndCol == 0 {
		t.Fatal("did not find reclaim button region")
	}

	click := tea.MouseMsg{X: reclaim.StartCol + 1, Y: 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	_, cmd := model.Update(click)
	if cmd == nil {
		t.Fatal("clicking the reclaim button produced no command")
	}
	cmd()

	if !called {
		t.Fatal("clicking the reclaim button did not send the reclaim action")
	}
}

func TestHostControlTUI_CopyKeyCopiesInviteWithoutTogglingReveal(t *testing.T) {
	var out bytes.Buffer
	model := newHostControlTUI("/tmp/getsloth.sock", &out, hostControlSnapshot{
		InviteURL: "https://127.0.0.1:5173/s/example",
		Password:  "sloth-demo",
	})

	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("c")}))
	model = updated.(hostControlTUI)
	if model.showInvite {
		t.Fatal("pressing 'c' should not reveal the credentials in the dashboard")
	}
	if cmd == nil {
		t.Fatal("pressing 'c' produced no command")
	}
	cmd()

	if !bytes.Contains(out.Bytes(), []byte("\x1b]52;")) {
		t.Errorf("pressing 'c' should copy the invite via an OSC52 sequence, got %q", out.Bytes())
	}
}

func TestHostControlTUI_ToggleInviteKeyRevealsAndHidesCredentials(t *testing.T) {
	var out bytes.Buffer
	model := newHostControlTUI("/tmp/getsloth.sock", &out, hostControlSnapshot{
		Live:      true,
		InviteURL: "https://127.0.0.1:5173/s/example",
		Password:  "sloth-demo",
	})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	model = updated.(hostControlTUI)

	toggle := tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("i")})

	updated, _ = model.Update(toggle)
	model = updated.(hostControlTUI)
	if view := model.View(); !strings.Contains(view, "sloth-demo") {
		t.Errorf("first 'i' press should reveal the password, view = %q", view)
	}

	updated, _ = model.Update(toggle)
	model = updated.(hostControlTUI)
	if view := model.View(); strings.Contains(view, "sloth-demo") {
		t.Errorf("second 'i' press should hide the password again, view = %q", view)
	}
}
