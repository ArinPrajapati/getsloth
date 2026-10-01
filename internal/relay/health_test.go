package relay

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/gorilla/websocket"
)

func newHealthTestServer(t *testing.T) (string, *Server, func()) {
	t.Helper()
	server := NewServer()
	server.healthInterval = 20 * time.Millisecond
	server.healthTimeout = 60 * time.Millisecond
	ts := httptest.NewServer(server.Handler())
	return "ws" + strings.TrimPrefix(ts.URL, "http"), server, ts.Close
}

func TestHealthQuality_UsesDocumentedRTTThresholds(t *testing.T) {
	for _, test := range []struct {
		name string
		rtt  time.Duration
		want string
	}{
		{name: "good upper bound", rtt: 150 * time.Millisecond, want: protocol.ViewerQualityGood},
		{name: "laggy lower bound", rtt: 151 * time.Millisecond, want: protocol.ViewerQualityLaggy},
		{name: "laggy upper bound", rtt: 500 * time.Millisecond, want: protocol.ViewerQualityLaggy},
		{name: "stalled over limit", rtt: 501 * time.Millisecond, want: protocol.ViewerQualityStalled},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := healthQuality(test.rtt); got != test.want {
				t.Fatalf("healthQuality(%s) = %q, want %q", test.rtt, got, test.want)
			}
		})
	}
}

func readNextPing(t *testing.T, viewer *websocket.Conn) protocol.PingMsg {
	t.Helper()
	for {
		_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, raw, err := viewer.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage waiting for ping: %v", err)
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal health message: %v", err)
		}
		if envelope.Type == "presence" {
			continue
		}
		if envelope.Type != "ping" {
			t.Fatalf("message while waiting for ping = %q", envelope.Type)
		}
		var ping protocol.PingMsg
		if err := json.Unmarshal(raw, &ping); err != nil {
			t.Fatalf("unmarshal ping: %v", err)
		}
		return ping
	}
}

func readHealthPresence(t *testing.T, hs *hostStub, viewerID string, quality string, wantRTT bool) protocol.PresenceConnectionInfo {
	t.Helper()
	for {
		var presence protocol.PresenceMsg
		hs.next(t, &presence)
		for _, connection := range presence.Connections {
			if (viewerID != "" && connection.ID != viewerID) || (viewerID == "" && connection.Role != "viewer") || connection.Quality != quality {
				continue
			}
			if (connection.RTTMs != nil) != wantRTT {
				t.Fatalf("health presence = %+v, want RTT present=%t", connection, wantRTT)
			}
			return connection
		}
	}
}

func TestViewerHealth_ReportsRelayMeasuredRTTAndQuality(t *testing.T) {
	base, _, cleanup := newHealthTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	viewer := authedViewer(t, base, created.SessionID, hs)

	ping := readNextPing(t, viewer)
	if ping.Nonce == "" {
		t.Fatal("health ping nonce is empty")
	}
	if err := viewer.WriteJSON(protocol.PongMsg{
		Envelope: protocol.NewEnvelope("pong"),
		Nonce:    ping.Nonce,
	}); err != nil {
		t.Fatalf("sending pong: %v", err)
	}

	entry := readHealthPresence(t, hs, "", protocol.ViewerQualityGood, true)
	if entry.RTTMs == nil || *entry.RTTMs < 0 {
		t.Fatalf("relay RTT = %v, want a non-negative measurement", entry.RTTMs)
	}
}

func TestViewerHealth_DoesNotProbeBeforeAuthentication(t *testing.T) {
	base, _, cleanup := newHealthTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	viewer := dial(t, base+"/ws/viewer/"+created.SessionID)

	_ = viewer.SetReadDeadline(time.Now().Add(120 * time.Millisecond))
	if _, _, err := viewer.ReadMessage(); err == nil {
		t.Fatal("unauthenticated viewer received a health message")
	}
}

func TestViewerHealth_ClearsStaleRTTWhenPongTimesOut(t *testing.T) {
	base, _, cleanup := newHealthTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	viewer := authedViewer(t, base, created.SessionID, hs)

	firstPing := readNextPing(t, viewer)
	if err := viewer.WriteJSON(protocol.PongMsg{
		Envelope: protocol.NewEnvelope("pong"),
		Nonce:    firstPing.Nonce,
	}); err != nil {
		t.Fatalf("sending first pong: %v", err)
	}
	_ = readHealthPresence(t, hs, "", protocol.ViewerQualityGood, true)

	_ = readNextPing(t, viewer)
	stalled := readHealthPresence(t, hs, "", protocol.ViewerQualityStalled, false)
	if stalled.RTTMs != nil {
		t.Fatalf("stalled health retained stale RTT %dms", *stalled.RTTMs)
	}
}

func TestViewerHealth_IgnoresPongWithWrongNonce(t *testing.T) {
	base, _, cleanup := newHealthTestServer(t)
	defer cleanup()

	host := dial(t, base+"/ws/host")
	var created protocol.SessionCreatedMsg
	readMsg(t, host, &created)
	hs := newHostStub(t, host, true)
	viewer := authedViewer(t, base, created.SessionID, hs)

	ping := readNextPing(t, viewer)
	if err := viewer.WriteJSON(protocol.PongMsg{
		Envelope: protocol.NewEnvelope("pong"),
		Nonce:    ping.Nonce + "-wrong",
	}); err != nil {
		t.Fatalf("sending wrong pong: %v", err)
	}
	select {
	case raw := <-hs.other:
		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal host message: %v", err)
		}
		if envelope.Type == "presence" {
			t.Fatalf("wrong nonce produced a health presence: %s", raw)
		}
	case <-time.After(30 * time.Millisecond):
	}
}
