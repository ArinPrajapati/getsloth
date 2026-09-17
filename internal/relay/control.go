package relay

import (
	"encoding/json"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// handleTakeControl reassigns the active writer to conn and broadcasts
// control_changed to the host and every authenticated viewer. A
// host-originated take_control is authoritative: it always succeeds and
// starts HostLockWindow, during which any non-host take_control is
// rejected with NOT_ACTIVE_WRITER instead of reassigning - this is what
// makes "the host always has an instant override" actually true, per
// docs/protocol.md's Control model.
func (s *Server) handleTakeControl(session *Session, conn *Connection) {
	isHost := conn.ID() == "host"

	session.mu.Lock()
	if !isHost && time.Since(session.lastHostReclaim) < HostLockWindow {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrNotActiveWriter,
			Message:  "host recently reclaimed control",
		})
		return
	}

	role := "viewer"
	if isHost {
		role = "host"
		session.lastHostReclaim = time.Now()
	}
	session.activeWriterID = conn.ID()
	session.activeWriterRole = role

	host := session.host
	viewers := make([]*Connection, 0, len(session.viewers))
	for _, v := range session.viewers {
		if v.isAuthenticated() {
			viewers = append(viewers, v)
		}
	}
	session.mu.Unlock()

	msg := protocol.ControlChangedMsg{
		Envelope:         protocol.NewEnvelope("control_changed"),
		ActiveWriterID:   conn.ID(),
		ActiveWriterRole: role,
	}
	if host != nil {
		_ = host.writeJSON(msg)
	}
	for _, v := range viewers {
		_ = v.writeJSON(msg)
	}
}

// handleInput forwards a viewer's input to the host, but only if conn is
// the current active writer - otherwise it's silently dropped, never
// forwarded, per docs/protocol.md's Control model. NOT_ACTIVE_WRITER is
// still sent back so the UI can react, even though the drop itself
// isn't an error condition.
func (s *Server) handleInput(session *Session, conn *Connection, raw []byte) {
	session.mu.Lock()
	isActive := session.activeWriterID == conn.ID()
	host := session.host
	session.mu.Unlock()

	if !isActive {
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrNotActiveWriter,
			Message:  "not the active writer",
		})
		return
	}
	if host == nil {
		return
	}

	var msg protocol.InputMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	_ = host.writeJSON(protocol.InputForwardMsg{
		Envelope:   protocol.NewEnvelope("input"),
		DataBase64: msg.DataBase64,
		SenderID:   conn.ID(),
	})
}
