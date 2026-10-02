package relay

import (
	"encoding/json"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

const (
	healthPingInterval = 5 * time.Second
	healthPingTimeout  = 15 * time.Second
	healthGoodLimit    = 150 * time.Millisecond
	healthLaggyLimit   = 500 * time.Millisecond
)

func healthQuality(rtt time.Duration) string {
	switch {
	case rtt <= healthGoodLimit:
		return protocol.ViewerQualityGood
	case rtt <= healthLaggyLimit:
		return protocol.ViewerQualityLaggy
	default:
		return protocol.ViewerQualityStalled
	}
}

func (s *Server) runViewerHealth(session *Session) {
	interval := s.healthInterval
	if interval <= 0 {
		interval = healthPingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkViewerHealth(session)
		case <-session.done:
			return
		}
	}
}

func (s *Server) checkViewerHealth(session *Session) {
	now := time.Now()
	timeout := s.healthTimeout
	if timeout <= 0 {
		timeout = healthPingTimeout
	}

	type pingTarget struct {
		conn  *Connection
		nonce string
	}

	targets := make([]pingTarget, 0)
	healthChanged := false
	session.mu.Lock()
	for _, viewer := range session.viewers {
		if !viewer.isAuthenticated() {
			continue
		}
		if viewer.expireHealthPing(now, timeout) {
			healthChanged = true
		}

		nonce, err := newRandomID()
		if err != nil || !viewer.beginHealthPing(nonce, now) {
			continue
		}
		targets = append(targets, pingTarget{conn: viewer, nonce: nonce})
	}
	session.mu.Unlock()

	for _, target := range targets {
		_ = target.conn.writeJSON(protocol.PingMsg{
			Envelope: protocol.NewEnvelope("ping"),
			Nonce:    target.nonce,
		})
	}
	if healthChanged {
		s.broadcastPresence(session)
	}
}

func (s *Server) handlePong(session *Session, conn *Connection, raw []byte) {
	var msg protocol.PongMsg
	if err := json.Unmarshal(raw, &msg); err != nil || msg.Nonce == "" {
		return
	}
	if conn.recordHealthPong(msg.Nonce, time.Now()) {
		s.broadcastPresence(session)
	}
}
