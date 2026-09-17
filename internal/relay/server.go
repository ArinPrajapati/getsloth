package relay

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// Server is the getsloth relay: it accepts one host WebSocket connection
// per session and any number of viewer connections per session, per
// docs/protocol.md. It deliberately contains no password-comparison or
// auth-decision logic - see CONSTRAINTS.md's architecture rule and the
// depguard entry in .golangci.yml that enforces it.
type Server struct {
	registry *Registry
	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		registry: NewRegistry(),
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
	conn := newConnection(ws)

	session, err := s.registry.Create()
	if err != nil {
		conn.closeWithCode(CloseBadRequest, "could not create session")
		return
	}

	session.mu.Lock()
	session.host = conn
	session.mu.Unlock()

	if err := conn.writeJSON(SessionCreatedMsg{
		Envelope:     newEnvelope("session_created"),
		SessionID:    session.ID,
		ConnectionID: "host",
	}); err != nil {
		s.registry.remove(session.ID)
		return
	}

	// Block until the host disconnects. Message handling (auth_response,
	// output, take_control, ...) arrives in later tasks - B3 only needs
	// to detect the disconnect and tear the session down.
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}

	session.teardown(ReasonHostDisconnected)
	s.registry.remove(session.ID)
}

func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/ws/viewer/")
	session, ok := s.registry.Get(sessionID)

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn := newConnection(ws)

	if !ok {
		_ = conn.writeJSON(ErrorMsg{
			Envelope: newEnvelope("error"),
			Code:     ErrSessionNotFound,
			Message:  "no session with this id",
		})
		conn.closeWithCode(CloseSessionNotFound, "session_not_found")
		return
	}

	viewerID, err := newRandomID()
	if err != nil {
		conn.closeWithCode(CloseBadRequest, "internal error")
		return
	}

	if !session.addViewer(viewerID, conn) {
		_ = conn.writeJSON(ErrorMsg{
			Envelope: newEnvelope("error"),
			Code:     ErrSessionNotFound,
			Message:  "session has ended",
		})
		conn.closeWithCode(CloseSessionNotFound, "session_not_found")
		return
	}

	// Block until the viewer disconnects. Auth, output, chat, etc.
	// arrive in B5+.
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}

	session.removeViewer(viewerID)
}
