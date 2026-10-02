//go:build !windows

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/arinprajapati/getsloth/internal/relay"
	"github.com/gorilla/websocket"
)

func TestRunCLI_HelpVersionAndControlDispatch(t *testing.T) {
	var output bytes.Buffer
	if code := runCLI([]string{"getsloth", "--help"}, nil, &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	if !strings.Contains(output.String(), "getsloth --group") {
		t.Fatalf("help output = %q, want usage text", output.String())
	}

	output.Reset()
	if code := runCLI([]string{"getsloth", "--version"}, nil, &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("version exit code = %d, want 0", code)
	}
	if got := strings.TrimSpace(output.String()); got != "getsloth "+version {
		t.Fatalf("version output = %q, want %q", got, "getsloth "+version)
	}

	output.Reset()
	if code := runCLI([]string{"getsloth", "control"}, nil, &output, &bytes.Buffer{}); code != 2 {
		t.Fatalf("invalid control exit code = %d, want 2", code)
	}
	if !strings.Contains(output.String(), "getsloth control: usage") {
		t.Fatalf("invalid control output = %q, want control usage", output.String())
	}
}

func TestRunCLI_LocalFallbackPropagatesWrappedExitCode(t *testing.T) {
	t.Setenv("GETSLOTH_NO_TUI", "1")
	t.Setenv("GETSLOTH_RELAY_URL", "ws://127.0.0.1:1")
	t.Setenv("GETSLOTH_PASSWORD", "local-test-password")

	var stdout, stderr bytes.Buffer
	code := runCLIWithLauncher(
		[]string{"getsloth", "--remote", "sh", "-c", "printf 'local output\\n'; exit 7"},
		nonTerminalStdin(t),
		&stdout,
		&stderr,
		func(string) error {
			t.Fatal("host control launcher was called without a relay")
			return nil
		},
	)

	if code != 7 {
		t.Fatalf("wrapped exit code = %d, want 7", code)
	}
	if !strings.Contains(stdout.String(), "local output") {
		t.Fatalf("local stdout = %q, want wrapped command output", stdout.String())
	}
	for _, phrase := range []string{"could not reach relay", "continuing locally only"} {
		if !strings.Contains(stderr.String(), phrase) {
			t.Errorf("local stderr = %q, want %q", stderr.String(), phrase)
		}
	}
}

func TestRunCLI_RealRelayStreamsOutputAndCleansSession(t *testing.T) {
	base, closeRelay := newCLITestRelay(t)
	defer closeRelay()
	t.Setenv("GETSLOTH_NO_TUI", "1")
	t.Setenv("GETSLOTH_RELAY_URL", base)
	t.Setenv("GETSLOTH_WEB_URL", "https://viewer.example")
	t.Setenv("GETSLOTH_PASSWORD", "shared-test-password")

	var stdout, stderr stringBuffer
	done := make(chan int, 1)
	go func() {
		done <- runCLIWithLauncher(
			[]string{"getsloth", "--remote", "sh", "-c", "printf 'shared-ready\\n'; sleep 2"},
			nonTerminalStdin(t),
			&stdout,
			&stderr,
			func(string) error { return nil },
		)
	}()

	waitForCLIText(t, &stderr, "getsloth: live at ")
	waitForCLIText(t, &stderr, "getsloth: password: ")
	invite, password := cliInviteAndPassword(t, stderr.String())
	sessionID, hostKey := cliInviteParts(t, invite)
	viewer := cliAuthenticateViewer(t, base, sessionID, hostKey, password)
	defer func() { _ = viewer.Close() }()

	var output protocol.OutputMsg
	readCLIMessage(t, viewer, "output", &output)
	if data, err := decodeOutput(output); err != nil || !strings.Contains(string(data), "shared-ready") {
		t.Fatalf("replayed PTY output = %q, decode error = %v", data, err)
	}

	var ended protocol.SessionEndedMsg
	readCLIMessage(t, viewer, "session_ended", &ended)
	if ended.Reason != protocol.ReasonProcessExited {
		t.Fatalf("session ended reason = %q, want %q", ended.Reason, protocol.ReasonProcessExited)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("wrapped command exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CLI runner did not return after wrapped command exited")
	}
	if !strings.Contains(stdout.String(), "shared-ready") {
		t.Fatalf("host stdout = %q, want PTY output", stdout.String())
	}
	waitForCLISessionGone(t, base, sessionID)
}

func TestRunCLI_ControlSocketActionsAndSignals(t *testing.T) {
	base, closeRelay := newCLITestRelay(t)
	defer closeRelay()
	t.Setenv("GETSLOTH_NO_TUI", "1")
	t.Setenv("GETSLOTH_RELAY_URL", base)
	t.Setenv("GETSLOTH_WEB_URL", "https://viewer.example")
	t.Setenv("GETSLOTH_PASSWORD", "control-test-password")

	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = inputR.Close() }()
	defer func() { _ = inputW.Close() }()

	var stdout, stderr stringBuffer
	launcherPath := make(chan string, 1)
	done := make(chan int, 1)
	go func() {
		done <- runCLIWithLauncher(
			[]string{"getsloth", "--remote", "sh", "-c", "printf 'control-ready\\n'; sleep 30"},
			inputR,
			&stdout,
			&stderr,
			func(path string) error {
				launcherPath <- path
				return nil
			},
		)
	}()

	waitForCLIText(t, &stderr, "getsloth: live at ")
	waitForCLIText(t, &stderr, "getsloth: password: ")
	invite, password := cliInviteAndPassword(t, stderr.String())
	sessionID, hostKey := cliInviteParts(t, invite)
	var socketPath string
	select {
	case socketPath = <-launcherPath:
	case <-time.After(2 * time.Second):
		t.Fatal("host control launcher was not called")
	}
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("control socket stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("control socket permissions = %o, want 600", info.Mode().Perm())
	}

	viewer := cliAuthenticateViewer(t, base, sessionID, hostKey, password)
	defer func() { _ = viewer.Close() }()
	var initialOutput protocol.OutputMsg
	var initialPresence protocol.PresenceMsg
	readCLIOutputAndPresence(t, viewer, &initialOutput, &initialPresence)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{
		Envelope: protocol.NewEnvelope("take_control"),
		Cols:     protocol.DefaultCols,
		Rows:     protocol.DefaultRows,
	}); err != nil {
		t.Fatalf("viewer take_control: %v", err)
	}
	var viewerControl protocol.ControlChangedMsg
	readCLIMessage(t, viewer, "control_changed", &viewerControl)
	if viewerControl.ActiveWriterRole != "viewer" {
		t.Fatalf("viewer control role = %q, want viewer", viewerControl.ActiveWriterRole)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatalf("send SIGUSR2: %v", err)
	}
	var signalReclaim protocol.ControlChangedMsg
	readCLIMessage(t, viewer, "control_changed", &signalReclaim)
	if signalReclaim.ActiveWriterRole != "host" {
		t.Fatalf("SIGUSR2 control role = %q, want host", signalReclaim.ActiveWriterRole)
	}

	var console bytes.Buffer
	if code := runHostControlConsole([]string{"--socket", socketPath, "--action", "reclaim"}, &console); code != 0 {
		t.Fatalf("control reclaim exit code = %d, output = %q", code, console.String())
	}
	var socketReclaim protocol.ControlChangedMsg
	readCLIMessage(t, viewer, "control_changed", &socketReclaim)
	if socketReclaim.ActiveWriterRole != "host" {
		t.Fatalf("socket reclaim control role = %q, want host", socketReclaim.ActiveWriterRole)
	}
	if !strings.Contains(console.String(), "controller") {
		t.Fatalf("control reclaim output = %q, want dashboard snapshot", console.String())
	}

	console.Reset()
	if code := runHostControlConsole([]string{"--socket", socketPath, "--action", "kill"}, &console); code != 0 {
		t.Fatalf("control kill exit code = %d, output = %q", code, console.String())
	}
	var kicked protocol.KickedMsg
	readCLIMessage(t, viewer, "kicked", &kicked)
	if kicked.Reason != protocol.ReasonKillSwitch {
		t.Fatalf("control kill reason = %q, want %q", kicked.Reason, protocol.ReasonKillSwitch)
	}
	if !strings.Contains(console.String(), "LIVE") {
		t.Fatalf("control kill output = %q, want live session snapshot", console.String())
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("control socket disappeared after kill switch: %v", err)
	}

	viewer2 := cliAuthenticateViewer(t, base, sessionID, hostKey, password)
	defer func() { _ = viewer2.Close() }()
	var secondOutput protocol.OutputMsg
	var secondPresence protocol.PresenceMsg
	readCLIOutputAndPresence(t, viewer2, &secondOutput, &secondPresence)
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("send SIGUSR1: %v", err)
	}
	var signalKicked protocol.KickedMsg
	readCLIMessage(t, viewer2, "kicked", &signalKicked)
	if signalKicked.Reason != protocol.ReasonKillSwitch {
		t.Fatalf("SIGUSR1 reason = %q, want %q", signalKicked.Reason, protocol.ReasonKillSwitch)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case code := <-done:
		if code != -1 {
			t.Fatalf("panic-killed command exit code = %d, want -1", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("panic signal did not stop the wrapped PTY command")
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("control socket still exists after runner return, stat error = %v", err)
	}
	waitForCLISessionGone(t, base, sessionID)
}

func TestRunCLI_PTYStartupFailureStillCleansRelayAndControlSocket(t *testing.T) {
	base, closeRelay := newCLITestRelay(t)
	defer closeRelay()
	t.Setenv("GETSLOTH_NO_TUI", "1")
	t.Setenv("GETSLOTH_RELAY_URL", base)
	t.Setenv("GETSLOTH_WEB_URL", "https://viewer.example")
	t.Setenv("GETSLOTH_PASSWORD", "pty-failure-password")

	var stdout, stderr stringBuffer
	launcherPath := make(chan string, 1)
	code := runCLIWithLauncher(
		[]string{"getsloth", "--remote", "getsloth-test-command-that-does-not-exist"},
		nonTerminalStdin(t),
		&stdout,
		&stderr,
		func(path string) error {
			launcherPath <- path
			return errors.New("test terminal unavailable")
		},
	)
	if code != 1 {
		t.Fatalf("PTY startup failure exit code = %d, want 1", code)
	}
	waitForCLIText(t, &stderr, "getsloth: live at ")
	invite, _ := cliInviteAndPassword(t, stderr.String())
	sessionID, _ := cliInviteParts(t, invite)
	var socketPath string
	select {
	case socketPath = <-launcherPath:
	case <-time.After(2 * time.Second):
		t.Fatal("host control launcher was not called")
	}
	if !strings.Contains(stderr.String(), "host control console unavailable") {
		t.Fatalf("PTY failure stderr = %q, want launcher failure", stderr.String())
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("control socket still exists after PTY failure, stat error = %v", err)
	}
	waitForCLISessionGone(t, base, sessionID)
}

func newCLITestRelay(t *testing.T) (string, func()) {
	t.Helper()
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	return "ws" + strings.TrimPrefix(ts.URL, "http"), ts.Close
}

func waitForCLIText(t *testing.T, output *stringBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in output %q", want, output.String())
}

func cliInviteAndPassword(t *testing.T, output string) (string, string) {
	t.Helper()
	var invite, password string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "getsloth: live at ") {
			invite = strings.TrimSpace(strings.TrimPrefix(line, "getsloth: live at "))
		}
		if strings.HasPrefix(line, "getsloth: password: ") {
			password = strings.TrimSpace(strings.TrimPrefix(line, "getsloth: password: "))
		}
	}
	if invite == "" || password == "" {
		t.Fatalf("CLI invite/password missing from output %q", output)
	}
	return invite, password
}

func cliInviteParts(t *testing.T, invite string) (sessionID, hostKey string) {
	t.Helper()
	parsed, err := url.Parse(invite)
	if err != nil {
		t.Fatalf("parse invite URL: %v", err)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parse invite fragment: %v", err)
	}
	sessionID = strings.TrimPrefix(parsed.Path, "/s/")
	hostKey = fragment.Get("k")
	if sessionID == "" || hostKey == "" {
		t.Fatalf("invite URL %q missing session ID or host key", invite)
	}
	return sessionID, hostKey
}

func cliAuthenticateViewer(t *testing.T, base, sessionID, hostKey, password string) *websocket.Conn {
	t.Helper()
	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+sessionID, nil)
	if err != nil {
		t.Fatalf("dial CLI viewer: %v", err)
	}
	viewerPub, ciphertext := encryptAsViewer(t, hostKey, sessionID, password)
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
		DisplayName:        "integration viewer",
	}); err != nil {
		_ = viewer.Close()
		t.Fatalf("authenticate CLI viewer: %v", err)
	}
	_ = viewer.SetReadDeadline(time.Now().Add(3 * time.Second))
	var result protocol.AuthResultMsg
	if err := viewer.ReadJSON(&result); err != nil {
		_ = viewer.Close()
		t.Fatalf("read CLI viewer auth result: %v", err)
	}
	if !result.OK {
		_ = viewer.Close()
		t.Fatalf("CLI viewer auth failed with code %q", result.Code)
	}
	return viewer
}

func readCLIMessage(t *testing.T, viewer *websocket.Conn, messageType string, target any) {
	t.Helper()
	for {
		_ = viewer.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, raw, err := viewer.ReadMessage()
		if err != nil {
			t.Fatalf("read CLI message %q: %v", messageType, err)
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("decode CLI message envelope: %v", err)
		}
		if envelope.Type != messageType {
			continue
		}
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatalf("decode CLI %s: %v", messageType, err)
		}
		return
	}
}

func readCLIOutputAndPresence(t *testing.T, viewer *websocket.Conn, output *protocol.OutputMsg, presence *protocol.PresenceMsg) {
	t.Helper()
	gotOutput, gotPresence := false, false
	for !gotOutput || !gotPresence {
		_ = viewer.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, raw, err := viewer.ReadMessage()
		if err != nil {
			t.Fatalf("read CLI output/presence: %v", err)
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("decode CLI output/presence envelope: %v", err)
		}
		switch envelope.Type {
		case "output":
			if err := json.Unmarshal(raw, output); err != nil {
				t.Fatalf("decode CLI output: %v", err)
			}
			gotOutput = true
		case "presence":
			if err := json.Unmarshal(raw, presence); err != nil {
				t.Fatalf("decode CLI presence: %v", err)
			}
			gotPresence = true
		}
	}
}

func decodeOutput(message protocol.OutputMsg) ([]byte, error) {
	return base64.StdEncoding.DecodeString(message.DataBase64)
}

func waitForCLISessionGone(t *testing.T, base, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+sessionID, nil)
		if err == nil {
			_ = viewer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			var response protocol.ErrorMsg
			readErr := viewer.ReadJSON(&response)
			_ = viewer.Close()
			if readErr == nil && response.Code == protocol.ErrSessionNotFound {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("relay session %s was not removed", sessionID)
}
