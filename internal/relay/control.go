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
func (s *Server) handleTakeControl(session *Session, conn *Connection, request protocol.TakeControlMsg) {
	isHost := conn.ID() == "host"

	session.mu.Lock()
	if !isHost && session.mode == protocol.SessionModeGroup {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrReadOnlySession,
			Message:  "group sessions are host-controlled",
		})
		return
	}
	if !isHost && time.Since(session.lastHostReclaim) < HostLockWindow {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrNotActiveWriter,
			Message:  "host recently reclaimed control",
		})
		return
	}
	if !isHost && !validTerminalSize(request.Cols, request.Rows) {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrBadRequest,
			Message:  "take_control requires valid terminal dimensions",
		})
		return
	}

	role := "viewer"
	cols, rows := request.Cols, request.Rows
	if isHost {
		role = "host"
		session.lastHostReclaim = time.Now()
		cols, rows = session.hostCols, session.hostRows
	}
	session.activeWriterID = conn.ID()
	session.activeWriterRole = role
	if session.cols != cols || session.rows != rows {
		session.outputBacklog = nil
	}
	session.cols = cols
	session.rows = rows

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
		Cols:             cols,
		Rows:             rows,
	}
	if host != nil {
		_ = host.writeJSON(msg)
	}
	for _, v := range viewers {
		_ = v.writeJSON(msg)
	}
	// presence's per-connection is_active_writer flag would otherwise
	// go stale until the next unrelated roster change.
	s.broadcastPresence(session)
}

// handleInput forwards a viewer's input to the host, but only if conn is
// the current active writer - otherwise it's silently dropped, never
// forwarded, per docs/protocol.md's Control model. NOT_ACTIVE_WRITER is
// still sent back so the UI can react, even though the drop itself
// isn't an error condition.
func (s *Server) handleInput(session *Session, conn *Connection, raw []byte) {
	session.mu.Lock()
	mode := session.mode
	isActive := session.activeWriterID == conn.ID()
	host := session.host
	session.mu.Unlock()

	if mode == protocol.SessionModeGroup {
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrReadOnlySession,
			Message:  "group sessions are host-controlled",
		})
		return
	}
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

// handleResize forwards the active viewer's browser terminal size to the
// host so the real PTY rows/cols match the visible xterm grid. Without
// this, full-screen terminal UIs render into whatever small default size
// the PTY had, leaving nvim/htop/tmux painted only in the top-left of a
// much larger browser terminal.
func (s *Server) handleResize(session *Session, conn *Connection, raw []byte) {
	var msg protocol.ResizeMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if !validTerminalSize(msg.Cols, msg.Rows) {
		return
	}

	session.mu.Lock()
	if session.mode == protocol.SessionModeGroup {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrReadOnlySession,
			Message:  "group sessions are host-controlled",
		})
		return
	}
	if session.activeWriterID != conn.ID() {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrNotActiveWriter,
			Message:  "not the active writer",
		})
		return
	}
	host := session.host
	if session.cols != msg.Cols || session.rows != msg.Rows {
		session.outputBacklog = nil
	}
	session.cols = msg.Cols
	session.rows = msg.Rows
	viewers := authenticatedViewersLocked(session)
	session.mu.Unlock()

	if host == nil {
		return
	}

	_ = host.writeJSON(protocol.ResizeMsg{
		Envelope: protocol.NewEnvelope("resize"),
		Cols:     msg.Cols,
		Rows:     msg.Rows,
	})
	broadcastTerminalSize(viewers, msg.Cols, msg.Rows)
}

func (s *Server) handleHostSize(session *Session, msg protocol.HostSizeMsg) {
	if !validTerminalSize(msg.Cols, msg.Rows) {
		return
	}

	session.mu.Lock()
	session.hostCols = msg.Cols
	session.hostRows = msg.Rows
	if session.activeWriterRole != "host" {
		session.mu.Unlock()
		return
	}
	if session.cols != msg.Cols || session.rows != msg.Rows {
		session.outputBacklog = nil
	}
	session.cols = msg.Cols
	session.rows = msg.Rows
	viewers := authenticatedViewersLocked(session)
	session.mu.Unlock()

	broadcastTerminalSize(viewers, msg.Cols, msg.Rows)
}

func (s *Server) handleViewerDisconnect(session *Session, conn *Connection) {
	id := conn.ID()
	authenticated := conn.isAuthenticated()

	session.mu.Lock()
	delete(session.viewers, id)
	if authenticated {
		expires := time.Now().Add(ReconnectWindow)
		for token, record := range session.tokens {
			if record.connectionID == id {
				record.expiresAt = expires
				session.tokens[token] = record
			}
		}
		if session.mode == protocol.SessionModeRemote && session.remoteViewerID == id {
			session.remoteSlotExpiry = expires
		}
	}

	wasActive := session.activeWriterID == id
	if wasActive {
		session.activeWriterID = "host"
		session.activeWriterRole = "host"
		session.cols = session.hostCols
		session.rows = session.hostRows
		session.outputBacklog = nil
	}
	host := session.host
	viewers := authenticatedViewersLocked(session)
	cols, rows := session.cols, session.rows
	session.mu.Unlock()

	if wasActive {
		msg := protocol.ControlChangedMsg{
			Envelope:         protocol.NewEnvelope("control_changed"),
			ActiveWriterID:   "host",
			ActiveWriterRole: "host",
			Cols:             cols,
			Rows:             rows,
		}
		if host != nil {
			_ = host.writeJSON(msg)
		}
		for _, viewer := range viewers {
			_ = viewer.writeJSON(msg)
		}
	}
	s.broadcastPresence(session)
}

func authenticatedViewersLocked(session *Session) []*Connection {
	viewers := make([]*Connection, 0, len(session.viewers))
	for _, viewer := range session.viewers {
		if viewer.isAuthenticated() {
			viewers = append(viewers, viewer)
		}
	}
	return viewers
}

func broadcastTerminalSize(viewers []*Connection, cols, rows int) {
	msg := protocol.TerminalSizeMsg{
		Envelope: protocol.NewEnvelope("terminal_size"),
		Cols:     cols,
		Rows:     rows,
	}
	for _, viewer := range viewers {
		_ = viewer.writeJSON(msg)
	}
}
