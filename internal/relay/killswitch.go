package relay

import (
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// handleKillSwitch disconnects every currently connected viewer and
// invalidates every issued token, but leaves the session (and the host
// connection) untouched - the wrapped process keeps running, and the
// host can set a new password and reshare, per docs/protocol.md's
// Abuse/safety guardrails.
func (s *Server) handleKillSwitch(session *Session) {
	session.mu.Lock()
	viewers := make([]*Connection, 0, len(session.viewers))
	for _, v := range session.viewers {
		viewers = append(viewers, v)
	}
	session.viewers = map[string]*Connection{}
	session.tokens = map[string]tokenRecord{}
	session.remoteViewerID = ""
	session.remoteSlotExpiry = time.Time{}

	// Any auth attempt still awaiting a host verdict at this moment
	// belongs to a connection about to be kicked - clear it now rather
	// than waiting for the host to eventually respond to a viewer that
	// no longer exists. Without this, the rate-limit reservation
	// handleAuthAttempt made for it would stay held until the host
	// happens to respond (or the whole session tears down), needlessly
	// eating into that address's MaxConcurrentAuthAttempts budget.
	pending := session.pendingAuth
	session.pendingAuth = map[string]pendingAuth{}
	session.mu.Unlock()

	for _, p := range pending {
		s.rateLimiter.release(session.ID, p.remoteAddr)
	}

	for _, v := range viewers {
		_ = v.writeJSON(protocol.KickedMsg{
			Envelope: protocol.NewEnvelope("kicked"),
			Reason:   protocol.ReasonKillSwitch,
		})
		v.closeWithCode(protocol.CloseKicked, "kicked")
	}

	// session.viewers is already empty at this point, so this only
	// reaches the host - correct, since the just-kicked viewers no
	// longer have a connection to receive it on.
	s.broadcastPresence(session)
}
