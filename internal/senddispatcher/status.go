package senddispatcher

import (
	"strings"
	"sync"
	"time"
)

// StatusSnapshotInput is the pure data needed to build a dispatcher status snapshot.
type StatusSnapshotInput struct {
	WorkerID         string
	VisibleDeviceIDs []string
	Allowlist        []string
	Exclude          []string
	Backlog          BacklogSummary
	CapturedAt       time.Time
}

// StatusSnapshot mirrors the Python sdk_dispatcher_status_snapshot core shape.
type StatusSnapshot struct {
	WorkerID           string
	VisibleDeviceIDs   []string
	OwnedDeviceIDs     []string
	Allowlist          []string
	Exclude            []string
	VisibleDeviceCount int
	OwnedDeviceCount   int
	Backlog            BacklogSummary
	CapturedAt         time.Time
}

// StatusSnapshotCache keeps a short per-worker discovery/backlog snapshot.
type StatusSnapshotCache struct {
	mutex     sync.Mutex
	snapshot  StatusSnapshot
	expiresAt time.Time
	cached    bool
}

// NewStatusSnapshotCache creates an empty dispatcher status snapshot cache.
func NewStatusSnapshotCache() *StatusSnapshotCache {
	return &StatusSnapshotCache{}
}

// Get returns a cached snapshot when the worker id and TTL still match.
func (cache *StatusSnapshotCache) Get(workerID string, now time.Time) (StatusSnapshot, bool) {
	if cache == nil {
		return StatusSnapshot{}, false
	}
	workerID = strings.TrimSpace(workerID)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if !cache.cached {
		return StatusSnapshot{}, false
	}
	if strings.TrimSpace(cache.snapshot.WorkerID) != workerID {
		return StatusSnapshot{}, false
	}
	if !cache.expiresAt.After(now.UTC()) {
		return StatusSnapshot{}, false
	}
	return cloneStatusSnapshot(cache.snapshot), true
}

// Put stores one snapshot for a positive TTL.
func (cache *StatusSnapshotCache) Put(snapshot StatusSnapshot, now time.Time, ttlSeconds float64) {
	if cache == nil || ttlSeconds <= 0 {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.snapshot = cloneStatusSnapshot(snapshot)
	cache.expiresAt = now.UTC().Add(time.Duration(ttlSeconds * float64(time.Second)))
	cache.cached = true
}

// BuildStatusSnapshot applies ownership filters and count fields without side effects.
func BuildStatusSnapshot(input StatusSnapshotInput) StatusSnapshot {
	visible := cleanUniqueStrings(input.VisibleDeviceIDs)
	allowlist := cleanUniqueStrings(input.Allowlist)
	exclude := cleanUniqueStrings(input.Exclude)
	owned := FilterOwnedDeviceIDs(visible, allowlist, exclude)
	capturedAt := input.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	return StatusSnapshot{
		WorkerID:           input.WorkerID,
		VisibleDeviceIDs:   visible,
		OwnedDeviceIDs:     owned,
		Allowlist:          allowlist,
		Exclude:            exclude,
		VisibleDeviceCount: len(visible),
		OwnedDeviceCount:   len(owned),
		Backlog:            input.Backlog,
		CapturedAt:         capturedAt.UTC(),
	}
}

func cloneStatusSnapshot(snapshot StatusSnapshot) StatusSnapshot {
	snapshot.VisibleDeviceIDs = append([]string(nil), snapshot.VisibleDeviceIDs...)
	snapshot.OwnedDeviceIDs = append([]string(nil), snapshot.OwnedDeviceIDs...)
	snapshot.Allowlist = append([]string(nil), snapshot.Allowlist...)
	snapshot.Exclude = append([]string(nil), snapshot.Exclude...)
	if snapshot.Backlog.ByDevice != nil {
		byDevice := make(map[string]BacklogDeviceSummary, len(snapshot.Backlog.ByDevice))
		for deviceID, summary := range snapshot.Backlog.ByDevice {
			byDevice[deviceID] = summary
		}
		snapshot.Backlog.ByDevice = byDevice
	}
	return snapshot
}

func cleanUniqueStrings(values []string) []string {
	cleaned := cleanNonEmptyStrings(values)
	unique := make([]string, 0, len(cleaned))
	seen := map[string]struct{}{}
	for _, value := range cleaned {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

// HeartbeatDue reports whether a worker should write a heartbeat at now.
func HeartbeatDue(previous map[string]time.Time, workerID string, now time.Time, intervalSeconds float64) bool {
	normalizedWorkerID := strings.TrimSpace(workerID)
	if normalizedWorkerID == "" {
		normalizedWorkerID = "__default__"
	}
	previousAt, ok := previous[normalizedWorkerID]
	if !ok || previousAt.IsZero() {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.UTC().Sub(previousAt.UTC()).Seconds() >= intervalSeconds
}

// RememberHeartbeat returns previous heartbeat state with one worker timestamp updated.
func RememberHeartbeat(previous map[string]time.Time, workerID string, now time.Time) map[string]time.Time {
	if previous == nil {
		previous = map[string]time.Time{}
	}
	normalizedWorkerID := strings.TrimSpace(workerID)
	if normalizedWorkerID == "" {
		normalizedWorkerID = "__default__"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	previous[normalizedWorkerID] = now.UTC()
	return previous
}
