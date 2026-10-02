package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

func relayClientCoverageServer(t *testing.T, handler func(*websocket.Conn)) (string, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handler(ws)
	}))
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	return url, ts.Close
}

func TestConnectHost_ReturnsDialError(t *testing.T) {
	if _, _, err := connectHost("://not-a-websocket-url", testSessionConfig()); err == nil {
		t.Fatal("connectHost returned nil error for malformed relay URL")
	}
}

func TestConnectHost_ReturnsMalformedHandshakeError(t *testing.T) {
	url, closeServer := relayClientCoverageServer(t, func(ws *websocket.Conn) {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("not-json"))
		_ = ws.Close()
	})
	defer closeServer()

	if _, _, err := connectHost(url, testSessionConfig()); err == nil {
		t.Fatal("connectHost accepted a malformed session_created response")
	}
}

func TestConnectHost_ReturnsClosedHandshakeError(t *testing.T) {
	url, closeServer := relayClientCoverageServer(t, func(ws *websocket.Conn) {
		_ = ws.Close()
	})
	defer closeServer()

	if _, _, err := connectHost(url, testSessionConfig()); err == nil {
		t.Fatal("connectHost accepted a relay that closed before session_created")
	}
}

func TestConnectHost_ReturnsWriteErrorAfterRelayCloses(t *testing.T) {
	url, closeServer := relayClientCoverageServer(t, func(ws *websocket.Conn) {
		if err := ws.WriteJSON(protocol.SessionCreatedMsg{
			Envelope:     protocol.NewEnvelope("session_created"),
			SessionID:    "closed-after-created",
			ConnectionID: "host",
		}); err != nil {
			return
		}
		if tcp, ok := ws.UnderlyingConn().(*net.TCPConn); ok {
			_ = tcp.SetLinger(0)
		}
		_ = ws.Close()
	})
	defer closeServer()

	if _, _, err := connectHost(url, testSessionConfig()); err == nil {
		t.Fatal("connectHost returned nil error after relay closed before config write")
	}
}

func TestRelayOutputWriter_SwallowsClosedRelayErrors(t *testing.T) {
	serverReady := make(chan *websocket.Conn, 1)
	url, closeServer := relayClientCoverageServer(t, func(ws *websocket.Conn) {
		serverReady <- ws
	})
	defer closeServer()

	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial relay output fixture: %v", err)
	}
	defer func() { _ = client.Close() }()
	server := <-serverReady
	if tcp, ok := server.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcp.SetLinger(0)
	}
	_ = server.Close()

	w := &relayOutputWriter{ws: &safeConn{ws: client}}
	payload := []byte("local output must continue")
	if n, err := w.Write(payload); n != len(payload) || err != nil {
		t.Fatalf("Write after relay close = (%d, %v), want (%d, nil)", n, err, len(payload))
	}
}
