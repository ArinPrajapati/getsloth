package relay

import (
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestKillSwitch_DisconnectsViewers_SessionStaysAlive(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID)

	if err := hs.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")}); err != nil {
		t.Fatalf("sending kill_switch: %v", err)
	}

	var kicked protocol.KickedMsg
	readMsg(t, viewer, &kicked)
	if kicked.Type != "kicked" {
		t.Errorf("type = %q, want kicked", kicked.Type)
	}
	if kicked.Reason != protocol.ReasonKillSwitch {
		t.Errorf("reason = %q, want %q", kicked.Reason, protocol.ReasonKillSwitch)
	}

	// The relay must then close the viewer's socket.
	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Error("viewer connection wasn't closed after kill_switch")
	} else if closeErr, ok := err.(*websocket.CloseError); ok && closeErr.Code != protocol.CloseKicked {
		t.Errorf("close code = %d, want %d", closeErr.Code, protocol.CloseKicked)
	}

	// The session itself must survive: a fresh viewer can still connect
	// and authenticate afterward (host's key/password logic is
	// untouched - the relay doesn't hold any of that state to reset).
	viewer2 := authedViewer(t, base, created.SessionID)
	if viewer2 == nil {
		t.Fatal("could not connect a new viewer after kill_switch - session did not survive")
	}
}

func TestKillSwitch_HostConnectionUnaffected(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	_ = authedViewer(t, base, created.SessionID)

	if err := hs.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")}); err != nil {
		t.Fatalf("sending kill_switch: %v", err)
	}

	// The host shouldn't receive a kicked message about itself, and its
	// connection must stay open and usable - confirmed by successfully
	// sending another message afterward (output) and seeing no error.
	hs.expectNothing(t)
	if err := hs.WriteJSON(protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: "c3RpbGwgYWxpdmU=", // "still alive"
	}); err != nil {
		t.Fatalf("host connection unusable after kill_switch: %v", err)
	}
}
