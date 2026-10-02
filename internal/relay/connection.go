package relay

import (
	"sync"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// Connection wraps a single WebSocket connection - either the host or a
// viewer - with helpers for sending typed protocol messages and closing
// with a specific application close code per docs/protocol.md's Socket
// lifecycle section.
//
// id and authenticated are mutable: id changes on a successful resume
// (the reconnecting connection adopts the original connection_id the
// token was issued under, see Session.handleResume), and authenticated
// flips true once auth or resume succeeds.
//
// writeMu is separate from mu: many different goroutines write to a
// given Connection concurrently - its own read-loop goroutine (sending
// itself an error/auth_result), and any other connection's read-loop
// goroutine broadcasting to it (output, control_changed, session_ended,
// kicked, ...). gorilla/websocket explicitly does not support
// concurrent callers of its Write* methods on one connection, so every
// writeJSON call must serialize through this lock. (WriteControl and
// Close, used by closeWithCode, are documented by gorilla as safe to
// call concurrently with everything else, so they don't need it.)
type Connection struct {
	ws *websocket.Conn

	role string // "host" or "viewer", fixed at creation, never changes

	mu            sync.Mutex
	id            string
	authenticated bool
	displayName   string
	health        connectionHealth

	writeMu sync.Mutex
}

type connectionHealth struct {
	pendingNonce string
	pendingAt    time.Time
	rttMs        *int64
	quality      string
}

func newConnection(ws *websocket.Conn, id, role string) *Connection {
	return &Connection{
		ws:   ws,
		id:   id,
		role: role,
		health: connectionHealth{
			quality: protocol.ViewerQualityUnknown,
		},
	}
}

// Role returns "host" or "viewer". Unlike id, this never changes for
// the lifetime of the connection (even across a resume, which only
// re-keys id).
func (c *Connection) Role() string {
	return c.role
}

func (c *Connection) setDisplayName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.displayName = name
}

func (c *Connection) displayNameOrEmpty() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.displayName
}

func (c *Connection) ID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

func (c *Connection) setID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id = id
}

func (c *Connection) isAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticated
}

func (c *Connection) setAuthenticated(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authenticated = v
}

func (c *Connection) beginHealthPing(nonce string, sentAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.health.pendingNonce != "" {
		return false
	}
	c.health.pendingNonce = nonce
	c.health.pendingAt = sentAt
	return true
}

func (c *Connection) recordHealthPong(nonce string, receivedAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.health.pendingNonce != nonce {
		return false
	}

	rtt := receivedAt.Sub(c.health.pendingAt)
	if rtt < 0 {
		return false
	}
	rttMs := rtt.Milliseconds()
	c.health.pendingNonce = ""
	c.health.pendingAt = time.Time{}
	c.health.rttMs = &rttMs
	c.health.quality = healthQuality(rtt)
	return true
}

func (c *Connection) expireHealthPing(now time.Time, timeout time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.health.pendingNonce == "" || now.Sub(c.health.pendingAt) < timeout {
		return false
	}
	c.health.pendingNonce = ""
	c.health.pendingAt = time.Time{}
	c.health.rttMs = nil
	c.health.quality = protocol.ViewerQualityStalled
	return true
}

func (c *Connection) healthSnapshot() (*int64, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var rttMs *int64
	if c.health.rttMs != nil {
		value := *c.health.rttMs
		rttMs = &value
	}
	quality := c.health.quality
	if quality == "" {
		quality = protocol.ViewerQualityUnknown
	}
	return rttMs, quality
}

func (c *Connection) writeJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(v)
}

// closeWithCode sends a WebSocket close frame carrying the given
// application close code, then closes the underlying connection. reason
// is a short debugging string, not the JSON message body - callers that
// need to send a typed message (protocol.ErrorMsg, protocol.SessionEndedMsg,
// ...) should writeJSON it first and call closeWithCode after, per the
// "every other relay-initiated close sends the relevant typed message
// first" rule.
func (c *Connection) closeWithCode(code int, reason string) {
	_ = c.ws.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(time.Second),
	)
	_ = c.ws.Close()
}
