package relay

import "testing"

func TestRateLimiter_AllowsUpToMaxAttempts(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < RateLimitMaxAttempts-1; i++ {
		if blocked, _ := rl.blocked("sess", "1.2.3.4"); blocked {
			t.Fatalf("blocked after only %d failures, want allowed up to %d", i, RateLimitMaxAttempts)
		}
		rl.recordFailure("sess", "1.2.3.4")
	}

	if blocked, _ := rl.blocked("sess", "1.2.3.4"); blocked {
		t.Error("blocked before reaching RateLimitMaxAttempts")
	}
}

func TestRateLimiter_BlocksAtMaxAttempts(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < RateLimitMaxAttempts; i++ {
		rl.recordFailure("sess", "1.2.3.4")
	}

	blocked, retryAfter := rl.blocked("sess", "1.2.3.4")
	if !blocked {
		t.Fatal("not blocked after RateLimitMaxAttempts failures")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %v, want > 0", retryAfter)
	}
}

func TestRateLimiter_KeyedPerSessionAndAddress_NotGlobal(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < RateLimitMaxAttempts; i++ {
		rl.recordFailure("sess-a", "1.2.3.4")
	}

	if blocked, _ := rl.blocked("sess-a", "5.6.7.8"); blocked {
		t.Error("a different address on the same session got blocked - rate limit isn't scoped correctly")
	}
	if blocked, _ := rl.blocked("sess-b", "1.2.3.4"); blocked {
		t.Error("a different session from the same address got blocked - rate limit isn't scoped correctly")
	}
}

func TestRateLimiter_TryReserve_CapsConcurrentAttempts(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < MaxConcurrentAuthAttempts; i++ {
		ok, _ := rl.tryReserve("sess", "1.2.3.4")
		if !ok {
			t.Fatalf("reservation #%d rejected, want allowed up to %d concurrent", i, MaxConcurrentAuthAttempts)
		}
	}

	// One more concurrent attempt, with none of the prior ones resolved
	// yet (no recordFailure/recordSuccess/release called) - this is the
	// flood case: many auth messages sent before any round-trip
	// completes, which recordFailure-based blocking alone can't catch.
	if ok, _ := rl.tryReserve("sess", "1.2.3.4"); ok {
		t.Error("reservation succeeded beyond MaxConcurrentAuthAttempts with none released")
	}
}

func TestRateLimiter_Release_FreesSlotForNextReserve(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < MaxConcurrentAuthAttempts; i++ {
		if ok, _ := rl.tryReserve("sess", "1.2.3.4"); !ok {
			t.Fatalf("reservation #%d rejected", i)
		}
	}
	rl.release("sess", "1.2.3.4")

	if ok, _ := rl.tryReserve("sess", "1.2.3.4"); !ok {
		t.Error("reservation rejected after a release freed a slot")
	}
}

func TestRateLimiter_TryReserve_StillRespectsCooldownBlock(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < RateLimitMaxAttempts; i++ {
		rl.recordFailure("sess", "1.2.3.4")
	}

	// Cooldown blocking must still apply even with free concurrent
	// slots - the two mechanisms are independent, not a replacement for
	// each other.
	if ok, retryAfter := rl.tryReserve("sess", "1.2.3.4"); ok || retryAfter <= 0 {
		t.Errorf("tryReserve during cooldown = (%v, %v), want (false, >0)", ok, retryAfter)
	}
}

func TestRateLimiter_ClearSession_RemovesOnlyThatSessionsEntries(t *testing.T) {
	rl := newRateLimiter()
	rl.recordFailure("sess-a", "1.2.3.4")
	rl.recordFailure("sess-b", "5.6.7.8")

	rl.clearSession("sess-a")

	rl.mu.Lock()
	_, aStillPresent := rl.entries[rateLimitKey("sess-a", "1.2.3.4")]
	_, bStillPresent := rl.entries[rateLimitKey("sess-b", "5.6.7.8")]
	rl.mu.Unlock()

	if aStillPresent {
		t.Error("sess-a's entry survived clearSession(\"sess-a\")")
	}
	if !bStillPresent {
		t.Error("clearSession(\"sess-a\") incorrectly removed sess-b's entry too")
	}
}

func TestRateLimiter_SuccessClearsFailures(t *testing.T) {
	rl := newRateLimiter()

	for i := 0; i < RateLimitMaxAttempts-1; i++ {
		rl.recordFailure("sess", "1.2.3.4")
	}
	rl.recordSuccess("sess", "1.2.3.4")

	// One more failure right after a success shouldn't be treated as
	// the (RateLimitMaxAttempts)th of a still-running streak.
	rl.recordFailure("sess", "1.2.3.4")
	if blocked, _ := rl.blocked("sess", "1.2.3.4"); blocked {
		t.Error("blocked immediately after a success reset the counter, want allowed")
	}
}
