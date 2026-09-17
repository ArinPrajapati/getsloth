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
