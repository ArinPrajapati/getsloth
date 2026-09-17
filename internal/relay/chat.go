package relay

import (
	"encoding/json"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// handleChatMessage broadcasts a chat message to every connection in
// the session, including the sender - the Frontend chat panel has no
// optimistic local echo, it only renders a sent message once it arrives
// via this same broadcast, so excluding the sender would mean they
// never see their own message appear in their own chat log.
func (s *Server) handleChatMessage(session *Session, sender *Connection, raw []byte) {
	var msg protocol.ChatMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	session.mu.Lock()
	host := session.host
	viewers := make([]*Connection, 0, len(session.viewers))
	for _, v := range session.viewers {
		if v.isAuthenticated() {
			viewers = append(viewers, v)
		}
	}
	session.mu.Unlock()

	broadcast := protocol.ChatBroadcastMsg{
		Envelope:          protocol.NewEnvelope("chat_message"),
		SenderID:          sender.ID(),
		SenderRole:        sender.Role(),
		SenderDisplayName: sender.displayNameOrEmpty(),
		Text:              msg.Text,
	}

	if host != nil {
		_ = host.writeJSON(broadcast)
	}
	for _, v := range viewers {
		_ = v.writeJSON(broadcast)
	}
}
