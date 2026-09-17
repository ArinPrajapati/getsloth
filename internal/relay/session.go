package relay

import (
	"crypto/rand"
	"encoding/base64"
	"sync"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// Session represents one running getsloth host and its connected
// viewers. It does not route input/output yet - that starts in Task B4.
// B3 covers connection lifecycle only: creation, viewer join/reject, and
// teardown when the host disconnects.
type Session struct {
	ID string

	mu      sync.Mutex
	host    *Connection
	viewers map[string]*Connection
	closed  bool
}

// newRandomID generates a URL-safe random identifier - used both for
// session IDs and, for now, viewer connection IDs (B5's auth flow may
// later assign connection IDs at a point where more context is
// available; this is the general-purpose generator either can use).
func newRandomID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// addViewer registers a viewer connection. Returns false if the session
// has already torn down (host disconnected) - the caller should reject
// the viewer rather than register a connection to a dead session.
func (s *Session) addViewer(id string, c *Connection) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.viewers[id] = c
	return true
}

func (s *Session) removeViewer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.viewers, id)
}

// teardown notifies every connected viewer the session has ended, closes
// their connections, and marks the session closed so any viewer racing
// to join afterward is rejected instead of registered.
func (s *Session) teardown(reason string) {
	s.mu.Lock()
	viewers := make([]*Connection, 0, len(s.viewers))
	for _, c := range s.viewers {
		viewers = append(viewers, c)
	}
	s.viewers = map[string]*Connection{}
	s.closed = true
	s.mu.Unlock()

	for _, c := range viewers {
		_ = c.writeJSON(protocol.SessionEndedMsg{
			Envelope: protocol.NewEnvelope("session_ended"),
			Reason:   reason,
		})
		c.closeWithCode(protocol.CloseSessionEnded, "session_ended")
	}
}

// broadcastOutput forwards a chunk of host output to every currently
// connected viewer - and only viewers, never back to the host, which
// already has this output locally from its own PTY (see
// docs/protocol.md's "Relay -> viewers only" section).
func (s *Session) broadcastOutput(dataBase64 string) {
	s.mu.Lock()
	viewers := make([]*Connection, 0, len(s.viewers))
	for _, c := range s.viewers {
		viewers = append(viewers, c)
	}
	s.mu.Unlock()

	msg := protocol.OutputMsg{
		Envelope:   protocol.NewEnvelope("output"),
		DataBase64: dataBase64,
	}
	for _, c := range viewers {
		_ = c.writeJSON(msg)
	}
}

// Registry tracks all live sessions by ID.
type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewRegistry() *Registry {
	return &Registry{sessions: map[string]*Session{}}
}

// Create makes a new session with a fresh random ID and registers it.
// Every call creates a distinct session - the relay never merges or
// dedups host connections, so "one session per running getsloth
// process" is an invariant the CLI upholds (it only ever opens one host
// connection per run), not something the relay detects.
func (r *Registry) Create() (*Session, error) {
	id, err := newRandomID()
	if err != nil {
		return nil, err
	}
	s := &Session{ID: id, viewers: map[string]*Connection{}}
	r.mu.Lock()
	r.sessions[id] = s
	r.mu.Unlock()
	return s, nil
}

func (r *Registry) Get(id string) (*Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	return s, ok
}

func (r *Registry) remove(id string) {
	r.mu.Lock()
	delete(r.sessions, id)
	r.mu.Unlock()
}
