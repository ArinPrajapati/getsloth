package relay

import (
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestEndSession_ExplicitCleanEnd_UsesProcessExitedReason(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID)

	if err := hs.WriteJSON(protocol.EndSessionMsg{Envelope: protocol.NewEnvelope("end_session")}); err != nil {
		t.Fatalf("sending end_session: %v", err)
	}

	var ended protocol.SessionEndedMsg
	readMsg(t, viewer, &ended)
	if ended.Reason != protocol.ReasonProcessExited {
		t.Errorf("reason = %q, want %q (an explicit end_session, not an abrupt drop)", ended.Reason, protocol.ReasonProcessExited)
	}
}

func TestReconnectAfterSessionEnded_GetsCleanRejection_NotHang(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)

	if err := host.Close(); err != nil {
		t.Fatalf("closing host: %v", err)
	}
	// Give the relay's handleHost loop a moment to detect the
	// disconnect, tear down, and remove the session from the registry.
	time.Sleep(100 * time.Millisecond)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	var errMsg protocol.ErrorMsg
	readMsg(t, viewer, &errMsg)

	if errMsg.Code != protocol.ErrSessionNotFound {
		t.Errorf("code = %q, want %q - a viewer connecting after the session ended should be cleanly rejected, not hang", errMsg.Code, protocol.ErrSessionNotFound)
	}

	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Error("connection wasn't closed after SESSION_NOT_FOUND post-teardown")
	}
}
