package main

import (
	"encoding/base64"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// connectHost dials the relay's host endpoint and waits for the
// session_created response.
func connectHost(relayURL string) (*websocket.Conn, protocol.SessionCreatedMsg, error) {
	ws, _, err := websocket.DefaultDialer.Dial(relayURL+"/ws/host", nil)
	if err != nil {
		return nil, protocol.SessionCreatedMsg{}, err
	}

	var created protocol.SessionCreatedMsg
	if err := ws.ReadJSON(&created); err != nil {
		_ = ws.Close()
		return nil, protocol.SessionCreatedMsg{}, err
	}

	return ws, created, nil
}

// relayOutputWriter is an io.Writer that forwards each write to the
// relay as one or more "output" messages, chunked to stay within
// protocol.MaxOutputChunkBytes decoded bytes per message (see
// docs/protocol.md's Limits table). Composed with os.Stdout via
// io.MultiWriter so the host's own local terminal experience is
// unaffected by network hiccups on this side.
type relayOutputWriter struct {
	ws *websocket.Conn
}

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
		if err := w.ws.WriteJSON(msg); err != nil {
			return total - len(p), err
		}
		p = p[len(chunk):]
	}
	return total, nil
}
