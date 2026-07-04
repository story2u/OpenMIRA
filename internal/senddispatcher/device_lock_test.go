package senddispatcher

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestDeviceLockKeyAndTokenMirrorPythonShape protects Redis key/token compatibility.
func TestDeviceLockKeyAndTokenMirrorPythonShape(t *testing.T) {
	key, ok := DeviceLockKey(" zimo ")
	if !ok || key != "lock:sdk-device:zimo" {
		t.Fatalf("key=%q ok=%t", key, ok)
	}
	if key, ok := DeviceLockKey(" "); ok || key != "" {
		t.Fatalf("blank key=%q ok=%t", key, ok)
	}
	token := DeviceLockToken(123, " task-1 ", " abc ")
	if token != "123:task-1:abc" {
		t.Fatalf("token = %q", token)
	}
	if !strings.Contains(DeviceLockReleaseScript, "redis.call('get', KEYS[1]) == ARGV[1]") {
		t.Fatalf("release script changed: %s", DeviceLockReleaseScript)
	}
}

// TestDeviceLockWaitTimeoutMirrorsPythonEnvRules protects lock wait timeout parsing.
func TestDeviceLockWaitTimeoutMirrorsPythonEnvRules(t *testing.T) {
	if got := DeviceLockWaitTimeoutSeconds(mapLookup(map[string]string{})); got != 3600 {
		t.Fatalf("default wait timeout = %d", got)
	}
	if got := DeviceLockWaitTimeoutSeconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_TIMEOUT_SEC": "0"})); got != 1 {
		t.Fatalf("min wait timeout = %d", got)
	}
	if got := DeviceLockWaitTimeoutSeconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_TIMEOUT_SEC": "bad"})); got != 3600 {
		t.Fatalf("invalid wait timeout = %d", got)
	}
}

// TestDeviceLockTTLMatchesPythonFallback protects explicit and computed TTL behavior.
func TestDeviceLockTTLMatchesPythonFallback(t *testing.T) {
	if got := DeviceLockTTLMilliseconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_TTL_MS": "5000"}), 180); got != 10000 {
		t.Fatalf("explicit min ttl = %d", got)
	}
	if got := DeviceLockTTLMilliseconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_TTL_MS": "12000"}), 180); got != 12000 {
		t.Fatalf("explicit ttl = %d", got)
	}
	if got := DeviceLockTTLMilliseconds(mapLookup(map[string]string{}), 180); got != 780000 {
		t.Fatalf("computed default ttl = %d", got)
	}
	if got := DeviceLockTTLMilliseconds(mapLookup(map[string]string{"P1_SDK_DEVICE_BATCH_MAX_SIZE": "1"}), 30); got != 240000 {
		t.Fatalf("computed min ttl = %d", got)
	}
	if got := deviceLockDurationFromMilliseconds(250); got != 250*time.Millisecond {
		t.Fatalf("duration = %s", got)
	}
}

// TestDeviceLockWaitLogPolicyMirrorsPythonEnvRules protects diagnostics throttle knobs.
func TestDeviceLockWaitLogPolicyMirrorsPythonEnvRules(t *testing.T) {
	if got := DeviceLockWaitLogThresholdMilliseconds(mapLookup(map[string]string{})); got != 1000 {
		t.Fatalf("default threshold = %d", got)
	}
	if got := DeviceLockWaitLogThresholdMilliseconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_LOG_MS": "-1"})); got != 0 {
		t.Fatalf("min threshold = %d", got)
	}
	if got := DeviceLockWaitLogThresholdMilliseconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_LOG_MS": "bad"})); got != 1000 {
		t.Fatalf("invalid threshold = %d", got)
	}
	if got := DeviceLockWaitLogIntervalSeconds(mapLookup(map[string]string{})); got != 5 {
		t.Fatalf("default interval = %v", got)
	}
	if got := DeviceLockWaitLogIntervalSeconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_LOG_INTERVAL_SEC": "0.1"})); got != 1 {
		t.Fatalf("min interval = %v", got)
	}
	if got := DeviceLockWaitLogIntervalSeconds(mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_WAIT_LOG_INTERVAL_SEC": "bad"})); got != 5 {
		t.Fatalf("invalid interval = %v", got)
	}
}

// TestTryAcquireDeviceLockNoOpsWithoutStoreOrDevice keeps Redis optional at assembly time.
func TestTryAcquireDeviceLockNoOpsWithoutStoreOrDevice(t *testing.T) {
	if state, acquired, err := TryAcquireDeviceLock(context.Background(), nil, DeviceLockRequest{DeviceID: "zimo"}); err != nil || acquired || state.Key != "" {
		t.Fatalf("nil store state=%#v acquired=%t err=%v", state, acquired, err)
	}
	store := &recordingDeviceLockStore{acquireResult: true}
	if state, acquired, err := TryAcquireDeviceLock(context.Background(), store, DeviceLockRequest{DeviceID: " "}); err != nil || acquired || state.Key != "" || store.acquireCalls != 0 {
		t.Fatalf("blank device state=%#v acquired=%t err=%v calls=%d", state, acquired, err, store.acquireCalls)
	}
}

// TestTryAcquireDeviceLockDelegatesSETNXBoundary protects key/token/TTL parameters.
func TestTryAcquireDeviceLockDelegatesSETNXBoundary(t *testing.T) {
	store := &recordingDeviceLockStore{acquireResult: true}
	state, acquired, err := TryAcquireDeviceLock(context.Background(), store, DeviceLockRequest{
		DeviceID:               " zimo ",
		TaskID:                 " task-1 ",
		Nonce:                  "abc",
		PID:                    123,
		ExecutorTimeoutSeconds: 30,
		Env:                    mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_TTL_MS": "12000"}),
	})
	if err != nil || !acquired || !state.Acquired {
		t.Fatalf("state=%#v acquired=%t err=%v", state, acquired, err)
	}
	if store.key != "lock:sdk-device:zimo" || store.token != "123:task-1:abc" || store.ttl != 12*time.Second {
		t.Fatalf("store = %#v", store)
	}
	if state.Key != store.key || state.Token != store.token || state.TTL != store.ttl {
		t.Fatalf("state=%#v store=%#v", state, store)
	}
}

// TestReleaseDeviceLockOnlyReleasesOwnedState protects token-checked release boundary.
func TestReleaseDeviceLockOnlyReleasesOwnedState(t *testing.T) {
	store := &recordingDeviceLockStore{}
	if err := ReleaseDeviceLock(context.Background(), store, DeviceLockState{Key: "lock:sdk-device:zimo", Token: "token"}); err != nil {
		t.Fatalf("ReleaseDeviceLock unacquired returned error: %v", err)
	}
	if store.releaseCalls != 0 {
		t.Fatalf("release calls = %d", store.releaseCalls)
	}
	err := ReleaseDeviceLock(context.Background(), store, DeviceLockState{
		Key:      "lock:sdk-device:zimo",
		Token:    "123:task-1:abc",
		Acquired: true,
	})
	if err != nil {
		t.Fatalf("ReleaseDeviceLock returned error: %v", err)
	}
	if store.releaseCalls != 1 || store.releaseKey != "lock:sdk-device:zimo" || store.releaseToken != "123:task-1:abc" {
		t.Fatalf("store = %#v", store)
	}
}

// TestAcquireDeviceLockRetriesUntilAcquired mirrors Python wait-and-retry behavior.
func TestAcquireDeviceLockRetriesUntilAcquired(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	sleepCalls := 0
	store := &recordingDeviceLockStore{acquireResults: []bool{false, true}}
	state, err := AcquireDeviceLock(context.Background(), store, DeviceLockWaitOptions{
		Request:       DeviceLockRequest{DeviceID: "zimo", TaskID: "task-1", Nonce: "abc", PID: 123},
		RetryInterval: 200 * time.Millisecond,
		WaitTimeout:   time.Second,
		Now:           func() time.Time { return now },
		Sleep: func(_ context.Context, duration time.Duration) error {
			sleepCalls++
			now = now.Add(duration)
			return nil
		},
	})
	if err != nil || !state.Acquired {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if store.acquireCalls != 2 || sleepCalls != 1 {
		t.Fatalf("acquireCalls=%d sleepCalls=%d", store.acquireCalls, sleepCalls)
	}
}

// TestAcquireDeviceLockTimesOut mirrors Python wait timeout error text.
func TestAcquireDeviceLockTimesOut(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	store := &recordingDeviceLockStore{acquireResults: []bool{false, false, false}}
	state, err := AcquireDeviceLock(context.Background(), store, DeviceLockWaitOptions{
		Request:       DeviceLockRequest{DeviceID: " zimo ", TaskID: "task-1", Nonce: "abc", PID: 123},
		RetryInterval: time.Second,
		WaitTimeout:   time.Second,
		Now:           func() time.Time { return now },
		Sleep: func(_ context.Context, duration time.Duration) error {
			now = now.Add(duration)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "sdk device lock wait timeout: device_id=zimo") {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if state.Acquired || store.acquireCalls != 2 {
		t.Fatalf("state=%#v acquireCalls=%d", state, store.acquireCalls)
	}
}

// TestAcquireDeviceLockReturnsSleepError keeps cancelled waits observable.
func TestAcquireDeviceLockReturnsSleepError(t *testing.T) {
	sleepErr := context.Canceled
	store := &recordingDeviceLockStore{acquireResults: []bool{false}}
	_, err := AcquireDeviceLock(context.Background(), store, DeviceLockWaitOptions{
		Request:     DeviceLockRequest{DeviceID: "zimo", TaskID: "task-1", Nonce: "abc", PID: 123},
		WaitTimeout: time.Second,
		Sleep: func(context.Context, time.Duration) error {
			return sleepErr
		},
	})
	if err != sleepErr {
		t.Fatalf("error = %v", err)
	}
}

// TestAcquireDeviceLockEmitsWaitDiagnostics mirrors Python owner/PTTL wait logging data.
func TestAcquireDeviceLockEmitsWaitDiagnostics(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	store := &recordingDeviceLockStore{
		acquireResults: []bool{false, false, false, true},
		owner:          strings.Repeat("长", 200),
		pttl:           3 * time.Second,
	}
	events := []DeviceLockWaitEvent{}
	state, err := AcquireDeviceLock(context.Background(), store, DeviceLockWaitOptions{
		Request: DeviceLockRequest{
			DeviceID: "zimo",
			TaskID:   "task-1",
			Nonce:    "abc",
			PID:      123,
			Env: mapLookup(map[string]string{
				"P1_SDK_DEVICE_LOCK_WAIT_LOG_MS":           "200",
				"P1_SDK_DEVICE_LOCK_WAIT_LOG_INTERVAL_SEC": "1",
			}),
		},
		Inspector:     store,
		RetryInterval: 250 * time.Millisecond,
		WaitTimeout:   2 * time.Second,
		Now:           func() time.Time { return now },
		Sleep: func(_ context.Context, duration time.Duration) error {
			now = now.Add(duration)
			return nil
		},
		OnWait: func(event DeviceLockWaitEvent) {
			events = append(events, event)
		},
	})
	if err != nil || !state.Acquired {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events[0]
	if event.TaskID != "task-1" || event.DeviceID != "zimo" || event.Key != "lock:sdk-device:zimo" || event.Waited != 250*time.Millisecond {
		t.Fatalf("event = %#v", event)
	}
	if len([]rune(event.Owner)) != 160 || event.PTTL != 3*time.Second || !event.PTTLOK {
		t.Fatalf("event diagnostics = %#v", event)
	}
	if store.ownerCalls != 1 || store.pttlCalls != 1 {
		t.Fatalf("ownerCalls=%d pttlCalls=%d", store.ownerCalls, store.pttlCalls)
	}
}

type recordingDeviceLockStore struct {
	acquireResult  bool
	acquireResults []bool
	acquireCalls   int
	key            string
	token          string
	ttl            time.Duration
	releaseCalls   int
	releaseKey     string
	releaseToken   string
	owner          string
	ownerCalls     int
	pttl           time.Duration
	pttlCalls      int
}

func (store *recordingDeviceLockStore) SetDeviceLock(_ context.Context, key string, token string, ttl time.Duration) (bool, error) {
	store.acquireCalls++
	store.key = key
	store.token = token
	store.ttl = ttl
	if len(store.acquireResults) > 0 {
		result := store.acquireResults[0]
		store.acquireResults = store.acquireResults[1:]
		return result, nil
	}
	return store.acquireResult, nil
}

func (store *recordingDeviceLockStore) ReleaseDeviceLock(_ context.Context, key string, token string) error {
	store.releaseCalls++
	store.releaseKey = key
	store.releaseToken = token
	return nil
}

func (store *recordingDeviceLockStore) DeviceLockOwner(_ context.Context, key string) (string, error) {
	store.ownerCalls++
	store.key = key
	return store.owner, nil
}

func (store *recordingDeviceLockStore) DeviceLockPTTL(_ context.Context, key string) (time.Duration, error) {
	store.pttlCalls++
	store.key = key
	return store.pttl, nil
}
