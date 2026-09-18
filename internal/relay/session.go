package relay

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

// HostLockWindow is the confirmed constant from docs/protocol.md: after
// the host reclaims control, a non-host take_control is rejected for
// this long, so a viewer's request racing in right after can't silently
// undo the reclaim.
const HostLockWindow = 2 * time.Second

// ReconnectWindow is how long a disconnected viewer identity and token remain
// resumable. In Remote mode the single viewer slot is reserved for the same
// interval so another identity cannot steal an interrupted session.
const ReconnectWindow = 30 * time.Second

// Session represents one running getsloth host and its connected
// viewers.
type Session struct {
	ID string

	mu               sync.Mutex
	host             *Connection
	viewers          map[string]*Connection
	closed           bool
	mode             string
	cols             int
	rows             int
	hostCols         int
	hostRows         int
	remoteViewerID   string
	remoteSlotExpiry time.Time
	pendingAuth      map[string]pendingAuth // requestID -> the viewer awaiting a verdict
	tokens           map[string]tokenRecord // relay-issued token -> reconnect identity and expiry
	outputBacklog    []string               // recent base64 output chunks replayed to newly authenticated viewers
	activeWriterID   string                 // starts "host"
	activeWriterRole string                 // starts "host"
	lastHostReclaim  time.Time
}

type tokenRecord struct {
	connectionID string
	expiresAt    time.Time
}

// pendingAuth tracks a viewer's auth attempt while it's awaiting the
// host's verdict, so the relay can route auth_response back to the
// right connection and record the outcome against the right rate-limit
// bucket.
type pendingAuth struct {
	viewerID   string
	conn       *Connection
	remoteAddr string
}

const maxOutputBacklogChunks = 256

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

func (s *Session) configure(mode string, cols, rows int) bool {
	if mode != protocol.SessionModeRemote && mode != protocol.SessionModeGroup {
		return false
	}
	if !validTerminalSize(cols, rows) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	s.cols = cols
	s.rows = rows
	s.hostCols = cols
	s.hostRows = rows
	s.outputBacklog = nil
	return true
}

func validTerminalSize(cols, rows int) bool {
	return cols >= 2 && rows >= 2 && cols <= 1000 && rows <= 500
}

func (s *Session) authResult(token, connectionID string) protocol.AuthResultMsg {
	return protocol.AuthResultMsg{
		Envelope:         protocol.NewEnvelope("auth_result"),
		OK:               true,
		Token:            token,
		ConnectionID:     connectionID,
		Mode:             s.mode,
		Cols:             s.cols,
		Rows:             s.rows,
		ActiveWriterID:   s.activeWriterID,
		ActiveWriterRole: s.activeWriterRole,
	}
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
	s.outputBacklog = append(s.outputBacklog, dataBase64)
	if len(s.outputBacklog) > maxOutputBacklogChunks {
		s.outputBacklog = s.outputBacklog[len(s.outputBacklog)-maxOutputBacklogChunks:]
	}
	viewers := make([]*Connection, 0, len(s.viewers))
	for _, c := range s.viewers {
		if c.isAuthenticated() {
			viewers = append(viewers, c)
		}
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

func (s *Session) replayOutputTo(conn *Connection) {
	s.mu.Lock()
	backlog := append([]string(nil), s.outputBacklog...)
	s.mu.Unlock()

	for _, dataBase64 := range backlog {
		_ = conn.writeJSON(protocol.OutputMsg{
			Envelope:   protocol.NewEnvelope("output"),
			DataBase64: dataBase64,
		})
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
	s := &Session{
		ID:               id,
		viewers:          map[string]*Connection{},
		pendingAuth:      map[string]pendingAuth{},
		tokens:           map[string]tokenRecord{},
		mode:             protocol.SessionModeRemote,
		cols:             protocol.DefaultCols,
		rows:             protocol.DefaultRows,
		hostCols:         protocol.DefaultCols,
		hostRows:         protocol.DefaultRows,
		activeWriterID:   "host",
		activeWriterRole: "host",
	}
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
