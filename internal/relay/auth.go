package relay

import (
	"encoding/json"
	"time"

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
			var msg protocol.TakeControlMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				return
			}
			s.handleTakeControl(session, conn, msg)
		case "input":
			s.handleInput(session, conn, raw)
		case "resize":
			s.handleResize(session, conn, raw)
		case "chat_message":
			s.handleChatMessage(session, conn, raw)
		}
		// kill_switch is host-only, dispatched from handleHostMessage.
	}
}

func (s *Server) handleAuthAttempt(session *Session, conn *Connection, remoteAddr string, raw []byte) {
	// Re-authenticating an already-authenticated connection is wasted
	// host decrypt work for no protocol benefit - ignore it rather than
	// process it.
	if conn.isAuthenticated() {
		return
	}

	// tryReserve enforces two independent things at once: the cooldown
	// from repeated completed failures (what "blocked" used to check
	// alone), and a cap on concurrent in-flight attempts. The cooldown
	// alone doesn't catch a flood of auth messages sent before any of
	// them have had time to fail yet - none of those are "blocked" by a
	// failure count that hasn't been incremented, since recordFailure
	// only fires once a response comes back. See docs/protocol.md's
	// Rate limiting section for the confirmed per-completed-attempt
	// cooldown; MaxConcurrentAuthAttempts is this relay's own addition
	// to close that gap, not a protocol-specified value.
	reserved, retryAfter := s.rateLimiter.tryReserve(session.ID, remoteAddr)
	if !reserved {
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
		s.rateLimiter.release(session.ID, remoteAddr)
		return
	}
	// Set regardless of whether auth ultimately succeeds - harmless if
	// it fails, since a never-authenticated connection's display name
	// is never broadcast to anyone.
	if msg.DisplayName != "" {
		conn.setDisplayName(msg.DisplayName)
	}

	requestID, err := newRandomID()
	if err != nil {
		s.rateLimiter.release(session.ID, remoteAddr)
		return
	}

	session.mu.Lock()
	host := session.host
	if host == nil || session.closed {
		session.mu.Unlock()
		s.rateLimiter.release(session.ID, remoteAddr)
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
	// The reservation made above is released in handleAuthResponse,
	// once this attempt actually resolves - not here.
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
	// Releases the reservation handleAuthAttempt made, on every exit
	// path from here - success, failure, or the token-generation error
	// path below.
	defer s.rateLimiter.release(session.ID, pending.remoteAddr)

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
	if session.mode == protocol.SessionModeRemote && session.remoteViewerID != "" && session.remoteViewerID != pending.viewerID {
		if session.remoteSlotExpiry.IsZero() || time.Now().Before(session.remoteSlotExpiry) {
			session.mu.Unlock()
			_ = pending.conn.writeJSON(protocol.AuthResultMsg{
				Envelope: protocol.NewEnvelope("auth_result"),
				OK:       false,
				Code:     protocol.AuthCodeOccupied,
			})
			return
		}
		session.remoteViewerID = ""
	}
	if session.mode == protocol.SessionModeRemote {
		session.remoteViewerID = pending.viewerID
		session.remoteSlotExpiry = time.Time{}
	}
	session.tokens[token] = tokenRecord{connectionID: pending.viewerID}
	result := session.authResult(token, pending.viewerID)
	session.mu.Unlock()

	pending.conn.setAuthenticated(true)
	_ = pending.conn.writeJSON(result)
	session.replayOutputTo(pending.conn)
	s.broadcastPresence(session)
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
	record, ok := session.tokens[msg.Token]
	if ok && !record.expiresAt.IsZero() && time.Now().After(record.expiresAt) {
		delete(session.tokens, msg.Token)
		if session.remoteViewerID == record.connectionID {
			session.remoteViewerID = ""
			session.remoteSlotExpiry = time.Time{}
		}
		ok = false
	}
	if ok {
		delete(session.viewers, conn.ID())
		conn.setID(record.connectionID)
		session.viewers[record.connectionID] = conn
		record.expiresAt = time.Time{}
		session.tokens[msg.Token] = record
		if session.mode == protocol.SessionModeRemote {
			session.remoteViewerID = record.connectionID
			session.remoteSlotExpiry = time.Time{}
		}
	}
	result := session.authResult(msg.Token, record.connectionID)
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
	_ = conn.writeJSON(result)
	session.replayOutputTo(conn)
	s.broadcastPresence(session)
}
