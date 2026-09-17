package relay

import (
	"net/http/httptest"
	"strings"
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

// TestHostDisconnect_ClearsRateLimiterState is the regression test for
// a real leak found in review: rate-limiter entries were never tied to
// session lifecycle, so they persisted for the life of the relay
// *process* even after the session they belonged to was long gone.
// Uses NewServer directly (rather than newTestServer, which only
// exposes the URL) since this needs to inspect the server's internal
// rate-limiter state, not just observe protocol behavior.
func TestHostDisconnect_ClearsRateLimiterState(t *testing.T) {
	srv := NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	newHostStub(t, host, false) // every attempt fails, to populate a rate-limiter entry

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	if result := authenticateViewer(t, viewer); result.OK {
		t.Fatalf("attempt unexpectedly succeeded")
	}

	srv.rateLimiter.mu.Lock()
	before := len(srv.rateLimiter.entries)
	srv.rateLimiter.mu.Unlock()
	if before == 0 {
		t.Fatal("no rate-limiter entry was created by the failed attempt - test setup didn't exercise what it's supposed to")
	}

	if err := host.Close(); err != nil {
		t.Fatalf("closing host: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let handleHost's teardown path run

	srv.rateLimiter.mu.Lock()
	after := len(srv.rateLimiter.entries)
	srv.rateLimiter.mu.Unlock()
	if after != 0 {
		t.Errorf("rate-limiter still has %d entries after the session torn down, want 0", after)
	}
}
