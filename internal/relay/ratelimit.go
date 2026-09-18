package relay

import (
	"strings"
	"sync"
	"time"
)

// RateLimitMaxAttempts and RateLimitCooldown are the confirmed
// constants from docs/protocol.md.
const (
	RateLimitMaxAttempts = 5
	RateLimitCooldown    = 60 * time.Second
)

// MaxConcurrentAuthAttempts caps how many auth attempts from one
// (session, address) pair can be awaiting a host verdict at once.
// RateLimitMaxAttempts alone only counts *completed* round-trips (it's
// incremented from the host's response) - a burst of auth messages sent
// before any response comes back isn't limited by it at all, since none
// of them have failed yet by the time the next one arrives. This cap
// closes that gap independently of the cooldown mechanism.
const MaxConcurrentAuthAttempts = 3

// rateLimiter tracks failed auth attempts and in-flight reservations
// per (sessionID, remoteAddr), per docs/protocol.md's Rate limiting
// section - keyed on more than the ephemeral WebSocket connection so
// reconnecting doesn't reset the counter.
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
}

type rateEntry struct {
	fails        int
	blockedUntil time.Time
	pending      int
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

// tryReserve atomically checks both rate-limit mechanisms - the cooldown
// block from repeated completed failures, and the concurrent-in-flight
// cap - and, if neither applies, reserves a slot (incrementing pending)
// so a flood of simultaneous attempts can't all bypass the cooldown
// check the way calling blocked() followed by a separate increment
// would allow. Every successful reservation must be matched by exactly
// one release call once that attempt resolves.
func (r *rateLimiter) tryReserve(sessionID, remoteAddr string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := rateLimitKey(sessionID, remoteAddr)
	e, ok := r.entries[k]
	if !ok {
		e = &rateEntry{}
		r.entries[k] = e
	}

	if remaining := time.Until(e.blockedUntil); remaining > 0 {
		return false, remaining
	}
	if e.pending >= MaxConcurrentAuthAttempts {
		return false, RateLimitCooldown
	}

	e.pending++
	return true, 0
}

// release frees a slot reserved by tryReserve. Safe to call even if the
// entry has since been removed (e.g. by clearSession) - it's a no-op in
// that case rather than a panic, since teardown races with in-flight
// requests resolving are expected, not exceptional.
func (r *rateLimiter) release(sessionID, remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[rateLimitKey(sessionID, remoteAddr)]
	if !ok {
		return
	}
	if e.pending > 0 {
		e.pending--
	}
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

// recordSuccess clears any tracked failures for this pair. Deliberately
// leaves the entry itself in place rather than deleting it outright,
// since a concurrent tryReserve/release for the same pair could be
// racing this call - clearSession (tied to session teardown) is what
// actually reclaims the memory.
func (r *rateLimiter) recordSuccess(sessionID, remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[rateLimitKey(sessionID, remoteAddr)]
	if !ok {
		return
	}
	e.fails = 0
	e.blockedUntil = time.Time{}
}

// clearSession removes every entry belonging to sessionID. Without this,
// entries accumulate for the lifetime of the relay *process*, not just
// the session - called once a session tears down, since nothing else
// ties rate-limiter state to session lifecycle.
func (r *rateLimiter) clearSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := sessionID + "|"
	for k := range r.entries {
		if strings.HasPrefix(k, prefix) {
			delete(r.entries, k)
		}
	}
}
