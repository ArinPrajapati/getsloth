package relay

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newTestServer(t *testing.T) (wsURL string, cleanup func()) {
	t.Helper()
	srv := NewServer()
	ts := httptest.NewServer(srv.Handler())
	return "ws" + strings.TrimPrefix(ts.URL, "http"), ts.Close
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readMsg(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn.ReadJSON(v); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
}

func TestHostConnect_CreatesSessionAndReceivesSessionCreated(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")

	var msg SessionCreatedMsg
	readMsg(t, host, &msg)

	if msg.Type != "session_created" {
		t.Errorf("type = %q, want session_created", msg.Type)
	}
	if msg.SessionID == "" {
		t.Error("session_id is empty")
	}
	if msg.ConnectionID == "" {
		t.Error("connection_id is empty")
	}
}

func TestTwoHostConnections_GetDifferentSessions(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host1 := dial(t, base+"/ws/host")
	var msg1 SessionCreatedMsg
	readMsg(t, host1, &msg1)

	host2 := dial(t, base+"/ws/host")
	var msg2 SessionCreatedMsg
	readMsg(t, host2, &msg2)

	if msg1.SessionID == msg2.SessionID {
		t.Errorf("two separate host connections got the same session_id %q", msg1.SessionID)
	}
}

func TestViewerConnect_UnknownSession_GetsCleanRejection(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	viewer := dial(t, base+"/ws/viewer/does-not-exist")

	var msg ErrorMsg
	readMsg(t, viewer, &msg)

	if msg.Type != "error" {
		t.Errorf("type = %q, want error", msg.Type)
	}
	if msg.Code != ErrSessionNotFound {
		t.Errorf("code = %q, want %q", msg.Code, ErrSessionNotFound)
	}

	// The relay must then close the socket, not leave it hanging open.
	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Error("expected the connection to be closed after SESSION_NOT_FOUND, it wasn't")
	}
}

func TestViewerConnect_KnownSession_Succeeds(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created SessionCreatedMsg
	readMsg(t, host, &created)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)

	// No message is expected before auth (the auth public key now lives
	// in the share URL fragment, not delivered by the relay - see
	// docs/protocol.md). A short read with a deadline confirms the
	// relay doesn't send anything unexpected and doesn't close the
	// connection either.
	_ = viewer.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Error("expected no message before auth, but received one")
	} else if !websocket.IsCloseError(err) && !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected a read timeout (connection stayed open), got: %v", err)
	}
}

func TestHostDisconnect_TearsDownSessionAndDisconnectsViewers(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created SessionCreatedMsg
	readMsg(t, host, &created)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	// Drain the "no message before auth" state by giving the viewer
	// registration a moment to land server-side before we disconnect
	// the host.
	time.Sleep(50 * time.Millisecond)

	if err := host.Close(); err != nil {
		t.Fatalf("closing host connection: %v", err)
	}

	var ended SessionEndedMsg
	readMsg(t, viewer, &ended)

	if ended.Type != "session_ended" {
		t.Errorf("type = %q, want session_ended", ended.Type)
	}
	if ended.Reason != ReasonHostDisconnected {
		t.Errorf("reason = %q, want %q", ended.Reason, ReasonHostDisconnected)
	}
}
