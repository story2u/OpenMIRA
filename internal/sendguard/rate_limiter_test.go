package sendguard

import (
	"strings"
	"testing"
	"time"
)

func TestRateLimiterBlocksBurstAndRecovers(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimiterOptions{
		Window:      time.Minute,
		MaxSends:    20,
		Burst:       2,
		BurstWindow: 5 * time.Second,
		Now:         func() time.Time { return now },
	})

	if allowed, reason := limiter.Check("device-1"); !allowed || reason != "" {
		t.Fatalf("first check = %v %q, want allowed", allowed, reason)
	}
	limiter.Record("device-1")
	limiter.Record("device-1")
	allowed, reason := limiter.Check("device-1")
	if allowed || !strings.Contains(reason, "短时间内发送过快") {
		t.Fatalf("burst check = %v %q, want burst block", allowed, reason)
	}
	now = now.Add(6 * time.Second)
	allowed, reason = limiter.Check("device-1")
	if !allowed || reason != "" {
		t.Fatalf("after burst window = %v %q, want allowed", allowed, reason)
	}
}

func TestRateLimiterBlocksWindowAndRecovers(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimiterOptions{
		Window:      10 * time.Second,
		MaxSends:    2,
		Burst:       10,
		BurstWindow: time.Second,
		Now:         func() time.Time { return now },
	})

	limiter.Record("device-1")
	now = now.Add(time.Second)
	limiter.Record("device-1")
	allowed, reason := limiter.Check("device-1")
	if allowed || !strings.Contains(reason, "发送频率过高") {
		t.Fatalf("window check = %v %q, want window block", allowed, reason)
	}
	now = now.Add(10 * time.Second)
	allowed, reason = limiter.Check("device-1")
	if !allowed || reason != "" {
		t.Fatalf("after window = %v %q, want allowed", allowed, reason)
	}
}
