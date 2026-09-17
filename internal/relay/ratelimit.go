package relay

import (
	"sync"
	"time"
)

// RateLimitMaxAttempts and RateLimitCooldown are the confirmed
// constants from docs/protocol.md.
const (
	RateLimitMaxAttempts = 5
	RateLimitCooldown    = 60 * time.Second
)

// rateLimiter tracks failed auth attempts per (sessionID, remoteAddr),
// per docs/protocol.md's Rate limiting section - keyed on more than the
// ephemeral WebSocket connection so reconnecting doesn't reset the
// counter.
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
}

type rateEntry struct {
	fails        int
	blockedUntil time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{entries: map[string]*rateEntry{}}
}

func rateLimitKey(sessionID, remoteAddr string) string {
	return sessionID + "|" + remoteAddr
}

// blocked reports whether this (session, address) pair is currently
// rate-limited, and if so, how long until it isn't.
func (r *rateLimiter) blocked(sessionID, remoteAddr string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[rateLimitKey(sessionID, remoteAddr)]
	if !ok {
		return false, 0
	}
	if remaining := time.Until(e.blockedUntil); remaining > 0 {
		return true, remaining
	}
	return false, 0
}

// recordFailure increments the failure count and starts a cooldown once
// RateLimitMaxAttempts is reached.
func (r *rateLimiter) recordFailure(sessionID, remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := rateLimitKey(sessionID, remoteAddr)
	e, ok := r.entries[k]
	if !ok {
		e = &rateEntry{}
		r.entries[k] = e
	}
	e.fails++
	if e.fails >= RateLimitMaxAttempts {
		e.blockedUntil = time.Now().Add(RateLimitCooldown)
	}
}

// recordSuccess clears any tracked failures for this pair.
func (r *rateLimiter) recordSuccess(sessionID, remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, rateLimitKey(sessionID, remoteAddr))
}
