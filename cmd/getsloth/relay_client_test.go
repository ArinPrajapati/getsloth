package main

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/arinprajapati/getsloth/internal/relay"
	"github.com/gorilla/websocket"
)

func TestConnectHost_ReceivesSessionCreated(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base)
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	if created.SessionID == "" {
		t.Error("session_id is empty")
	}
}

func TestRelayOutputWriter_ChunksLargeWritesAndReachesViewer(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base)
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}
	const password = "chunk-test-password"
	go listenForAuthRequests(ws, created.SessionID, password, keys)

	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+created.SessionID, nil)
	if err != nil {
		t.Fatalf("dialing viewer: %v", err)
	}
	defer func() { _ = viewer.Close() }()

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), created.SessionID, password)
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
	}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}
	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authResult protocol.AuthResultMsg
	if err := viewer.ReadJSON(&authResult); err != nil {
		t.Fatalf("ReadJSON auth_result: %v", err)
	}
	if !authResult.OK {
		t.Fatalf("authentication failed, can't test output chunking")
	}

	w := &relayOutputWriter{ws: ws}

	// Deliberately bigger than protocol.MaxOutputChunkBytes to prove the
	// writer actually splits it rather than sending one oversized frame.
	payload := make([]byte, protocol.MaxOutputChunkBytes+10)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}

	n, err := w.Write(payload)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(payload) {
		t.Errorf("Write returned n=%d, want %d", n, len(payload))
	}

	var received []byte
	for len(received) < len(payload) {
		_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg protocol.OutputMsg
		if err := viewer.ReadJSON(&msg); err != nil {
			t.Fatalf("ReadJSON: %v", err)
		}
		chunk, err := base64.StdEncoding.DecodeString(msg.DataBase64)
		if err != nil {
			t.Fatalf("decoding chunk: %v", err)
		}
		if len(chunk) > protocol.MaxOutputChunkBytes {
			t.Errorf("chunk of %d bytes exceeds MaxOutputChunkBytes (%d)", len(chunk), protocol.MaxOutputChunkBytes)
		}
		received = append(received, chunk...)
	}

	if string(received) != string(payload) {
		t.Error("reassembled output does not match what was written")
	}
}
