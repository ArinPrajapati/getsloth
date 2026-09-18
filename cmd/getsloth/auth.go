package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/creack/pty"
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
//   - chat_message is printed to chatOut - without this, a viewer's
//     "add context without taking control" message (the whole reason
//     chat exists, per docs/ideas/getsloth.md) would silently vanish:
//     the host sitting at their real terminal would never see it.
//
// Blocks on ptmxCh until B2's run() has spawned the PTY (via
// onPTYReady), then runs until the connection closes.
func runHostMessageLoop(ws *safeConn, sessionID, password string, keys *hostauth.KeyPair, isActiveWriter *atomic.Bool, ptmxCh <-chan *os.File, chatOut io.Writer, status *hostSessionStatus) {
	ptmx := <-ptmxCh
	if status != nil {
		defer status.setDisconnected()
	}

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
			if validGridSize(msg.Cols, msg.Rows) {
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Rows: uint16(msg.Rows),
					Cols: uint16(msg.Cols),
				})
			}
			isActiveWriter.Store(msg.ActiveWriterRole == "host")
			if status != nil {
				status.updateControl(msg)
			}

		case "presence":
			var msg protocol.PresenceMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if status != nil {
				status.updatePresence(msg)
			}

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

		case "resize":
			var msg protocol.ResizeMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if msg.Cols < 2 || msg.Rows < 2 || msg.Cols > 1000 || msg.Rows > 500 {
				continue
			}
			_ = pty.Setsize(ptmx, &pty.Winsize{
				Rows: uint16(msg.Rows),
				Cols: uint16(msg.Cols),
			})

		case "chat_message":
			var msg protocol.ChatBroadcastMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			sender := msg.SenderDisplayName
			if sender == "" {
				if msg.SenderRole == "host" {
					sender = "you"
				} else {
					sender = "viewer"
				}
			}
			_, _ = fmt.Fprintf(chatOut, "getsloth: [chat] %s: %s\n", sender, msg.Text)
		}
	}
}
