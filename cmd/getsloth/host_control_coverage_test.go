package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func startHostControlCoverageResponder(t *testing.T, payload string) string {
	t.Helper()

	stateDir, err := os.MkdirTemp("", "gs-")
	if err != nil {
		t.Fatalf("create host control response directory: %v", err)
	}
	socketPath := filepath.Join(stateDir, "control.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = os.RemoveAll(stateDir)
		t.Fatalf("listen for host control response: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var request hostControlRequest
		_ = json.NewDecoder(conn).Decode(&request)
		_, _ = io.WriteString(conn, payload)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		_ = os.RemoveAll(stateDir)
	})
	return listener.Addr().String()
}

func TestHostControlCoverage_ConsoleValidationAndErrors(t *testing.T) {
	t.Run("invalid arguments print usage", func(t *testing.T) {
		var out bytes.Buffer
		if code := runHostControlConsole([]string{"--socket", ""}, &out); code != 2 {
			t.Fatalf("runHostControlConsole exit code = %d, want 2", code)
		}
		if !strings.Contains(out.String(), "usage:") {
			t.Fatalf("usage output = %q", out.String())
		}
	})

	t.Run("connect failure", func(t *testing.T) {
		var out bytes.Buffer
		if code := runHostControlConsole([]string{"--socket", filepath.Join(t.TempDir(), "missing.sock")}, &out); code != 1 {
			t.Fatalf("runHostControlConsole exit code = %d, want 1", code)
		}
		if !strings.Contains(out.String(), "connect to host session") {
			t.Fatalf("connect failure output = %q", out.String())
		}
	})

	cases := []struct {
		name       string
		payload    string
		wantOutput string
	}{
		{name: "malformed response", payload: "not json\n", wantOutput: "read host status"},
		{name: "error response", payload: `{"error":"host action denied"}` + "\n", wantOutput: "host action denied"},
		{name: "missing snapshot", payload: `{"error":""}` + "\n", wantOutput: "host returned no session status"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			socketPath := startHostControlCoverageResponder(t, test.payload)
			var out bytes.Buffer
			if code := runHostControlConsole([]string{"--socket", socketPath}, &out); code != 1 {
				t.Fatalf("runHostControlConsole exit code = %d, want 1; output = %q", code, out.String())
			}
			if !strings.Contains(out.String(), test.wantOutput) {
				t.Fatalf("response output = %q, want %q", out.String(), test.wantOutput)
			}
		})
	}
}

func TestHostControlCoverage_WatchValidationErrors(t *testing.T) {
	cases := []struct {
		name       string
		payload    string
		wantOutput string
	}{
		{name: "connect failure", payload: "", wantOutput: "connect to host session"},
		{name: "malformed response", payload: "not json\n", wantOutput: "read host status"},
		{name: "error response", payload: `{"error":"watch denied"}` + "\n", wantOutput: "watch denied"},
		{name: "missing snapshot", payload: `{"error":""}` + "\n", wantOutput: "host returned no session status"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			socketPath := filepath.Join(t.TempDir(), "missing.sock")
			if test.payload != "" {
				socketPath = startHostControlCoverageResponder(t, test.payload)
			}
			var out bytes.Buffer
			if code := runHostControlConsole([]string{"--socket", socketPath, "--watch"}, &out); code != 1 {
				t.Fatalf("watch console exit code = %d, want 1; output = %q", code, out.String())
			}
			if !strings.Contains(out.String(), test.wantOutput) {
				t.Fatalf("watch output = %q, want %q", out.String(), test.wantOutput)
			}
		})
	}
}

func TestHostControlCoverage_LocalEndpointCleanupIsIdempotent(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{Live: true}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	stateDir := server.stateDir
	socketPath := server.socketPath
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat host control socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("host control socket permissions = %o, want 600", got)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close host control server: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("second close host control server: %v", err)
	}
	if _, err := net.Dial("unix", socketPath); err == nil {
		t.Fatal("host control socket remained connectable after Close")
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("host control state directory still exists: stat error = %v", err)
	}
}

func TestHostControlCoverage_ServerIgnoresMalformedLocalRequest(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{Live: true}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	conn, err := net.Dial("unix", server.socketPath)
	if err != nil {
		t.Fatalf("dial host control socket: %v", err)
	}
	if _, err := io.WriteString(conn, "not json\n"); err != nil {
		t.Fatalf("write malformed local request: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close malformed local request: %v", err)
	}
}

func TestHostControlCoverage_CloseReportsListenerFailure(t *testing.T) {
	server, err := startHostControlServer(func() hostControlSnapshot { return hostControlSnapshot{} })
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	stateDir := server.stateDir
	if err := server.listener.Close(); err != nil {
		t.Fatalf("close listener before server close: %v", err)
	}
	err = server.Close()
	if err == nil {
		t.Fatal("Close succeeded after listener was already closed")
	}
	if secondErr := server.Close(); secondErr == nil || secondErr.Error() != err.Error() {
		t.Fatalf("second Close error = %v, want stable first error %v", secondErr, err)
	}
	if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
		t.Fatalf("state directory survived listener close failure: %v", statErr)
	}
}

func TestHostControlCoverage_TUIInitRefreshAndActionLifecycle(t *testing.T) {
	actionCalled := make(chan struct{})
	server, err := startHostControlServer(func() hostControlSnapshot {
		return hostControlSnapshot{Live: true, Mode: "remote", InviteURL: "https://example.test/s/session"}
	})
	if err != nil {
		t.Fatalf("startHostControlServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	server.setActions(func() error {
		close(actionCalled)
		return nil
	}, nil)

	model := newHostControlTUI(server.socketPath, &bytes.Buffer{}, hostControlSnapshot{Live: false})
	initial := model.Init()()
	initialMsg, ok := initial.(hostControlSnapshotMsg)
	if !ok || initialMsg.err != nil || !initialMsg.snapshot.Live {
		t.Fatalf("Init command result = %#v, want live snapshot", initial)
	}

	updated, tick := model.Update(initialMsg)
	model = updated.(hostControlTUI)
	if !model.snapshot.Live || tick == nil {
		t.Fatalf("snapshot update = %+v, tick command = %v", model.snapshot, tick)
	}
	if refreshMessage := tick(); refreshMessage == nil {
		t.Fatal("snapshot retry command returned no refresh message")
	}

	updated, refresh := model.Update(hostControlRefreshMsg{})
	model = updated.(hostControlTUI)
	if refresh == nil {
		t.Fatal("refresh message produced no refresh command")
	}
	refreshed, ok := refresh().(hostControlSnapshotMsg)
	if !ok || refreshed.err != nil || !refreshed.snapshot.Live {
		t.Fatalf("refresh command result = %#v, want live snapshot", refreshed)
	}

	updated, action := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("r")}))
	model = updated.(hostControlTUI)
	if action == nil {
		t.Fatal("reclaim key produced no action command")
	}
	actionMessage, ok := action().(hostControlSnapshotMsg)
	if !ok || actionMessage.err != nil {
		t.Fatalf("action command result = %#v", actionMessage)
	}
	<-actionCalled

	previous := model.snapshot
	updated, tick = model.Update(hostControlSnapshotMsg{err: errors.New("host channel disconnected")})
	model = updated.(hostControlTUI)
	if !reflect.DeepEqual(model.snapshot, previous) || tick == nil {
		t.Fatalf("error snapshot update changed state or omitted retry: before=%+v after=%+v tick=%v", previous, model.snapshot, tick)
	}
}

func TestHostControlCoverage_TUIReportsRequestErrorsAndIgnoresNoopInput(t *testing.T) {
	model := newHostControlTUI(filepath.Join(t.TempDir(), "missing.sock"), &bytes.Buffer{}, hostControlSnapshot{Live: true})
	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes}))
	if cmd != nil || updated.(hostControlTUI).snapshot.Live != model.snapshot.Live {
		t.Fatalf("empty key update = %#v, %v; want no-op", updated, cmd)
	}
	updated, cmd = model.Update(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd != nil || updated.(hostControlTUI).snapshot.Live != model.snapshot.Live {
		t.Fatalf("non-keybar mouse update = %#v, %v; want no-op", updated, cmd)
	}

	_, cmd = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("r")}))
	if cmd == nil {
		t.Fatal("action against missing socket produced no command")
	}
	message, ok := cmd().(hostControlSnapshotMsg)
	if !ok || message.err == nil {
		t.Fatalf("failed action message = %#v, want an error", message)
	}

	cases := []struct {
		name    string
		payload string
	}{
		{name: "error response", payload: `{"error":"request rejected"}` + "\n"},
		{name: "missing snapshot", payload: `{"error":""}` + "\n"},
		{name: "malformed response", payload: "not json\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			socketPath := startHostControlCoverageResponder(t, test.payload)
			_, err := requestHostControlSnapshot(socketPath, "snapshot")
			if err == nil {
				t.Fatal("requestHostControlSnapshot succeeded for invalid response")
			}
			var controlErr *hostControlError
			if test.name != "malformed response" && !errors.As(err, &controlErr) {
				t.Fatalf("request error = %v, want hostControlError", err)
			}
		})
	}
}

func TestHostControlCoverage_TUIMouseGapAndDashboardWidthBoundaries(t *testing.T) {
	model := newHostControlTUI("/tmp/getsloth.sock", &bytes.Buffer{}, hostControlSnapshot{})
	firstRegion := hostControlKeybarRegions(false)[0]
	_, cmd := model.Update(tea.MouseMsg{X: firstRegion.EndCol + 1, Y: hostControlKeybarRow, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if cmd != nil {
		t.Fatal("clicking a keybar gap produced a command")
	}
	if got := maxDashboardWidth(1, 2); got != 2 {
		t.Fatalf("maxDashboardWidth below minimum = %d, want 2", got)
	}
	if got := maxDashboardWidth(3, 2); got != 3 {
		t.Fatalf("maxDashboardWidth above minimum = %d, want 3", got)
	}
	if got := truncateDashboardText("long label", 1); got != "…" {
		t.Fatalf("truncateDashboardText width 1 = %q, want ellipsis", got)
	}
	if got := truncateDashboardText("long label", 0); got != "long label" {
		t.Fatalf("truncateDashboardText width 0 = %q, want original", got)
	}
	if view := renderHostControlDashboard(hostControlSnapshot{}, 10, false); view == "" {
		t.Fatal("narrow dashboard rendered no content")
	}
	if got := hostControlKeyAction('x'); got != "" {
		t.Fatalf("unknown key action = %q, want empty action", got)
	}
	if _, cmd := model.handleControlKey('x'); cmd != nil {
		t.Fatal("unknown TUI control key produced a command")
	}
}
