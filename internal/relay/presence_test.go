package relay

import (
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// findConn returns the entry for id, or nil.
func findConn(connections []protocol.PresenceConnectionInfo, id string) *protocol.PresenceConnectionInfo {
	for i := range connections {
		if connections[i].ID == id {
			return &connections[i]
		}
	}
	return nil
}

func TestPresence_BroadcastOnViewerAuth(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)
	result := authenticateViewer(t, viewer)
	if !result.OK {
		t.Fatalf("auth failed")
	}

	// Host must see a presence update including the new viewer.
	var hostPresence protocol.PresenceMsg
	hs.next(t, &hostPresence)
	if hostPresence.Type != "presence" {
		t.Fatalf("host's next message was %q, want presence", hostPresence.Type)
	}
	if entry := findConn(hostPresence.Connections, result.ConnectionID); entry == nil {
		t.Fatalf("presence connections %+v does not include the new viewer %q", hostPresence.Connections, result.ConnectionID)
	}
	if hostEntry := findConn(hostPresence.Connections, "host"); hostEntry == nil || hostEntry.Role != "host" {
		t.Errorf("presence connections %+v does not correctly include the host", hostPresence.Connections)
	}

	// The viewer must also receive it, including themselves.
	var viewerPresence protocol.PresenceMsg
	readMsg(t, viewer, &viewerPresence)
	if entry := findConn(viewerPresence.Connections, result.ConnectionID); entry == nil {
		t.Fatalf("viewer's own presence view %+v does not include themselves", viewerPresence.Connections)
	}
}

func TestPresence_ReflectsActiveWriterAfterTakeControl(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")}); err != nil {
		t.Fatalf("sending take_control: %v", err)
	}

	// control_changed arrives first (existing behavior), then presence
	// reflecting the new is_active_writer state.
	var hostControlChanged protocol.ControlChangedMsg
	hs.next(t, &hostControlChanged)
	var hostPresence protocol.PresenceMsg
	hs.next(t, &hostPresence)

	viewerEntry := findConn(hostPresence.Connections, hostControlChanged.ActiveWriterID)
	if viewerEntry == nil || !viewerEntry.IsActiveWriter {
		t.Errorf("presence after take_control %+v does not mark the new active writer", hostPresence.Connections)
	}
	if hostEntry := findConn(hostPresence.Connections, "host"); hostEntry == nil || hostEntry.IsActiveWriter {
		t.Errorf("presence after take_control still marks host as active writer: %+v", hostPresence.Connections)
	}
}

func TestPresence_BroadcastOnViewerDisconnect(t *testing.T) {
	base, cleanup := newTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)

	viewer := authedViewer(t, base, created.SessionID, hs)

	if err := viewer.Close(); err != nil {
		t.Fatalf("closing viewer: %v", err)
	}

	var afterLeave protocol.PresenceMsg
	hs.next(t, &afterLeave)
	if len(afterLeave.Connections) != 1 {
		t.Errorf("presence after viewer disconnect has %d connections, want 1 (host only): %+v", len(afterLeave.Connections), afterLeave.Connections)
	}
}
