//go:build !windows

package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type authCoverageOutput struct {
	writes chan string
}

func (w *authCoverageOutput) Write(p []byte) (int, error) {
	w.writes <- string(p)
	return len(p), nil
}

func waitForAuthCoverageRender(t *testing.T, renders <-chan string) string {
	t.Helper()
	select {
	case render := <-renders:
		return render
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for host status render")
		return ""
	}
}

func newAuthCoverageWebSockets(t *testing.T) (*safeConn, *websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverConn := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- ws
	}))

	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		ts.Close()
		t.Fatalf("dial coverage websocket: %v", err)
	}
	server := <-serverConn
	cleanup := func() {
		_ = client.Close()
		_ = server.Close()
		ts.Close()
	}
	return &safeConn{ws: server}, client, cleanup
}

func sendAuthCoverageRaw(t *testing.T, client *websocket.Conn, raw string) {
	t.Helper()
	if err := client.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
		t.Fatalf("write raw host-loop message: %v", err)
	}
}

func TestRunHostMessageLoop_ValidatesDispatchesAndMarksDisconnect(t *testing.T) {
	ptmx, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = slave.Close() }()
	oldState, err := term.MakeRaw(int(slave.Fd()))
	if err != nil {
		t.Fatalf("term.MakeRaw: %v", err)
	}
	defer func() { _ = term.Restore(int(slave.Fd()), oldState) }()

	ws, client, cleanup := newAuthCoverageWebSockets(t)
	defer cleanup()
	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("hostauth.NewKeyPair: %v", err)
	}

	active := &atomic.Bool{}
	active.Store(true)
	ptmxCh := make(chan ptyConn, 1)
	ptmxCh <- newUnixPTY(ptmx)
	output := &authCoverageOutput{writes: make(chan string, 32)}
	status := newHostSessionStatus(protocol.SessionModeRemote, output)
	_ = waitForAuthCoverageRender(t, output.writes)

	loopDone := make(chan struct{})
	go func() {
		runHostMessageLoop(ws, "coverage-session", "correct-password", keys, active, ptmxCh, status)
		close(loopDone)
	}()

	// These messages have a valid envelope but malformed typed payloads. Each
	// must be ignored without terminating the host's message loop.
	for _, raw := range []string{
		"not-json",
		`{"v":1,"type":"unknown"}`,
		`{"v":1,"type":"auth_request","request_id":7}`,
		`{"v":1,"type":"control_changed","active_writer_id":7}`,
		`{"v":1,"type":"presence","connections":"not-a-list"}`,
		`{"v":1,"type":"input","data_base64":7}`,
		`{"v":1,"type":"resize","cols":"wide"}`,
		`{"v":1,"type":"chat_message","text":7}`,
	} {
		sendAuthCoverageRaw(t, client, raw)
	}

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), "coverage-session", "correct-password")
	if err := client.WriteJSON(protocol.AuthRequestMsg{
		Envelope:           protocol.NewEnvelope("auth_request"),
		RequestID:          "request-1",
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
	}); err != nil {
		t.Fatalf("write valid auth request: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authResponse protocol.AuthResponseMsg
	if err := client.ReadJSON(&authResponse); err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	if authResponse.RequestID != "request-1" || !authResponse.OK {
		t.Fatalf("auth response = %#v, want successful request-1 response", authResponse)
	}

	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: 88, Rows: 28}); err != nil {
		t.Fatalf("set baseline PTY size: %v", err)
	}
	if err := client.WriteJSON(protocol.ControlChangedMsg{
		Envelope:         protocol.NewEnvelope("control_changed"),
		ActiveWriterID:   "viewer-1",
		ActiveWriterRole: "viewer",
		Cols:             1,
		Rows:             1,
	}); err != nil {
		t.Fatalf("write invalid-size control message: %v", err)
	}
	waitForAuthCoverageRender(t, output.writes)
	if active.Load() {
		t.Fatal("invalid-size control message left host active")
	}
	if got := readCoverageGrid(t, ptmx); got != (coverageGrid{cols: 88, rows: 28}) {
		t.Fatalf("invalid-size control changed PTY to %#v, want 88x28", got)
	}

	if err := client.WriteJSON(protocol.ControlChangedMsg{
		Envelope:         protocol.NewEnvelope("control_changed"),
		ActiveWriterID:   "viewer-1",
		ActiveWriterRole: "viewer",
		Cols:             100,
		Rows:             30,
	}); err != nil {
		t.Fatalf("write valid control message: %v", err)
	}
	waitForAuthCoverageRender(t, output.writes)
	if active.Load() {
		t.Fatal("viewer control message left host active")
	}
	if got := readCoverageGrid(t, ptmx); got != (coverageGrid{cols: 100, rows: 30}) {
		t.Fatalf("valid control PTY size = %#v, want 100x30", got)
	}

	latency := int64(17)
	if err := client.WriteJSON(protocol.PresenceMsg{
		Envelope: protocol.NewEnvelope("presence"),
		Connections: []protocol.PresenceConnectionInfo{
			{ID: "host", Role: "host", IsActiveWriter: false},
			{ID: "viewer-1", Role: "viewer", DisplayName: "Alice", IsActiveWriter: true, RTTMs: &latency, Quality: protocol.ViewerQualityGood},
		},
	}); err != nil {
		t.Fatalf("write presence message: %v", err)
	}
	waitForAuthCoverageRender(t, output.writes)
	snapshot := status.snapshot()
	if len(snapshot.Viewers) != 1 || snapshot.Viewers[0].Name != "Alice" || snapshot.Viewers[0].RTTMs == nil || *snapshot.Viewers[0].RTTMs != latency {
		t.Fatalf("status viewers = %#v, want Alice with %dms RTT", snapshot.Viewers, latency)
	}

	sendAuthCoverageRaw(t, client, `{"v":1,"type":"input","data_base64":"%%%"}`)
	expectedInput := []byte("viewer input")
	inputRead := make(chan []byte, 1)
	go func() {
		got := make([]byte, len(expectedInput))
		_, readErr := io.ReadFull(slave, got)
		if readErr != nil {
			inputRead <- nil
			return
		}
		inputRead <- got
	}()
	if err := client.WriteJSON(protocol.InputForwardMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: base64.StdEncoding.EncodeToString(expectedInput),
	}); err != nil {
		t.Fatalf("write valid input message: %v", err)
	}
	select {
	case got := <-inputRead:
		if string(got) != string(expectedInput) {
			t.Fatalf("PTY input = %q, want %q", got, expectedInput)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for valid input to reach PTY")
	}

	if err := client.WriteJSON(protocol.ResizeMsg{Envelope: protocol.NewEnvelope("resize"), Cols: 1, Rows: 1}); err != nil {
		t.Fatalf("write invalid resize: %v", err)
	}
	if err := client.WriteJSON(protocol.ResizeMsg{Envelope: protocol.NewEnvelope("resize"), Cols: 91, Rows: 21}); err != nil {
		t.Fatalf("write valid resize: %v", err)
	}
	if err := client.WriteJSON(protocol.ChatBroadcastMsg{
		Envelope:          protocol.NewEnvelope("chat_message"),
		SenderRole:        "viewer",
		SenderDisplayName: "",
		Text:              "check the logs",
	}); err != nil {
		t.Fatalf("write viewer chat message: %v", err)
	}
	chatRender := waitForAuthCoverageRender(t, output.writes)
	if !strings.Contains(chatRender, "Alice controls") {
		t.Fatalf("chat render = %q, want the current controller summary", chatRender)
	}
	if events := status.snapshot().Events; len(events) == 0 || !strings.Contains(events[0], "check the logs") {
		t.Fatalf("status events after viewer chat = %#v, want chat text", events)
	}
	if got := readCoverageGrid(t, ptmx); got != (coverageGrid{cols: 91, rows: 21}) {
		t.Fatalf("valid resize PTY size = %#v, want 91x21", got)
	}

	if err := client.WriteJSON(protocol.ChatBroadcastMsg{
		Envelope:   protocol.NewEnvelope("chat_message"),
		SenderRole: "host",
		Text:       "host note",
	}); err != nil {
		t.Fatalf("write host chat message: %v", err)
	}
	hostChatRender := waitForAuthCoverageRender(t, output.writes)
	if !strings.Contains(hostChatRender, "Alice controls") {
		t.Fatalf("host chat render = %q, want the current controller summary", hostChatRender)
	}
	if events := status.snapshot().Events; len(events) == 0 || !strings.Contains(events[0], "host note") {
		t.Fatalf("status events after host chat = %#v, want host note", events)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("close coverage websocket: %v", err)
	}
	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("host message loop did not stop after websocket disconnect")
	}
	waitForAuthCoverageRender(t, output.writes)
	if status.snapshot().Live {
		t.Fatal("status remains live after host message loop disconnect")
	}
}
