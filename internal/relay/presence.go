package relay

import "github.com/arinprajapati/getsloth/internal/protocol"

// broadcastPresence sends the current roster (host + every authenticated
// viewer, each tagged with whether they're the active writer) to every
// connection in the session. Called whenever the roster or active-writer
// state changes: a viewer authenticates or resumes, disconnects, the
// kill switch fires, or control changes hands.
func (s *Server) broadcastPresence(session *Session) {
	session.mu.Lock()
	host := session.host
	activeWriterID := session.activeWriterID

	var connections []protocol.PresenceConnectionInfo
	if host != nil {
		connections = append(connections, protocol.PresenceConnectionInfo{
			ID:             "host",
			Role:           "host",
			IsActiveWriter: activeWriterID == "host",
		})
	}
	viewers := make([]*Connection, 0, len(session.viewers))
	for _, v := range session.viewers {
		if v.isAuthenticated() {
			viewers = append(viewers, v)
		}
	}
	session.mu.Unlock()

	for _, v := range viewers {
		connections = append(connections, protocol.PresenceConnectionInfo{
			ID:             v.ID(),
			Role:           "viewer",
			DisplayName:    v.displayNameOrEmpty(),
			IsActiveWriter: v.ID() == activeWriterID,
		})
	}

	msg := protocol.PresenceMsg{
		Envelope:    protocol.NewEnvelope("presence"),
		Connections: connections,
	}

	if host != nil {
		_ = host.writeJSON(msg)
	}
	for _, v := range viewers {
		_ = v.writeJSON(msg)
	}
}
