package relay

import (
	"encoding/base64"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestRemoteMode_AuthResultCarriesCanonicalSessionState(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	configureSession(t, hs, protocol.SessionModeRemote, 180, 50)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, viewer)

	if !result.OK {
		t.Fatal("auth failed")
	}
	if result.Mode != protocol.SessionModeRemote || result.Cols != 180 || result.Rows != 50 {
		t.Fatalf("auth session state = mode %q, %dx%d; want remote, 180x50", result.Mode, result.Cols, result.Rows)
	}
	if result.ActiveWriterID != "host" || result.ActiveWriterRole != "host" {
		t.Fatalf("active writer = %q/%q, want host/host", result.ActiveWriterID, result.ActiveWriterRole)
	}
}

func TestRemoteMode_RejectsSecondAuthenticatedIdentity(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	first := authedViewer(t, base, created.SessionID, hs)
	_ = first
	second := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, second)

	if result.OK || result.Code != protocol.AuthCodeOccupied {
		t.Fatalf("second auth = ok %v code %q, want false/%q", result.OK, result.Code, protocol.AuthCodeOccupied)
	}
}

func TestGroupMode_RejectsViewerControlInputAndResize(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	configureSession(t, hs, protocol.SessionModeGroup, 160, 44)
	viewer := authedViewer(t, base, created.SessionID, hs)

	attempts := []any{
		protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control"), Cols: 40, Rows: 20},
		protocol.InputMsg{Envelope: protocol.NewEnvelope("input"), DataBase64: base64.StdEncoding.EncodeToString([]byte("no"))},
		protocol.ResizeMsg{Envelope: protocol.NewEnvelope("resize"), Cols: 40, Rows: 20},
	}

	for _, attempt := range attempts {
		if err := viewer.WriteJSON(attempt); err != nil {
			t.Fatalf("sending forbidden group action: %v", err)
		}
		var got protocol.ErrorMsg
		readMsg(t, viewer, &got)
		if got.Code != protocol.ErrReadOnlySession {
			t.Fatalf("group action error = %q, want %q", got.Code, protocol.ErrReadOnlySession)
		}
	}
	hs.expectNothing(t)
}

func TestRemoteMode_ActiveViewerDisconnectRestoresLatestHostGeometry(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	configureSession(t, hs, protocol.SessionModeRemote, 180, 50)
	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{
		Envelope: protocol.NewEnvelope("take_control"),
		Cols:     40,
		Rows:     20,
	}); err != nil {
		t.Fatalf("taking control: %v", err)
	}
	var viewerControl protocol.ControlChangedMsg
	hs.nextSkippingPresence(t, &viewerControl)
	readMsgSkippingPresence(t, viewer, &viewerControl)

	if err := hs.WriteJSON(protocol.HostSizeMsg{
		Envelope: protocol.NewEnvelope("host_size"),
		Cols:     200,
		Rows:     60,
	}); err != nil {
		t.Fatalf("reporting host size: %v", err)
	}
	// Host and viewer are separate sockets, so synchronize on a later host
	// message before closing the viewer. This proves the size is the latest one
	// processed by the relay rather than assuming cross-socket arrival order.
	if err := hs.WriteJSON(protocol.ChatMsg{
		Envelope: protocol.NewEnvelope("chat_message"),
		Text:     "size barrier",
	}); err != nil {
		t.Fatalf("sending host size barrier: %v", err)
	}
	var hostBarrier protocol.ChatBroadcastMsg
	hs.nextSkippingPresence(t, &hostBarrier)
	var viewerBarrier protocol.ChatBroadcastMsg
	readMsgSkippingPresence(t, viewer, &viewerBarrier)
	if err := viewer.Close(); err != nil {
		t.Fatalf("closing viewer: %v", err)
	}

	var restored protocol.ControlChangedMsg
	hs.nextSkippingPresence(t, &restored)
	if restored.ActiveWriterRole != "host" || restored.Cols != 200 || restored.Rows != 60 {
		t.Fatalf("restored state = %s %dx%d, want host 200x60", restored.ActiveWriterRole, restored.Cols, restored.Rows)
	}
}
