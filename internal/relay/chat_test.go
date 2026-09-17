package relay

import (
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestChat_ViewerMessage_ReachesHostAndOtherViewers(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer1 := authedViewer(t, base, created.SessionID, hs)
	viewer2 := authedViewer(t, base, created.SessionID, hs)

	if err := viewer1.WriteJSON(protocol.ChatMsg{
		Envelope: protocol.NewEnvelope("chat_message"),
		Text:     "hello from viewer1",
	}); err != nil {
		t.Fatalf("sending chat: %v", err)
	}

	var hostMsg protocol.ChatBroadcastMsg
	hs.next(t, &hostMsg)
	if hostMsg.Text != "hello from viewer1" {
		t.Errorf("host received text = %q, want %q", hostMsg.Text, "hello from viewer1")
	}
	if hostMsg.SenderRole != "viewer" {
		t.Errorf("sender_role = %q, want viewer", hostMsg.SenderRole)
	}
	if hostMsg.SenderID == "" {
		t.Error("sender_id is empty")
	}

	var v2Msg protocol.ChatBroadcastMsg
	readMsg(t, viewer2, &v2Msg)
	if v2Msg.Text != "hello from viewer1" {
		t.Errorf("viewer2 received text = %q, want %q", v2Msg.Text, "hello from viewer1")
	}

	// The sender must ALSO receive their own message broadcast back:
	// the Frontend chat panel (web/src/chat-panel.ts) has no optimistic
	// local echo - it only renders a sent message once it arrives via
	// the relay's broadcast, same as everyone else's. Without this, a
	// sender would never see their own message appear in their own
	// chat log. viewer1 also has an undrained presence message sitting
	// ahead of it from viewer2 joining (authedViewer only drains the
	// *new* viewer's and host's copies, not already-connected viewers'
	// copies) - skip past it.
	var selfEcho protocol.ChatBroadcastMsg
	readMsgSkippingPresence(t, viewer1, &selfEcho)
	if selfEcho.Text != "hello from viewer1" {
		t.Errorf("sender's own echo text = %q, want %q", selfEcho.Text, "hello from viewer1")
	}
}

func TestChat_HostMessage_ReachesViewers(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := hs.WriteJSON(protocol.ChatMsg{
		Envelope: protocol.NewEnvelope("chat_message"),
		Text:     "hello from host",
	}); err != nil {
		t.Fatalf("sending chat: %v", err)
	}

	var got protocol.ChatBroadcastMsg
	readMsg(t, viewer, &got)
	if got.Text != "hello from host" {
		t.Errorf("viewer received text = %q, want %q", got.Text, "hello from host")
	}
	if got.SenderRole != "host" {
		t.Errorf("sender_role = %q, want host", got.SenderRole)
	}
}

func TestChat_NeverAppliedToPTY(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.ChatMsg{
		Envelope: protocol.NewEnvelope("chat_message"),
		Text:     "this must never reach the PTY",
	}); err != nil {
		t.Fatalf("sending chat: %v", err)
	}

	// A chat message must never trigger an InputForwardMsg to the host -
	// only the ChatBroadcastMsg it also expects.
	var got protocol.ChatBroadcastMsg
	hs.next(t, &got)
	if got.Type != "chat_message" {
		t.Fatalf("host's next message was type %q, want chat_message (an input forward would mean chat leaked into the PTY path)", got.Type)
	}
}
