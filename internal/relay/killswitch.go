package relay

import "github.com/arinprajapati/getsloth/internal/protocol"

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
	session.tokens = map[string]string{}
	session.mu.Unlock()

	for _, v := range viewers {
		_ = v.writeJSON(protocol.KickedMsg{
			Envelope: protocol.NewEnvelope("kicked"),
			Reason:   protocol.ReasonKillSwitch,
		})
		v.closeWithCode(protocol.CloseKicked, "kicked")
	}
}
