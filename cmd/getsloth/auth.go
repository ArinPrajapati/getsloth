package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
)

// runHostMessageLoop reads every message the relay sends to the host
// and dispatches it:
//   - auth_request gets decrypted and answered locally - the only place
//     password comparison happens, per CONSTRAINTS.md's architecture
//     rule.
//   - control_changed updates isActiveWriter, which gates whether the
//     host's own local keystrokes (in pty.go's gatedWriter) currently
//     reach the PTY.
//   - input is a viewer's approved keystrokes, already confirmed by the
//     relay to be from the current active writer - written to the PTY
//     directly, no further gating needed here.
//
// Blocks on ptmxCh until B2's run() has spawned the PTY (via
// onPTYReady), then runs until the connection closes.
func runHostMessageLoop(ws *safeConn, sessionID, password string, keys *hostauth.KeyPair, isActiveWriter *atomic.Bool, ptmxCh <-chan *os.File) {
	ptmx := <-ptmxCh

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}

		switch env.Type {
		case "auth_request":
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

		case "control_changed":
			var msg protocol.ControlChangedMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			isActiveWriter.Store(msg.ActiveWriterRole == "host")

		case "input":
			var msg protocol.InputForwardMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(msg.DataBase64)
			if err != nil {
				continue
			}
			_, _ = ptmx.Write(data)
		}
	}
}
