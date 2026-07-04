// Package sendguard contains shared manual-send preflight guards.
package sendguard

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Limiter is the minimal interface send services need.
type Limiter interface {
	Check(deviceID string) (bool, string)
	Record(deviceID string)
}

// RateLimitError maps device send throttling to HTTP 429.
type RateLimitError struct {
	Reason string
}

func (err RateLimitError) Error() string {
	return strings.TrimSpace(err.Reason)
}

// RateLimiterOptions mirrors Python RATE_LIMIT_* runtime knobs.
type RateLimiterOptions struct {
	Window      time.Duration
	MaxSends    int
	Burst       int
	BurstWindow time.Duration
	Now         func() time.Time
}

// RateLimiter tracks per-device successful manual sends in memory.
type RateLimiter struct {
	mu      sync.Mutex
	records map[string][]time.Time
	options RateLimiterOptions
}

// NewRateLimiter builds the legacy sliding-window device send limiter.
func NewRateLimiter(options RateLimiterOptions) *RateLimiter {
	if options.Window <= 0 {
		options.Window = time.Minute
	}
	if options.MaxSends <= 0 {
		options.MaxSends = 20
	}
	if options.Burst <= 0 {
		options.Burst = 5
	}
	if options.BurstWindow <= 0 {
		options.BurstWindow = 5 * time.Second
	}
	return &RateLimiter{records: map[string][]time.Time{}, options: options}
}

// Check returns whether another send can be accepted for a device.
func (limiter *RateLimiter) Check(deviceID string) (bool, string) {
	if limiter == nil {
		return true, ""
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return true, ""
	}
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	records := limiter.cleanupLocked(deviceID, now)
	if len(records) >= limiter.maxSends() {
		remaining := records[0].Add(limiter.window()).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return false, fmt.Sprintf("设备 %s 发送频率过高，%.0f秒内已发送%d条，请等待%.0f秒后重试。", deviceID, limiter.window().Seconds(), len(records), remaining.Seconds())
	}
	burstCutoff := now.Add(-limiter.burstWindow())
	burstCount := 0
	for _, record := range records {
		if !record.Before(burstCutoff) {
			burstCount++
		}
	}
	if burstCount >= limiter.burst() {
		return false, fmt.Sprintf("设备 %s 短时间内发送过快，%.0f秒内已发送%d条，请稍后再试。", deviceID, limiter.burstWindow().Seconds(), burstCount)
	}
	return true, ""
}

// Record stores one successful accepted send for a device.
func (limiter *RateLimiter) Record(deviceID string) {
	if limiter == nil {
		return
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	records := limiter.cleanupLocked(deviceID, now)
	limiter.records[deviceID] = append(records, now)
}

func (limiter *RateLimiter) cleanupLocked(deviceID string, now time.Time) []time.Time {
	cutoff := now.Add(-limiter.window())
	records := limiter.records[deviceID]
	start := 0
	for start < len(records) && records[start].Before(cutoff) {
		start++
	}
	records = append([]time.Time(nil), records[start:]...)
	limiter.records[deviceID] = records
	return records
}

func (limiter *RateLimiter) now() time.Time {
	if limiter.options.Now != nil {
		return limiter.options.Now().UTC()
	}
	return time.Now().UTC()
}

func (limiter *RateLimiter) window() time.Duration {
	return limiter.options.Window
}

func (limiter *RateLimiter) maxSends() int {
	return limiter.options.MaxSends
}

func (limiter *RateLimiter) burst() int {
	return limiter.options.Burst
}

func (limiter *RateLimiter) burstWindow() time.Duration {
	return limiter.options.BurstWindow
}
