package relay

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestAuth_CorrectPassword_GrantsTokenAndUnlocksOutput(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, viewer)

	if !result.OK {
		t.Fatalf("auth_result.ok = false, want true")
	}
	if result.Token == "" {
		t.Error("no token issued on successful auth")
	}
	if result.ConnectionID == "" {
		t.Error("no connection_id returned on successful auth")
	}

	// A successful auth also triggers a presence broadcast on both
	// sides (see internal/relay/presence.go) - drain it before the
	// output exchange this test actually checks.
	var hostPresence protocol.PresenceMsg
	hs.next(t, &hostPresence)
	var viewerPresence protocol.PresenceMsg
	readMsg(t, viewer, &viewerPresence)

	if err := hs.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("now visible")),
	}); err != nil {
		t.Fatalf("host WriteJSON: %v", err)
	}
	var got protocol.OutputMsg
	readMsg(t, viewer, &got)
	data, _ := base64.StdEncoding.DecodeString(got.DataBase64)
	if string(data) != "now visible" {
		t.Errorf("viewer received %q after auth, want %q", data, "now visible")
	}
}

func TestAuth_CorrectPassword_ReplaysRecentOutput(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	if err := hs.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("sh-3.2$ ")),
	}); err != nil {
		t.Fatalf("host WriteJSON: %v", err)
	}

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, viewer)
	if !result.OK {
		t.Fatalf("auth_result.ok = false, want true")
	}

	var got protocol.OutputMsg
	readMsg(t, viewer, &got)
	data, _ := base64.StdEncoding.DecodeString(got.DataBase64)
	if string(data) != "sh-3.2$ " {
		t.Errorf("replayed output = %q, want shell prompt", data)
	}
}

func TestAuth_WrongPassword_Rejected_OutputStaysGated(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, false)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, viewer)

	if result.OK {
		t.Fatal("auth_result.ok = true for a rejected password, want false")
	}
	if result.Code != protocol.AuthCodeFailed {
		t.Errorf("code = %q, want %q", result.Code, protocol.AuthCodeFailed)
	}
	if result.Token != "" {
		t.Error("a token was issued despite auth failing")
	}

	// Output still must not reach this viewer.
	if err := hs.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("should not arrive")),
	}); err != nil {
		t.Fatalf("host WriteJSON: %v", err)
	}
	_ = viewer.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Error("unauthenticated viewer received output, expected none")
	}
}

func TestAuth_UnauthenticatedMessage_GetsUnauthorizedNotClosed(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	if err := viewer.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"), // arbitrary non-auth message type
		DataBase64: "irrelevant",
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var errMsg protocol.ErrorMsg
	readMsg(t, viewer, &errMsg)
	if errMsg.Code != protocol.ErrUnauthorized {
		t.Errorf("code = %q, want %q", errMsg.Code, protocol.ErrUnauthorized)
	}

	// Per docs/protocol.md, UNAUTHORIZED must not close the connection -
	// confirm the viewer can still successfully authenticate afterward.
	newHostStub(t, host, true)
	if result := authenticateViewer(t, viewer); !result.OK {
		t.Error("could not authenticate after an earlier UNAUTHORIZED - connection may have been incorrectly closed")
	}
}

func TestAuth_RateLimitedAfterMaxFailures(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	newHostStub(t, host, false) // every attempt fails

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)

	for i := 0; i < RateLimitMaxAttempts; i++ {
		result := authenticateViewer(t, viewer)
		if result.OK {
			t.Fatalf("attempt #%d unexpectedly succeeded", i)
		}
	}

	// The (RateLimitMaxAttempts+1)th attempt must be rejected without
	// even reaching the host - hostStub only answers auth_request, so
	// if the relay still contacted the host we'd get AUTH_FAILED, not
	// RATE_LIMITED.
	result := authenticateViewer(t, viewer)
	if result.Code != protocol.AuthCodeRateLimited {
		t.Errorf("code = %q, want %q after %d failed attempts", result.Code, protocol.AuthCodeRateLimited, RateLimitMaxAttempts)
	}
	if result.RetryAfterMs <= 0 {
		t.Errorf("retry_after_ms = %d, want > 0", result.RetryAfterMs)
	}
}

// TestAuth_ConcurrentFlood_BeforeAnyResponse_IsCapped is the regression
// test for a real vulnerability found in review: the cooldown-based
// rate limit only counts *completed* round-trips (it's incremented from
// the host's response), so a burst of auth messages sent before any of
// them has had time to fail wasn't limited at all - each one reached
// the host regardless of how many were already in flight. This proves
// MaxConcurrentAuthAttempts closes that gap independently of the
// cooldown mechanism.
func TestAuth_ConcurrentFlood_BeforeAnyResponse_IsCapped(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)

	// A host stub that reads auth_request but deliberately never
	// responds - holding every attempt "in flight" for the duration of
	// the test, which is exactly the flood scenario: many attempts sent
	// before any round-trip completes.
	authRequests := make(chan protocol.AuthRequestMsg, MaxConcurrentAuthAttempts+2)
	go func() {
		for {
			_, raw, err := host.ReadMessage()
			if err != nil {
				return
			}
			var req protocol.AuthRequestMsg
			if err := json.Unmarshal(raw, &req); err != nil {
				continue
			}
			authRequests <- req
		}
	}()

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)

	for i := 0; i < MaxConcurrentAuthAttempts; i++ {
		if err := viewer.WriteJSON(protocol.AuthMsg{
			Envelope:           protocol.NewEnvelope("auth"),
			ViewerPubkeyBase64: "test-pubkey",
			CiphertextBase64:   "test-ciphertext",
		}); err != nil {
			t.Fatalf("sending auth attempt #%d: %v", i, err)
		}
	}

	// Confirm all MaxConcurrentAuthAttempts genuinely reached the host -
	// otherwise the next check (the extra one being rejected) would be
	// trivially true for the wrong reason.
	for i := 0; i < MaxConcurrentAuthAttempts; i++ {
		select {
		case <-authRequests:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d attempts reached the host", i, MaxConcurrentAuthAttempts)
		}
	}

	// The next attempt, sent while all prior ones are still
	// unresolved, must be rejected as RATE_LIMITED without an
	// auth_request ever reaching the host for it.
	result := authenticateViewer(t, viewer)
	if result.Code != protocol.AuthCodeRateLimited {
		t.Errorf("code = %q, want %q for an attempt beyond MaxConcurrentAuthAttempts with none resolved", result.Code, protocol.AuthCodeRateLimited)
	}
	select {
	case <-authRequests:
		t.Error("an auth_request reached the host beyond MaxConcurrentAuthAttempts - the concurrent cap did not hold")
	case <-time.After(300 * time.Millisecond):
		// correct - nothing more arrived
	}
}

func TestResume_ValidToken_SkipsHostRoundTrip(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer1 := dial(t, base+"/ws/viewer/"+created.SessionID)
	firstAuth := authenticateViewer(t, viewer1)
	if !firstAuth.OK {
		t.Fatalf("initial auth failed")
	}
	_ = viewer1.Close()

	// Reconnect as a new WebSocket connection (simulating a brief
	// network drop), presenting the token instead of re-authenticating.
	viewer2 := dial(t, base+"/ws/viewer/"+created.SessionID)
	if err := viewer2.WriteJSON(protocol.ResumeMsg{
		Envelope: protocol.NewEnvelope("resume"),
		Token:    firstAuth.Token,
	}); err != nil {
		t.Fatalf("sending resume: %v", err)
	}
	var result protocol.AuthResultMsg
	readMsg(t, viewer2, &result)

	if !result.OK {
		t.Fatal("resume with a valid token failed")
	}
	if result.ConnectionID != firstAuth.ConnectionID {
		t.Errorf("resume connection_id = %q, want the original %q", result.ConnectionID, firstAuth.ConnectionID)
	}

	// Output must reach the resumed connection without any further auth.
	if err := hs.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: base64.StdEncoding.EncodeToString([]byte("resumed")),
	}); err != nil {
		t.Fatalf("host WriteJSON: %v", err)
	}
	// viewer1's disconnect (above) can trigger its own presence
	// broadcast at an unpredictable time relative to this read, since
	// the server detects the close asynchronously - skip past it rather
	// than assume a fixed message count.
	var got protocol.OutputMsg
	readMsgSkippingPresence(t, viewer2, &got)
	data, _ := base64.StdEncoding.DecodeString(got.DataBase64)
	if string(data) != "resumed" {
		t.Errorf("resumed viewer received %q, want %q", data, "resumed")
	}
}

func TestResume_InvalidToken_Rejected(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	if err := viewer.WriteJSON(protocol.ResumeMsg{
		Envelope: protocol.NewEnvelope("resume"),
		Token:    "not-a-real-token",
	}); err != nil {
		t.Fatalf("sending resume: %v", err)
	}
	var result protocol.AuthResultMsg
	readMsg(t, viewer, &result)

	if result.OK {
		t.Error("resume with an invalid token succeeded, want rejected")
	}
}
