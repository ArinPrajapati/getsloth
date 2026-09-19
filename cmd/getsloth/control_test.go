package main

import (
	"encoding/base64"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/arinprajapati/getsloth/internal/relay"
	"github.com/gorilla/websocket"
)

// stringBuffer is a concurrency-safe append-only buffer - run()'s copy
// goroutine writes to stdout while the test polls it, which a plain
// bytes.Buffer doesn't support safely.
type stringBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *stringBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *stringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestControlHandoff_ViewerInputReachesRealPTY is the end-to-end proof
// this whole task exists for: a viewer takes control and types, and
// those bytes actually land in the wrapped process's PTY through the
// real host<->relay flow - not simulated at any layer.
func TestControlHandoff_ViewerInputReachesRealPTY(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base, testSessionConfig())
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}
	const password = "control-test-password"

	active := &atomic.Bool{}
	active.Store(true)
	ptmxCh := make(chan *os.File, 1)
	go runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh, nil)

	// Run `cat` in a real PTY via run() itself - exercising the actual
	// production PTY-spawn path, with onPTYReady feeding this same
	// channel the message loop is reading from, exactly as main.go
	// wires it.
	var stdout stringBuffer
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = stdinR.Close() }()

	done := make(chan int, 1)
	go func() {
		done <- run([]string{"cat"}, stdinR, &stdout, active, func(f *os.File) { ptmxCh <- f }, nil, nil, nil)
	}()

	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+created.SessionID, nil)
	if err != nil {
		t.Fatalf("dialing viewer: %v", err)
	}
	defer func() { _ = viewer.Close() }()

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), created.SessionID, password)
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
	}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}
	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authResult protocol.AuthResultMsg
	if err := viewer.ReadJSON(&authResult); err != nil {
		t.Fatalf("ReadJSON auth_result: %v", err)
	}
	if !authResult.OK {
		t.Fatalf("auth failed, can't test control handoff")
	}
	// A successful auth also triggers a presence broadcast (see
	// internal/relay/presence.go) - drain it before the control_changed
	// this test actually checks.
	var presence protocol.PresenceMsg
	if err := viewer.ReadJSON(&presence); err != nil {
		t.Fatalf("ReadJSON presence: %v", err)
	}

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("sending take_control: %v", err)
	}
	var controlChanged protocol.ControlChangedMsg
	if err := viewer.ReadJSON(&controlChanged); err != nil {
		t.Fatalf("ReadJSON control_changed: %v", err)
	}
	if controlChanged.ActiveWriterRole != "viewer" {
		t.Fatalf("active_writer_role = %q, want viewer", controlChanged.ActiveWriterRole)
	}
	// The host's own message loop needs a moment to process its copy of
	// control_changed and flip isActiveWriter to false.
	time.Sleep(100 * time.Millisecond)

	if err := viewer.WriteJSON(protocol.InputMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("hello from the viewer\n")),
	}); err != nil {
		t.Fatalf("sending input: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), "hello from the viewer") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(stdout.String(), "hello from the viewer") {
		t.Fatalf("cat's output (echoed via PTY) = %q, want it to contain the viewer's input", stdout.String())
	}

	// Closing the host's local stdin pipe does NOT end cat - cat is
	// reading from the PTY slave, which stays open regardless of what
	// happens to the host's own local input source. End it the same way
	// a real terminal would (Ctrl-D, EOF in canonical mode), sent
	// through the same viewer-input path just proven above, then close
	// the local pipe purely to let run()'s own stdin-copy goroutine exit
	// cleanly rather than leak.
	if err := viewer.WriteJSON(protocol.InputMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte{0x04}),
	}); err != nil {
		t.Fatalf("sending EOF: %v", err)
	}
	_ = stdinW.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("run() did not return after sending EOF to end cat")
	}
}
