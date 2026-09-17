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

func authedViewer(t *testing.T, base, sessionID string) *websocket.Conn {
	t.Helper()
	viewer := dial(t, base+"/ws/viewer/"+sessionID)
	if result := authenticateViewer(t, viewer); !result.OK {
		t.Fatalf("authenticateViewer: ok=false, want true")
	}
	return viewer
}

func TestTakeControl_ViewerBecomesActiveWriter_AllGetControlChanged(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
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

	viewer := authedViewer(t, base, created.SessionID)

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

	viewer := authedViewer(t, base, created.SessionID)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
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

	var fwd protocol.InputForwardMsg
	hs.next(t, &fwd)
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

func TestTakeControl_HostReclaim_LocksOutViewerBriefly(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID)

	// Viewer takes control first.
	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
		t.Fatalf("viewer take_control: %v", err)
	}
	var discard protocol.ControlChangedMsg
	hs.next(t, &discard)
	readMsg(t, viewer, &discard)

	// Host reclaims. The relay broadcasts control_changed to everyone,
	// including the host itself (unlike output, which excludes it) - so
	// hs.other gets this too and must be drained before the later
	// expectNothing check, or it would be mistaken for a spurious
	// broadcast from the rejected attempt below.
	if err := hs.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
		t.Fatalf("host take_control: %v", err)
	}
	var hostOwnEcho protocol.ControlChangedMsg
	hs.next(t, &hostOwnEcho)
	var afterHostReclaim protocol.ControlChangedMsg
	readMsg(t, viewer, &afterHostReclaim)
	if afterHostReclaim.ActiveWriterRole != "host" {
		t.Fatalf("after host reclaim, role = %q, want host", afterHostReclaim.ActiveWriterRole)
	}

	// Viewer immediately tries to take it back - must be rejected while
	// inside HostLockWindow.
	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
		t.Fatalf("viewer take_control (should be locked out): %v", err)
	}
	var errMsg protocol.ErrorMsg
	readMsg(t, viewer, &errMsg)
	if errMsg.Code != protocol.ErrNotActiveWriter {
		t.Errorf("code = %q, want %q", errMsg.Code, protocol.ErrNotActiveWriter)
	}

	// Host must not have received a spurious control_changed from the
	// rejected attempt.
	hs.expectNothing(t)
}
