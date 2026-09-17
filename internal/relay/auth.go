package relay

import (
	"encoding/json"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// handleViewerMessage decodes one message from a viewer connection and
// acts on it. "auth" and "resume" are handled regardless of auth state;
// everything else requires the connection to already be authenticated,
// per docs/protocol.md's Errors section - UNAUTHORIZED does not close
// the connection, it's a routine, expected outcome for a message sent
// before auth completes.
func (s *Server) handleViewerMessage(session *Session, conn *Connection, remoteAddr string, raw []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}

	switch env.Type {
	case "auth":
		s.handleAuthAttempt(session, conn, remoteAddr, raw)
	case "resume":
		s.handleResume(session, conn, raw)
	default:
		if !conn.isAuthenticated() {
			_ = conn.writeJSON(protocol.ErrorMsg{
				Envelope: protocol.NewEnvelope("error"),
				Code:     protocol.ErrUnauthorized,
				Message:  "not authenticated",
			})
			return
		}
		switch env.Type {
		case "take_control":
			s.handleTakeControl(session, conn)
		case "input":
			s.handleInput(session, conn, raw)
		}
		// chat_message, kill_switch arrive in later tasks.
	}
}

func (s *Server) handleAuthAttempt(session *Session, conn *Connection, remoteAddr string, raw []byte) {
	if blocked, retryAfter := s.rateLimiter.blocked(session.ID, remoteAddr); blocked {
		_ = conn.writeJSON(protocol.AuthResultMsg{
			Envelope:     protocol.NewEnvelope("auth_result"),
			OK:           false,
			Code:         protocol.AuthCodeRateLimited,
			RetryAfterMs: retryAfter.Milliseconds(),
		})
		return
	}

	var msg protocol.AuthMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	requestID, err := newRandomID()
	if err != nil {
		return
	}

	session.mu.Lock()
	host := session.host
	if host == nil || session.closed {
		session.mu.Unlock()
		_ = conn.writeJSON(protocol.AuthResultMsg{
			Envelope: protocol.NewEnvelope("auth_result"),
			OK:       false,
			Code:     protocol.AuthCodeFailed,
		})
		return
	}
	session.pendingAuth[requestID] = pendingAuth{
		viewerID:   conn.ID(),
		conn:       conn,
		remoteAddr: remoteAddr,
	}
	session.mu.Unlock()

	_ = host.writeJSON(protocol.AuthRequestMsg{
		Envelope:           protocol.NewEnvelope("auth_request"),
		RequestID:          requestID,
		ViewerPubkeyBase64: msg.ViewerPubkeyBase64,
		CiphertextBase64:   msg.CiphertextBase64,
	})
}

// handleAuthResponse completes a pending handleAuthAttempt once the host
// replies. Called from the host connection's message loop.
func (s *Server) handleAuthResponse(session *Session, msg protocol.AuthResponseMsg) {
	session.mu.Lock()
	pending, ok := session.pendingAuth[msg.RequestID]
	if ok {
		delete(session.pendingAuth, msg.RequestID)
	}
	session.mu.Unlock()
	if !ok {
		return
	}

	if !msg.OK {
		s.rateLimiter.recordFailure(session.ID, pending.remoteAddr)
		_ = pending.conn.writeJSON(protocol.AuthResultMsg{
			Envelope: protocol.NewEnvelope("auth_result"),
			OK:       false,
			Code:     protocol.AuthCodeFailed,
		})
		return
	}

	s.rateLimiter.recordSuccess(session.ID, pending.remoteAddr)

	token, err := newRandomID()
	if err != nil {
		_ = pending.conn.writeJSON(protocol.AuthResultMsg{
			Envelope: protocol.NewEnvelope("auth_result"),
			OK:       false,
			Code:     protocol.AuthCodeFailed,
		})
		return
	}

	session.mu.Lock()
	session.tokens[token] = pending.viewerID
	session.mu.Unlock()

	pending.conn.setAuthenticated(true)
	_ = pending.conn.writeJSON(protocol.AuthResultMsg{
		Envelope:     protocol.NewEnvelope("auth_result"),
		OK:           true,
		Token:        token,
		ConnectionID: pending.viewerID,
	})
}

// handleResume validates a viewer's resume token and, if valid, makes
// this connection adopt the connection_id the token was originally
// issued under - no host round-trip, since the relay itself is the
// token's issuer and authority (see docs/protocol.md's Reconnect
// section).
func (s *Server) handleResume(session *Session, conn *Connection, raw []byte) {
	var msg protocol.ResumeMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	session.mu.Lock()
	ownerID, ok := session.tokens[msg.Token]
	if ok {
		delete(session.viewers, conn.ID())
		conn.setID(ownerID)
		session.viewers[ownerID] = conn
	}
	session.mu.Unlock()

	if !ok {
		_ = conn.writeJSON(protocol.AuthResultMsg{
			Envelope: protocol.NewEnvelope("auth_result"),
			OK:       false,
			Code:     protocol.AuthCodeFailed,
		})
		return
	}

	conn.setAuthenticated(true)
	_ = conn.writeJSON(protocol.AuthResultMsg{
		Envelope:     protocol.NewEnvelope("auth_result"),
		OK:           true,
		Token:        msg.Token,
		ConnectionID: ownerID,
	})
}
