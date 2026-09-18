package main

import (
	"encoding/base64"
	"sync"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// safeConn wraps the host's single WebSocket connection to the relay
// with a write mutex. Three independent goroutines write to this
// connection over its lifetime - the PTY output copy loop
// (relayOutputWriter), runHostMessageLoop (auth_response), and the
// SIGUSR1/SIGUSR2 signal handler (take_control/kill_switch/end_session)
// - and gorilla/websocket explicitly does not support concurrent
// writers on one connection: "Applications are responsible for
// ensuring that no more than one goroutine calls the write methods
// concurrently." Only one goroutine ever reads (runHostMessageLoop), so
// reads aren't guarded here.
type safeConn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *safeConn) WriteJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(v)
}

func (c *safeConn) ReadMessage() (messageType int, p []byte, err error) {
	return c.ws.ReadMessage()
}

func (c *safeConn) Close() error {
	return c.ws.Close()
}

// connectHost dials the relay's host endpoint and waits for the
// session_created response.
func connectHost(relayURL string, config protocol.SessionConfigMsg) (*safeConn, protocol.SessionCreatedMsg, error) {
	ws, _, err := websocket.DefaultDialer.Dial(relayURL+"/ws/host", nil)
	if err != nil {
		return nil, protocol.SessionCreatedMsg{}, err
	}

	conn := &safeConn{ws: ws}
	var created protocol.SessionCreatedMsg
	if err := ws.ReadJSON(&created); err != nil {
		_ = conn.Close()
		return nil, protocol.SessionCreatedMsg{}, err
	}
	if err := conn.WriteJSON(config); err != nil {
		_ = conn.Close()
		return nil, protocol.SessionCreatedMsg{}, err
	}

	return conn, created, nil
}

// relayOutputWriter is an io.Writer that forwards each write to the
// relay as one or more "output" messages, chunked to stay within
// protocol.MaxOutputChunkBytes decoded bytes per message (see
// docs/protocol.md's Limits table). Composed with os.Stdout via
// io.MultiWriter so the host's own local terminal experience is
// unaffected by network hiccups on this side.
type relayOutputWriter struct {
	ws *safeConn
}

// Write always returns (len(p), nil), never propagating a relay-side
// failure: io.MultiWriter aborts on its first writer's error, and
// io.Copy aborts its whole loop on any destination error - a
// transient network hiccup here must not silently freeze the host's
// own local terminal output, which is exactly what happened before this
// was fixed (io.Copy(stdout, ptmx) would stop entirely on the first
// WriteJSON failure, even though os.Stdout itself was fine). Matches
// the same swallow-and-keep-going pattern gatedWriter already used for
// dropped input.
func (w *relayOutputWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		chunk := p
		if len(chunk) > protocol.MaxOutputChunkBytes {
			chunk = chunk[:protocol.MaxOutputChunkBytes]
		}
		msg := protocol.OutputMsg{
			Envelope:   protocol.NewEnvelope("output"),
			DataBase64: base64.StdEncoding.EncodeToString(chunk),
		}
		_ = w.ws.WriteJSON(msg)
		p = p[len(chunk):]
	}
	return total, nil
}
