package main

import (
	"encoding/json"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

// listenForAuthRequests reads messages from the relay in a loop and
// answers any auth_request by decrypting and comparing locally - this
// is the only place password comparison happens, per CONSTRAINTS.md's
// architecture rule. Meant to run in its own goroutine alongside run()'s
// PTY copy loop; returns when the connection closes.
func listenForAuthRequests(ws *websocket.Conn, sessionID, password string, keys *hostauth.KeyPair) {
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		if env.Type != "auth_request" {
			continue
		}

		var req protocol.AuthRequestMsg
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}

		ok := keys.VerifyPassword(sessionID, req.ViewerPubkeyBase64, req.CiphertextBase64, password)
		_ = ws.WriteJSON(protocol.AuthResponseMsg{
			Envelope:  protocol.NewEnvelope("auth_response"),
			RequestID: req.RequestID,
			OK:        ok,
		})
	}
}
