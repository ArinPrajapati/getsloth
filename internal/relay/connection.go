package relay

import (
	"time"

	"github.com/gorilla/websocket"
)

// Connection wraps a single WebSocket connection - either the host or a
// viewer - with helpers for sending typed protocol messages and closing
// with a specific application close code per docs/protocol.md's Socket
// lifecycle section.
type Connection struct {
	ws *websocket.Conn
}

func newConnection(ws *websocket.Conn) *Connection {
	return &Connection{ws: ws}
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
