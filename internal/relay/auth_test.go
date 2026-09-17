package relay

import (
	"encoding/base64"
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
	// even reaching the host - simulateHostAuthResponder only answers
	// auth_request, so if the relay still contacted the host we'd get
	// AUTH_FAILED, not RATE_LIMITED.
	result := authenticateViewer(t, viewer)
	if result.Code != protocol.AuthCodeRateLimited {
		t.Errorf("code = %q, want %q after %d failed attempts", result.Code, protocol.AuthCodeRateLimited, RateLimitMaxAttempts)
	}
	if result.RetryAfterMs <= 0 {
		t.Errorf("retry_after_ms = %d, want > 0", result.RetryAfterMs)
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
	var got protocol.OutputMsg
	readMsg(t, viewer2, &got)
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
