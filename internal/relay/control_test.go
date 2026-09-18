package relay

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// hostStub is the single reader/writer for a test's host connection.
// gorilla/websocket doesn't support concurrent reads (or unsynchronized
// concurrent writes) on one connection, so a background auto-responder
// goroutine and foreground test assertions can't both call
// host.ReadMessage()/WriteJSON() directly without racing - this is the
// bug that made the first version of these tests hang. hostStub owns
// the connection's only read loop, auto-answers auth_request, and
// forwards every other message onto a channel the test drains with
// next(); all writes (including the background auth_response) go
// through its mutex-guarded WriteJSON.
type hostStub struct {
	ws    *websocket.Conn
	other chan []byte
	mu    sync.Mutex
}

func newHostStub(t *testing.T, ws *websocket.Conn, authOK bool) *hostStub {
	t.Helper()
	hs := &hostStub{ws: ws, other: make(chan []byte, 16)}
	go func() {
		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				close(hs.other)
				return
			}
			var env protocol.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				continue
			}
			if env.Type == "auth_request" {
				var req protocol.AuthRequestMsg
				if err := json.Unmarshal(raw, &req); err != nil {
					continue
				}
				_ = hs.WriteJSON(protocol.AuthResponseMsg{
					Envelope:  protocol.NewEnvelope("auth_response"),
					RequestID: req.RequestID,
					OK:        authOK,
				})
				continue
			}
			hs.other <- raw
		}
	}()
	return hs
}

func (hs *hostStub) WriteJSON(v any) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return hs.ws.WriteJSON(v)
}

func (hs *hostStub) next(t *testing.T, v any) {
	t.Helper()
	select {
	case raw, ok := <-hs.other:
		if !ok {
			t.Fatal("host connection closed while waiting for a message")
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message on host")
	}
}

// nextSkippingPresence is like next, but discards any "presence"
// messages first. Presence broadcasts can be triggered asynchronously
// (e.g. a connection closing) and interleave unpredictably with
// whatever a test is actually asserting on - tests that care about
// presence itself use next/readMsg directly against a specific
// expected sequence; tests that don't want to hand-count every
// presence fan-out use this instead.
func (hs *hostStub) nextSkippingPresence(t *testing.T, v any) {
	t.Helper()
	for {
		select {
		case raw, ok := <-hs.other:
			if !ok {
				t.Fatal("host connection closed while waiting for a message")
			}
			var env protocol.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if env.Type == "presence" {
				continue
			}
			if err := json.Unmarshal(raw, v); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			return
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for a non-presence message on host")
		}
	}
}

// readMsgSkippingPresence is nextSkippingPresence's counterpart for a
// raw viewer *websocket.Conn - reads raw frames (not via ReadJSON,
// which would consume a frame before its type could be checked) so a
// "presence" frame can be discarded and the next one decoded into v.
func readMsgSkippingPresence(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if env.Type == "presence" {
			continue
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return
	}
}

// expectNothing fails if a message arrives on host within a short
// window - used to prove something was correctly NOT forwarded/sent.
func (hs *hostStub) expectNothing(t *testing.T) {
	t.Helper()
	select {
	case raw, ok := <-hs.other:
		if ok {
			t.Errorf("expected no message on host, got: %s", raw)
		}
	case <-time.After(300 * time.Millisecond):
		// correct - nothing arrived
	}
}

func configureSession(t *testing.T, hs *hostStub, mode string, cols, rows int) {
	t.Helper()
	if err := hs.WriteJSON(protocol.SessionConfigMsg{
		Envelope: protocol.NewEnvelope("session_config"),
		Mode:     mode,
		HostCols: cols,
		HostRows: rows,
	}); err != nil {
		t.Fatalf("configuring session: %v", err)
	}
}

// authedViewer connects and authenticates a viewer, then drains the
// presence broadcast a successful auth triggers on both sides (see
// internal/relay/presence.go) - callers want a ready-to-use pair of
// connections sitting at their next "real" message, not to manually
// account for presence every time. hs must be the host stub already
// running for this session (every current caller has one).
func authedViewer(t *testing.T, base, sessionID string, hs *hostStub) *websocket.Conn {
	t.Helper()
	viewer := dial(t, base+"/ws/viewer/"+sessionID)
	if result := authenticateViewer(t, viewer); !result.OK {
		t.Fatalf("authenticateViewer: ok=false, want true")
	}
	var viewerPresence protocol.PresenceMsg
	readMsg(t, viewer, &viewerPresence)
	var hostPresence protocol.PresenceMsg
	hs.next(t, &hostPresence)
	return viewer
}

func TestTakeControl_ViewerBecomesActiveWriter_AllGetControlChanged(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("sending take_control: %v", err)
	}

	// Host must be told control changed too - it needs this to gate its
	// own local keystrokes.
	var hostMsg protocol.ControlChangedMsg
	hs.next(t, &hostMsg)
	if hostMsg.ActiveWriterRole != "viewer" {
		t.Errorf("host's control_changed: role = %q, want viewer", hostMsg.ActiveWriterRole)
	}

	var viewerMsg protocol.ControlChangedMsg
	readMsg(t, viewer, &viewerMsg)
	if viewerMsg.ActiveWriterRole != "viewer" {
		t.Errorf("viewer's control_changed: role = %q, want viewer", viewerMsg.ActiveWriterRole)
	}
	if viewerMsg.ActiveWriterID == "" {
		t.Error("active_writer_id is empty")
	}
}

func TestInput_OnlyAppliedFromActiveWriter(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	// Viewer is not the active writer yet (host is, by default) - its
	// input must be dropped, never forwarded to the host.
	if err := viewer.WriteJSON(protocol.InputMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("should not reach host")),
	}); err != nil {
		t.Fatalf("sending input: %v", err)
	}

	hs.expectNothing(t)
}

func TestInput_ForwardedToHost_WhenSenderIsActiveWriter(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("sending take_control: %v", err)
	}
	var hostControlChanged protocol.ControlChangedMsg
	hs.next(t, &hostControlChanged)
	var viewerControlChanged protocol.ControlChangedMsg
	readMsg(t, viewer, &viewerControlChanged)

	if err := viewer.WriteJSON(protocol.InputMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("ls -la\r")),
	}); err != nil {
		t.Fatalf("sending input: %v", err)
	}

	// take_control's own presence broadcast (see handleTakeControl) is
	// still queued ahead of the input forward at this point - skip it.
	var fwd protocol.InputForwardMsg
	hs.nextSkippingPresence(t, &fwd)
	data, err := base64.StdEncoding.DecodeString(fwd.DataBase64)
	if err != nil {
		t.Fatalf("decoding forwarded input: %v", err)
	}
	if string(data) != "ls -la\r" {
		t.Errorf("forwarded input = %q, want %q", data, "ls -la\r")
	}
	if fwd.SenderID != viewerControlChanged.ActiveWriterID {
		t.Errorf("sender_id = %q, want the viewer's connection_id %q", fwd.SenderID, viewerControlChanged.ActiveWriterID)
	}
}

func TestResize_OnlyForwardedFromActiveWriter(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.ResizeMsg{
		Envelope: protocol.NewEnvelope("resize"),
		Cols:     160,
		Rows:     44,
	}); err != nil {
		t.Fatalf("sending inactive resize: %v", err)
	}
	hs.expectNothing(t)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("sending take_control: %v", err)
	}
	var hostControlChanged protocol.ControlChangedMsg
	hs.nextSkippingPresence(t, &hostControlChanged)
	var viewerControlChanged protocol.ControlChangedMsg
	readMsgSkippingPresence(t, viewer, &viewerControlChanged)

	if err := viewer.WriteJSON(protocol.ResizeMsg{
		Envelope: protocol.NewEnvelope("resize"),
		Cols:     160,
		Rows:     44,
	}); err != nil {
		t.Fatalf("sending active resize: %v", err)
	}

	var resize protocol.ResizeMsg
	hs.nextSkippingPresence(t, &resize)
	if resize.Cols != 160 || resize.Rows != 44 {
		t.Fatalf("forwarded resize = %dx%d, want 160x44", resize.Cols, resize.Rows)
	}
	if resize.Type != "resize" {
		t.Fatalf("forwarded type = %q, want resize", resize.Type)
	}
}

func TestTakeControl_HostReclaim_LocksOutViewerBriefly(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	// Viewer takes control first. Each take_control also triggers its
	// own presence broadcast (see handleTakeControl) - use the
	// presence-skipping helpers throughout this test rather than
	// hand-count exactly how many presence messages queue up on each
	// side at each step.
	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("viewer take_control: %v", err)
	}
	var discard protocol.ControlChangedMsg
	hs.nextSkippingPresence(t, &discard)
	readMsgSkippingPresence(t, viewer, &discard)

	// Host reclaims. The relay broadcasts control_changed to everyone,
	// including the host itself (unlike output, which excludes it).
	if err := hs.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
		t.Fatalf("host take_control: %v", err)
	}
	var hostOwnEcho protocol.ControlChangedMsg
	hs.nextSkippingPresence(t, &hostOwnEcho)
	var afterHostReclaim protocol.ControlChangedMsg
	readMsgSkippingPresence(t, viewer, &afterHostReclaim)
	if afterHostReclaim.ActiveWriterRole != "host" {
		t.Fatalf("after host reclaim, role = %q, want host", afterHostReclaim.ActiveWriterRole)
	}
	// The reclaim's own presence broadcast arrives AFTER its
	// control_changed (see handleTakeControl) - nextSkippingPresence
	// above only skips presence messages queued BEFORE the target it
	// returns, not ones that arrive after, so this one is still queued
	// on host and must be drained explicitly before the later
	// expectNothing check.
	var hostReclaimPresence protocol.PresenceMsg
	hs.next(t, &hostReclaimPresence)

	// Viewer immediately tries to take it back - must be rejected while
	// inside HostLockWindow.
	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 120, Rows: 36}); err != nil {
		t.Fatalf("viewer take_control (should be locked out): %v", err)
	}
	var errMsg protocol.ErrorMsg
	readMsgSkippingPresence(t, viewer, &errMsg)
	if errMsg.Code != protocol.ErrNotActiveWriter {
		t.Errorf("code = %q, want %q", errMsg.Code, protocol.ErrNotActiveWriter)
	}

	// Host must not have received a spurious control_changed (or
	// presence - a rejected take_control changes nothing) from the
	// rejected attempt.
	hs.expectNothing(t)
}
