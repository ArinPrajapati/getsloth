package relay

import (
	"sync"
	"time"

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
type Connection struct {
	ws *websocket.Conn

	mu            sync.Mutex
	id            string
	authenticated bool
}

func newConnection(ws *websocket.Conn, id string) *Connection {
	return &Connection{ws: ws, id: id}
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

func (c *Connection) writeJSON(v any) error {
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
