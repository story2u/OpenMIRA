package senddispatcher

import (
	"context"
	"sync"
	"time"
)

// RuntimeState keeps in-process dispatcher state that Python stored on ctx.
type RuntimeState struct {
	mutex         sync.Mutex
	ActiveDevices *ActiveDeviceSet
	SnapshotCache *StatusSnapshotCache
	LastTargets   map[string]ReuseKey
	LastHeartbeat map[string]time.Time
	LatestStatus  StatusSnapshot
	hasStatus     bool
}

// ClaimStatusResult is the pre-claim snapshot and heartbeat outcome.
type ClaimStatusResult struct {
	Snapshot       StatusSnapshot
	Heartbeat      HeartbeatResult
	HeartbeatError error
}

// RuntimeRunOnceResult is one stateful non-loop dispatcher pass.
type RuntimeRunOnceResult struct {
	Status ClaimStatusResult
	Run    RunOnceResult
	Sticky StickyRunResult
}

// RuntimeLoopTickResult is one dispatcher loop body without sleeping.
type RuntimeLoopTickResult struct {
	Run             RuntimeRunOnceResult
	DispatchedCount int
	NextDelay       time.Duration
}

// NewRuntimeState creates an empty local dispatcher runtime state.
func NewRuntimeState() *RuntimeState {
	return &RuntimeState{
		ActiveDevices: NewActiveDeviceSet(),
		SnapshotCache: NewStatusSnapshotCache(),
		LastTargets:   map[string]ReuseKey{},
		LastHeartbeat: map[string]time.Time{},
	}
}

// RunOnce captures runtime status and executes one non-loop dispatcher pass.
func (state *RuntimeState) RunOnce(ctx context.Context, service Service, workerID string) (RuntimeRunOnceResult, error) {
	if state == nil {
		state = NewRuntimeState()
	}
	status, err := state.CaptureStatusSnapshotForClaim(ctx, service, workerID)
	if err != nil {
		return RuntimeRunOnceResult{}, err
	}
	activeDevices := state.ensureActiveDevices()
	lastTargets := state.lastTargetsSnapshot()
	run, err := service.RunOnceWithActiveSet(ctx, status.Snapshot.WorkerID, status.Snapshot, activeDevices, BatchMaxSize(service.Env), lastTargets)
	if err != nil {
		return RuntimeRunOnceResult{Status: status}, err
	}
	state.StoreLastTargets(run.LastTargets)
	result := RuntimeRunOnceResult{Status: status, Run: run}
	if run.Claimed && len(run.Batch.Dispatchable) > 0 {
		sticky, err := service.RunStickyFollowups(ctx, run.Batch.Dispatchable[0], status.Snapshot.WorkerID, BatchMaxSize(service.Env), StickyMaxRounds(service.Env), run.LastTargets)
		if err != nil {
			return result, err
		}
		result.Sticky = sticky
		state.StoreLastTargets(sticky.LastTargets)
	}
	return result, nil
}

// RunLoopTick executes one loop body and returns the caller-controlled delay.
func (state *RuntimeState) RunLoopTick(ctx context.Context, service Service, workerID string) (RuntimeLoopTickResult, error) {
	run, err := state.RunOnce(ctx, service, workerID)
	if err != nil {
		return RuntimeLoopTickResult{
			Run:       run,
			NextDelay: pollIntervalDuration(service.Env),
		}, err
	}
	dispatchedCount := run.Run.ClaimedCount + run.Sticky.ClaimedCount
	return RuntimeLoopTickResult{
		Run:             run,
		DispatchedCount: dispatchedCount,
		NextDelay:       nextLoopDelay(service.Env, dispatchedCount > 0),
	}, nil
}

// CaptureStatusSnapshotForClaim captures, stores, and heartbeats before task claim.
func (state *RuntimeState) CaptureStatusSnapshotForClaim(ctx context.Context, service Service, workerID string) (ClaimStatusResult, error) {
	if state == nil {
		state = NewRuntimeState()
	}
	snapshotCache := state.ensureSnapshotCache()
	if service.SnapshotCache == nil {
		service.SnapshotCache = snapshotCache
	}
	snapshot, err := service.CaptureStatusSnapshot(ctx, workerID)
	if err != nil {
		return ClaimStatusResult{}, err
	}
	state.StoreStatusSnapshot(snapshot)

	previousHeartbeat := state.heartbeatSnapshot()
	heartbeat, heartbeatErr := service.RecordStatusHeartbeat(ctx, snapshot, previousHeartbeat, service.now())
	if heartbeatErr == nil {
		state.StoreHeartbeatState(heartbeat.Previous)
	}
	return ClaimStatusResult{
		Snapshot:       snapshot,
		Heartbeat:      heartbeat,
		HeartbeatError: heartbeatErr,
	}, nil
}

// StoreStatusSnapshot records the latest dispatcher status snapshot.
func (state *RuntimeState) StoreStatusSnapshot(snapshot StatusSnapshot) {
	if state == nil {
		return
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	state.LatestStatus = cloneStatusSnapshot(snapshot)
	state.hasStatus = true
}

// LatestStatusSnapshot returns the latest stored dispatcher status snapshot.
func (state *RuntimeState) LatestStatusSnapshot() (StatusSnapshot, bool) {
	if state == nil {
		return StatusSnapshot{}, false
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if !state.hasStatus {
		return StatusSnapshot{}, false
	}
	return cloneStatusSnapshot(state.LatestStatus), true
}

// StoreHeartbeatState replaces the per-worker heartbeat throttle state.
func (state *RuntimeState) StoreHeartbeatState(previous map[string]time.Time) {
	if state == nil || previous == nil {
		return
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	state.LastHeartbeat = cloneHeartbeatState(previous)
}

// StoreLastTargets replaces the per-device current-chat reuse memory.
func (state *RuntimeState) StoreLastTargets(lastTargets map[string]ReuseKey) {
	if state == nil || lastTargets == nil {
		return
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	state.LastTargets = cloneLastTargets(lastTargets)
}

func (state *RuntimeState) ensureActiveDevices() *ActiveDeviceSet {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if state.ActiveDevices == nil {
		state.ActiveDevices = NewActiveDeviceSet()
	}
	return state.ActiveDevices
}

func (state *RuntimeState) ensureSnapshotCache() *StatusSnapshotCache {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if state.ActiveDevices == nil {
		state.ActiveDevices = NewActiveDeviceSet()
	}
	if state.SnapshotCache == nil {
		state.SnapshotCache = NewStatusSnapshotCache()
	}
	if state.LastTargets == nil {
		state.LastTargets = map[string]ReuseKey{}
	}
	if state.LastHeartbeat == nil {
		state.LastHeartbeat = map[string]time.Time{}
	}
	return state.SnapshotCache
}

func (state *RuntimeState) heartbeatSnapshot() map[string]time.Time {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if state.LastHeartbeat == nil {
		state.LastHeartbeat = map[string]time.Time{}
	}
	return cloneHeartbeatState(state.LastHeartbeat)
}

func (state *RuntimeState) lastTargetsSnapshot() map[string]ReuseKey {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if state.LastTargets == nil {
		state.LastTargets = map[string]ReuseKey{}
	}
	return cloneLastTargets(state.LastTargets)
}

func cloneLastTargets(lastTargets map[string]ReuseKey) map[string]ReuseKey {
	if lastTargets == nil {
		return nil
	}
	cloned := make(map[string]ReuseKey, len(lastTargets))
	for deviceID, key := range lastTargets {
		cloned[deviceID] = key
	}
	return cloned
}

func cloneHeartbeatState(previous map[string]time.Time) map[string]time.Time {
	if previous == nil {
		return nil
	}
	cloned := make(map[string]time.Time, len(previous))
	for workerID, previousAt := range previous {
		cloned[workerID] = previousAt
	}
	return cloned
}

func nextLoopDelay(lookup EnvLookup, dispatched bool) time.Duration {
	if dispatched {
		return 0
	}
	return pollIntervalDuration(lookup)
}

func pollIntervalDuration(lookup EnvLookup) time.Duration {
	return time.Duration(PollIntervalSeconds(lookup) * float64(time.Second))
}
