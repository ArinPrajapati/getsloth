package relay

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// Server is the getsloth relay: it accepts one host WebSocket connection
// per session and any number of viewer connections per session, per
// docs/protocol.md. It deliberately contains no password-comparison or
// auth-decision logic - see CONSTRAINTS.md's architecture rule and the
// depguard entry in .golangci.yml that enforces it.
type Server struct {
	registry    *Registry
	rateLimiter *rateLimiter
	upgrader    websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		registry:    NewRegistry(),
		rateLimiter: newRateLimiter(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// The relay is meant to be reachable from any browser tab
			// holding a valid share link, not restricted to one origin.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/host", s.handleHost)
	mux.HandleFunc("/ws/viewer/", s.handleViewer)
	return mux
}

func (s *Server) handleHost(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn := newConnection(ws, "host")

	session, err := s.registry.Create()
	if err != nil {
		conn.closeWithCode(protocol.CloseBadRequest, "could not create session")
		return
	}

	session.mu.Lock()
	session.host = conn
	session.mu.Unlock()

	if err := conn.writeJSON(protocol.SessionCreatedMsg{
		Envelope:     protocol.NewEnvelope("session_created"),
		SessionID:    session.ID,
		ConnectionID: "host",
	}); err != nil {
		s.registry.remove(session.ID)
		return
	}

	// Block until the host disconnects, dispatching each message as it
	// arrives. take_control, kill_switch, chat_message, end_session
	// arrive in later tasks.
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}
		s.handleHostMessage(session, raw)
	}

	session.teardown(protocol.ReasonHostDisconnected)
	s.registry.remove(session.ID)
}

// handleHostMessage decodes one message from the host connection and
// acts on it.
func (s *Server) handleHostMessage(session *Session, raw []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}

	switch env.Type {
	case "output":
		var msg protocol.OutputMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			return
		}
		session.broadcastOutput(msg.DataBase64)
	case "auth_response":
		var msg protocol.AuthResponseMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			return
		}
		s.handleAuthResponse(session, msg)
	case "take_control":
		session.mu.Lock()
		host := session.host
		session.mu.Unlock()
		if host != nil {
			s.handleTakeControl(session, host)
		}
	}
}

func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/ws/viewer/")
	session, ok := s.registry.Get(sessionID)
	remoteAddr := r.RemoteAddr

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	if !ok {
		conn := newConnection(ws, "")
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrSessionNotFound,
			Message:  "no session with this id",
		})
		conn.closeWithCode(protocol.CloseSessionNotFound, "session_not_found")
		return
	}

	viewerID, err := newRandomID()
	if err != nil {
		newConnection(ws, "").closeWithCode(protocol.CloseBadRequest, "internal error")
		return
	}
	conn := newConnection(ws, viewerID)

	if !session.addViewer(viewerID, conn) {
		_ = conn.writeJSON(protocol.ErrorMsg{
			Envelope: protocol.NewEnvelope("error"),
			Code:     protocol.ErrSessionNotFound,
			Message:  "session has ended",
		})
		conn.closeWithCode(protocol.CloseSessionNotFound, "session_not_found")
		return
	}

	// Block until the viewer disconnects, dispatching each message.
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}
		s.handleViewerMessage(session, conn, remoteAddr, raw)
	}

	session.removeViewer(conn.ID())
}
