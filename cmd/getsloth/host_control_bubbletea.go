package main

import (
	"encoding/json"
	"io"
	"net"
	"time"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
)

type hostControlSnapshotMsg struct {
	snapshot hostControlSnapshot
	err      error
}

type hostControlTUI struct {
	socketPath string
	out        io.Writer
	snapshot   hostControlSnapshot
	width      int
	height     int
	showInvite bool
}

func newHostControlTUI(socketPath string, out io.Writer, snapshot hostControlSnapshot) hostControlTUI {
	return hostControlTUI{socketPath: socketPath, out: out, snapshot: snapshot, width: 80}
}

func (m hostControlTUI) Init() tea.Cmd {
	return hostControlRefreshCmd(m.socketPath)
}

func (m hostControlTUI) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case hostControlSnapshotMsg:
		if msg.err == nil {
			m.snapshot = msg.snapshot
		}
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return hostControlRefreshMsg{} })
	case hostControlRefreshMsg:
		return m, hostControlRefreshCmd(m.socketPath)
	case tea.KeyMsg:
		key := msg.String()
		if len(key) == 0 {
			return m, nil
		}
		return m.handleControlKey(key[0])
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft || msg.Y != hostControlKeybarRow {
			return m, nil
		}
		for _, region := range hostControlKeybarRegions(m.showInvite) {
			if msg.X >= region.StartCol && msg.X < region.EndCol {
				return m.handleControlKey(region.Key)
			}
		}
	}
	return m, nil
}

// hostControlKeybarRow is the line the keybar renders on within the
// dashboard view: header (0), keybar (1), then a blank line and the body.
const hostControlKeybarRow = 1

func (m hostControlTUI) handleControlKey(key byte) (tea.Model, tea.Cmd) {
	switch hostControlKeyAction(key) {
	case "quit":
		return m, tea.Quit
	case "reclaim", "kill":
		return m, hostControlActionCmd(m.socketPath, hostControlKeyAction(key))
	case "snapshot":
		m.showInvite = !m.showInvite
		if m.showInvite {
			return m, hostControlCopyInviteCmd(m.out, m.snapshot)
		}
		return m, nil
	case "copy":
		return m, hostControlCopyInviteCmd(m.out, m.snapshot)
	}
	return m, nil
}

type hostControlRefreshMsg struct{}

func (m hostControlTUI) View() string {
	return renderHostControlDashboard(m.snapshot, m.width, m.showInvite)
}

// hostControlCopyInviteCmd copies the invite URL and password to the local
// clipboard via an OSC52 terminal escape sequence. This works over SSH
// without any clipboard tooling on the host; failures are ignored because
// copying is a convenience, not a required part of revealing the invite.
func hostControlCopyInviteCmd(out io.Writer, snapshot hostControlSnapshot) tea.Cmd {
	return func() tea.Msg {
		if out != nil {
			_, _ = osc52.New(snapshot.InviteURL + "\n" + snapshot.Password).WriteTo(out)
		}
		return nil
	}
}

func hostControlRefreshCmd(socketPath string) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := requestHostControlSnapshot(socketPath, "snapshot")
		return hostControlSnapshotMsg{snapshot: snapshot, err: err}
	}
}

func hostControlActionCmd(socketPath, action string) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := requestHostControlSnapshot(socketPath, action)
		return hostControlSnapshotMsg{snapshot: snapshot, err: err}
	}
}

func requestHostControlSnapshot(socketPath, action string) (hostControlSnapshot, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return hostControlSnapshot{}, err
	}
	defer func() { _ = conn.Close() }()
	if err := json.NewEncoder(conn).Encode(hostControlRequest{Action: action}); err != nil {
		return hostControlSnapshot{}, err
	}
	var response hostControlResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return hostControlSnapshot{}, err
	}
	if response.Error != "" {
		return hostControlSnapshot{}, &hostControlError{message: response.Error}
	}
	if response.Snapshot == nil {
		return hostControlSnapshot{}, &hostControlError{message: "host returned no session status"}
	}
	return *response.Snapshot, nil
}

type hostControlError struct {
	message string
}

func (e *hostControlError) Error() string {
	return e.message
}
