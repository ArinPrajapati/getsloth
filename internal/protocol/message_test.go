package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewEnvelopeUsesCurrentProtocolVersion(t *testing.T) {
	envelope := NewEnvelope("ping")
	if envelope.V != ProtocolVersion || envelope.Type != "ping" {
		t.Fatalf("envelope = %+v, want v%d ping", envelope, ProtocolVersion)
	}
}

func TestHealthMessagesUseNonceWireShape(t *testing.T) {
	ping, err := json.Marshal(PingMsg{Envelope: NewEnvelope("ping"), Nonce: "nonce-1"})
	if err != nil {
		t.Fatalf("marshal ping: %v", err)
	}
	if string(ping) != `{"v":1,"type":"ping","nonce":"nonce-1"}` {
		t.Fatalf("ping JSON = %s", ping)
	}

	var pong PongMsg
	if err := json.Unmarshal([]byte(`{"v":1,"type":"pong","nonce":"nonce-1"}`), &pong); err != nil {
		t.Fatalf("unmarshal pong: %v", err)
	}
	if pong.V != ProtocolVersion || pong.Type != "pong" || pong.Nonce != "nonce-1" {
		t.Fatalf("pong = %+v", pong)
	}
}

func TestPresenceHealthFieldsAreOptionalUntilMeasured(t *testing.T) {
	withoutHealth, err := json.Marshal(PresenceConnectionInfo{
		ID:             "viewer-1",
		Role:           "viewer",
		IsActiveWriter: false,
	})
	if err != nil {
		t.Fatalf("marshal unmeasured presence: %v", err)
	}
	if strings.Contains(string(withoutHealth), "rtt_ms") || strings.Contains(string(withoutHealth), "quality") {
		t.Fatalf("unmeasured presence exposed health fields: %s", withoutHealth)
	}

	rttMs := int64(42)
	withHealth, err := json.Marshal(PresenceConnectionInfo{
		ID:             "viewer-1",
		Role:           "viewer",
		IsActiveWriter: false,
		RTTMs:          &rttMs,
		Quality:        ViewerQualityGood,
	})
	if err != nil {
		t.Fatalf("marshal measured presence: %v", err)
	}
	if !strings.Contains(string(withHealth), `"rtt_ms":42`) || !strings.Contains(string(withHealth), `"quality":"good"`) {
		t.Fatalf("measured presence omitted health fields: %s", withHealth)
	}
}
