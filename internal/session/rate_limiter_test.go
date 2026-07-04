package session

import (
	"strings"
	"testing"
	"time"
)

func TestLoginRateLimiterBlocksBurstAttempts(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	limiter := NewLoginRateLimiter(LoginRateLimiterOptions{
		Window:      5 * time.Minute,
		MaxAttempts: 20,
		Burst:       2,
		BurstWindow: time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	for i := 0; i < 2; i++ {
		if ok, reason := limiter.Check("auth:127.0.0.1"); !ok {
			t.Fatalf("attempt %d blocked: %s", i, reason)
		}
		limiter.Record("auth:127.0.0.1")
	}

	ok, reason := limiter.Check("auth:127.0.0.1")
	if ok {
		t.Fatal("third burst attempt allowed, want blocked")
	}
	if !strings.Contains(reason, "短时间内发送过快") || !strings.Contains(reason, "auth:127.0.0.1") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestLoginRateLimiterBlocksWindowAttemptsAndRecovers(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	limiter := NewLoginRateLimiter(LoginRateLimiterOptions{
		Window:      10 * time.Second,
		MaxAttempts: 2,
		Burst:       10,
		BurstWindow: time.Second,
		Now: func() time.Time {
			return now
		},
	})

	for i := 0; i < 2; i++ {
		limiter.Record("auth:10.0.0.1")
	}
	ok, reason := limiter.Check("auth:10.0.0.1")
	if ok {
		t.Fatal("window-limited attempt allowed, want blocked")
	}
	if !strings.Contains(reason, "发送频率过高") {
		t.Fatalf("unexpected reason: %s", reason)
	}

	now = now.Add(11 * time.Second)
	ok, reason = limiter.Check("auth:10.0.0.1")
	if !ok {
		t.Fatalf("attempt after window blocked: %s", reason)
	}
}
