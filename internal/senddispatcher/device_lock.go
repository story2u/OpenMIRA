package senddispatcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDeviceLockWaitTimeoutSeconds     = 3600
	defaultExecutorSubprocessTimeoutSeconds = 180
	minDeviceLockTTLMilliseconds            = 10000
	minComputedDeviceLockTTLMilliseconds    = 240000
	defaultDeviceLockWaitLogThresholdMillis = 1000
	defaultDeviceLockWaitLogIntervalSeconds = 5
	defaultDeviceLockRetryInterval          = 200 * time.Millisecond
	DeviceLockReleaseScript                 = "if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end"
)

// DeviceLockStore is the Redis SET-NX/EVAL boundary used by SDK device locks.
type DeviceLockStore interface {
	SetDeviceLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error)
	ReleaseDeviceLock(ctx context.Context, key string, token string) error
}

// DeviceLockInspector reads owner diagnostics for a busy Redis device lock.
type DeviceLockInspector interface {
	DeviceLockOwner(ctx context.Context, key string) (string, error)
	DeviceLockPTTL(ctx context.Context, key string) (time.Duration, error)
}

// DeviceLockRequest describes one SDK device lock acquire attempt.
type DeviceLockRequest struct {
	DeviceID               string
	TaskID                 string
	Nonce                  string
	PID                    int
	ExecutorTimeoutSeconds int
	Env                    EnvLookup
}

// DeviceLockState is the local owner state needed for safe release.
type DeviceLockState struct {
	Key      string
	Token    string
	TTL      time.Duration
	Acquired bool
}

// DeviceLockWaitOptions controls retry behavior for Redis device lock acquire.
type DeviceLockWaitOptions struct {
	Request       DeviceLockRequest
	Inspector     DeviceLockInspector
	OnWait        func(DeviceLockWaitEvent)
	RetryInterval time.Duration
	WaitTimeout   time.Duration
	Now           func() time.Time
	Sleep         func(context.Context, time.Duration) error
}

// DeviceLockWaitEvent is emitted when a busy lock wait crosses the diagnostics threshold.
type DeviceLockWaitEvent struct {
	TaskID   string
	DeviceID string
	Key      string
	Waited   time.Duration
	Owner    string
	PTTL     time.Duration
	PTTLOK   bool
}

// DeviceLockKey returns the Redis key for a canonical SDK device id.
func DeviceLockKey(deviceID string) (string, bool) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", false
	}
	return "lock:sdk-device:" + deviceID, true
}

// DeviceLockToken returns the lock owner token shape used by Python SDK sends.
func DeviceLockToken(pid int, taskID string, nonce string) string {
	if pid <= 0 {
		pid = os.Getpid()
	}
	return fmt.Sprintf("%d:%s:%s", pid, strings.TrimSpace(taskID), strings.TrimSpace(nonce))
}

// NewDeviceLockNonce returns a UUID-like random hex nonce for lock ownership.
func NewDeviceLockNonce() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

// TryAcquireDeviceLock applies Python-compatible SET NX PX lock semantics once.
func TryAcquireDeviceLock(ctx context.Context, store DeviceLockStore, request DeviceLockRequest) (DeviceLockState, bool, error) {
	if store == nil {
		return DeviceLockState{}, false, nil
	}
	key, ok := DeviceLockKey(request.DeviceID)
	if !ok {
		return DeviceLockState{}, false, nil
	}
	state := DeviceLockState{
		Key:   key,
		Token: DeviceLockToken(request.PID, request.TaskID, request.Nonce),
		TTL:   deviceLockDurationFromMilliseconds(DeviceLockTTLMilliseconds(request.Env, request.ExecutorTimeoutSeconds)),
	}
	acquired, err := store.SetDeviceLock(ctx, state.Key, state.Token, state.TTL)
	if err != nil || !acquired {
		return state, false, err
	}
	state.Acquired = true
	return state, true, nil
}

// AcquireDeviceLock retries SET-NX until acquired or the Python-compatible timeout expires.
func AcquireDeviceLock(ctx context.Context, store DeviceLockStore, options DeviceLockWaitOptions) (DeviceLockState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := options.Request
	startedAt := deviceLockNow(options.Now)
	waitTimeout := options.WaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = time.Duration(DeviceLockWaitTimeoutSeconds(request.Env)) * time.Second
	}
	retryInterval := options.RetryInterval
	if retryInterval <= 0 {
		retryInterval = defaultDeviceLockRetryInterval
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	waitLogThreshold := deviceLockDurationFromMilliseconds(DeviceLockWaitLogThresholdMilliseconds(request.Env))
	waitLogInterval := time.Duration(DeviceLockWaitLogIntervalSeconds(request.Env) * float64(time.Second))
	lastWaitLogAt := time.Time{}
	for {
		state, acquired, err := TryAcquireDeviceLock(ctx, store, request)
		if err != nil || acquired || state.Key == "" {
			return state, err
		}
		now := deviceLockNow(options.Now)
		waited := now.Sub(startedAt)
		if shouldEmitDeviceLockWaitEvent(waited, waitLogThreshold, now, lastWaitLogAt, waitLogInterval) {
			options.emitWaitEvent(ctx, state.Key, request, waited)
			lastWaitLogAt = now
		}
		if waited >= waitTimeout {
			return state, fmt.Errorf("sdk device lock wait timeout: device_id=%s", strings.TrimSpace(request.DeviceID))
		}
		if err := sleep(ctx, retryInterval); err != nil {
			return state, err
		}
	}
}

// ReleaseDeviceLock releases a previously acquired SDK device lock.
func ReleaseDeviceLock(ctx context.Context, store DeviceLockStore, state DeviceLockState) error {
	if store == nil || !state.Acquired || strings.TrimSpace(state.Key) == "" || strings.TrimSpace(state.Token) == "" {
		return nil
	}
	return store.ReleaseDeviceLock(ctx, state.Key, state.Token)
}

// DeviceLockWaitTimeoutSeconds returns how long a task may wait for a device lock.
func DeviceLockWaitTimeoutSeconds(lookup EnvLookup) int {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_LOCK_WAIT_TIMEOUT_SEC"))
	if raw == "" {
		return defaultDeviceLockWaitTimeoutSeconds
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultDeviceLockWaitTimeoutSeconds
	}
	if value < 1 {
		return 1
	}
	return value
}

// DeviceLockTTLMilliseconds returns the Redis TTL used for SDK device locks.
func DeviceLockTTLMilliseconds(lookup EnvLookup, executorTimeoutSeconds int) int {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_LOCK_TTL_MS"))
	if raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			if value < minDeviceLockTTLMilliseconds {
				return minDeviceLockTTLMilliseconds
			}
			return value
		}
	}
	if executorTimeoutSeconds <= 0 {
		executorTimeoutSeconds = defaultExecutorSubprocessTimeoutSeconds
	}
	batchSize := BatchMaxSize(lookup)
	extraBatches := batchSize - 1
	if extraBatches < 1 {
		extraBatches = 1
	}
	computed := (executorTimeoutSeconds + 60*extraBatches + 60) * 1000
	if computed < minComputedDeviceLockTTLMilliseconds {
		return minComputedDeviceLockTTLMilliseconds
	}
	return computed
}

// DeviceLockWaitLogThresholdMilliseconds returns when owner diagnostics start.
func DeviceLockWaitLogThresholdMilliseconds(lookup EnvLookup) int {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_LOCK_WAIT_LOG_MS"))
	if raw == "" {
		return defaultDeviceLockWaitLogThresholdMillis
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultDeviceLockWaitLogThresholdMillis
	}
	if value < 0 {
		return 0
	}
	return value
}

// DeviceLockWaitLogIntervalSeconds returns repeated wait diagnostic spacing.
func DeviceLockWaitLogIntervalSeconds(lookup EnvLookup) float64 {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_LOCK_WAIT_LOG_INTERVAL_SEC"))
	if raw == "" {
		return defaultDeviceLockWaitLogIntervalSeconds
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultDeviceLockWaitLogIntervalSeconds
	}
	if value < 1 {
		return 1
	}
	return value
}

func deviceLockDurationFromMilliseconds(milliseconds int) time.Duration {
	return time.Duration(milliseconds) * time.Millisecond
}

func deviceLockNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldEmitDeviceLockWaitEvent(waited time.Duration, threshold time.Duration, now time.Time, last time.Time, interval time.Duration) bool {
	if waited < threshold {
		return false
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= interval
}

func (options DeviceLockWaitOptions) emitWaitEvent(ctx context.Context, key string, request DeviceLockRequest, waited time.Duration) {
	if options.OnWait == nil {
		return
	}
	event := DeviceLockWaitEvent{
		TaskID:   strings.TrimSpace(request.TaskID),
		DeviceID: strings.TrimSpace(request.DeviceID),
		Key:      key,
		Waited:   waited,
	}
	if options.Inspector != nil {
		if owner, err := options.Inspector.DeviceLockOwner(ctx, key); err == nil {
			event.Owner = truncateString(strings.TrimSpace(owner), 160)
		}
		if pttl, err := options.Inspector.DeviceLockPTTL(ctx, key); err == nil {
			event.PTTL = pttl
			event.PTTLOK = true
		}
	}
	options.OnWait(event)
}

func truncateString(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
