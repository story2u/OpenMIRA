package session

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	defaultLoginRateWindow      = 300 * time.Second
	defaultLoginRateMaxAttempts = 20
	defaultLoginRateBurst       = 5
	defaultLoginRateBurstWindow = 60 * time.Second
)

// LoginRateLimiterOptions mirrors AUTH_RATE_LIMIT_* runtime knobs.
type LoginRateLimiterOptions struct {
	Window      time.Duration
	MaxAttempts int
	Burst       int
	BurstWindow time.Duration
	Now         func() time.Time
}

// LoginRateLimiter tracks per-source authentication attempts in memory.
type LoginRateLimiter struct {
	Window      time.Duration
	MaxAttempts int
	Burst       int
	BurstWindow time.Duration
	Now         func() time.Time

	mu      sync.Mutex
	records map[string][]time.Time
}

// NewLoginRateLimiter builds the legacy sliding-window auth limiter.
func NewLoginRateLimiter(options LoginRateLimiterOptions) *LoginRateLimiter {
	limiter := &LoginRateLimiter{
		Window:      options.Window,
		MaxAttempts: options.MaxAttempts,
		Burst:       options.Burst,
		BurstWindow: options.BurstWindow,
		Now:         options.Now,
		records:     map[string][]time.Time{},
	}
	if limiter.Window <= 0 {
		limiter.Window = defaultLoginRateWindow
	}
	if limiter.MaxAttempts <= 0 {
		limiter.MaxAttempts = defaultLoginRateMaxAttempts
	}
	if limiter.Burst <= 0 {
		limiter.Burst = defaultLoginRateBurst
	}
	if limiter.BurstWindow <= 0 {
		limiter.BurstWindow = defaultLoginRateBurstWindow
	}
	return limiter
}

// Check reports whether another attempt is allowed for key.
func (limiter *LoginRateLimiter) Check(key string) (bool, string) {
	if limiter == nil {
		return true, ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return true, ""
	}
	now := limiter.clock()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	records := limiter.cleanupLocked(key, now)
	if len(records) >= limiter.maxAttempts() {
		remaining := records[0].Add(limiter.window()).Sub(now)
		return false, fmt.Sprintf(
			"设备 %s 发送频率过高，%.0f秒内已发送%d条，请等待%.0f秒后重试。",
			key,
			limiter.window().Seconds(),
			len(records),
			math.Max(0, remaining.Seconds()),
		)
	}

	burstCutoff := now.Add(-limiter.burstWindow())
	burstCount := 0
	for _, timestamp := range records {
		if !timestamp.Before(burstCutoff) {
			burstCount++
		}
	}
	if burstCount >= limiter.burst() {
		return false, fmt.Sprintf(
			"设备 %s 短时间内发送过快，%.0f秒内已发送%d条，请稍后再试。",
			key,
			limiter.burstWindow().Seconds(),
			burstCount,
		)
	}
	return true, ""
}

// Record stores one completed auth attempt for key.
func (limiter *LoginRateLimiter) Record(key string) {
	if limiter == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	now := limiter.clock()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	records := limiter.cleanupLocked(key, now)
	limiter.records[key] = append(records, now)
}

func (limiter *LoginRateLimiter) cleanupLocked(key string, now time.Time) []time.Time {
	records := limiter.records[key]
	cutoff := now.Add(-limiter.window())
	start := 0
	for start < len(records) && records[start].Before(cutoff) {
		start++
	}
	records = append([]time.Time{}, records[start:]...)
	limiter.records[key] = records
	return records
}

func (limiter *LoginRateLimiter) clock() time.Time {
	if limiter.Now != nil {
		return limiter.Now()
	}
	return time.Now()
}

func (limiter *LoginRateLimiter) window() time.Duration {
	if limiter.Window <= 0 {
		return defaultLoginRateWindow
	}
	return limiter.Window
}

func (limiter *LoginRateLimiter) maxAttempts() int {
	if limiter.MaxAttempts <= 0 {
		return defaultLoginRateMaxAttempts
	}
	return limiter.MaxAttempts
}

func (limiter *LoginRateLimiter) burst() int {
	if limiter.Burst <= 0 {
		return defaultLoginRateBurst
	}
	return limiter.Burst
}

func (limiter *LoginRateLimiter) burstWindow() time.Duration {
	if limiter.BurstWindow <= 0 {
		return defaultLoginRateBurstWindow
	}
	return limiter.BurstWindow
}
