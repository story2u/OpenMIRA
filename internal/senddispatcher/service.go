package senddispatcher

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// TerminalUpdater writes terminal task status through the task service boundary.
type TerminalUpdater interface {
	UpdateTerminalStatus(ctx context.Context, taskID string, update tasks.StatusUpdate) (tasks.Record, error)
}

// ClaimNextFunc claims one durable SDK task through an injected persistence adapter.
type ClaimNextFunc func(ctx context.Context, request ClaimRequest) (tasks.Record, bool, error)

// ClaimBatchAfterFunc claims same-chat followups through an injected persistence adapter.
type ClaimBatchAfterFunc func(ctx context.Context, request BatchClaimRequest) ([]tasks.Record, error)

// ExecuteBatchFunc executes executor-ready send tasks through an injected SDK boundary.
type ExecuteBatchFunc func(ctx context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error)

// RecordHeartbeatFunc writes one send worker heartbeat through an injected adapter.
type RecordHeartbeatFunc func(ctx context.Context, record HeartbeatRecord) error

// ListDevicesFunc returns SDK-visible device ids for this dispatcher process.
type ListDevicesFunc func(ctx context.Context) ([]string, error)

// BacklogSummaryFunc returns accepted durable send backlog for owned devices.
type BacklogSummaryFunc func(ctx context.Context, ownedDeviceIDs []string) (BacklogSummary, error)

// ListRunningTasksFunc returns running tasks visible to stale recovery.
type ListRunningTasksFunc func(ctx context.Context) ([]tasks.Record, error)

// SDKDeviceIDResolver resolves SDK device aliases to canonical P1 device ids.
type SDKDeviceIDResolver interface {
	ResolveSDKDeviceID(ctx context.Context, deviceID string) (string, error)
}

// Service coordinates pre-SDK send dispatcher decisions.
type Service struct {
	Terminal                         TerminalUpdater
	ClaimNextTask                    ClaimNextFunc
	ClaimBatchAfterTask              ClaimBatchAfterFunc
	ExecuteBatch                     ExecuteBatchFunc
	RecordHeartbeat                  RecordHeartbeatFunc
	ListDevices                      ListDevicesFunc
	SummarizeBacklog                 BacklogSummaryFunc
	ListRunningTasks                 ListRunningTasksFunc
	DeviceLockStore                  DeviceLockStore
	DeviceIDResolver                 SDKDeviceIDResolver
	DeviceHealth                     SDKDeviceHealthRecorder
	DeviceHealthReader               SDKDeviceHealthReader
	TerminalSync                     TerminalStateSyncOptions
	NewDeviceLockNonce               func() string
	DeviceLockPID                    int
	DeviceLockExecutorTimeoutSeconds int
	DeviceLockSleep                  func(context.Context, time.Duration) error
	SnapshotCache                    *StatusSnapshotCache
	Env                              EnvLookup
	Hostname                         func() string
	Now                              func() time.Time
	MaxAgeSeconds                    func() float64
}

// ExecutorAdapterOptions returns the optional SDK executor adapter wiring for this service.
func (service Service) ExecutorAdapterOptions() SDKExecutorAdapterOptions {
	return SDKExecutorAdapterOptions{
		Now:          service.Now,
		StatusWriter: service.Terminal,
		DeviceHealth: service.DeviceHealth,
		Terminal:     service.TerminalSync,
		Env:          service.Env,
	}
}

// PreflightResult is the outcome of checking one claimed task before SDK execution.
type PreflightResult struct {
	Dispatchable bool
	Decision     TerminalDecision
	Record       tasks.Record
}

// ClaimPreflightResult is one claimed task after pre-SDK gates.
type ClaimPreflightResult struct {
	Claimed   bool
	Task      tasks.Record
	Preflight PreflightResult
}

// DispatchBatchResult is one claimed batch after pre-SDK terminal gates.
type DispatchBatchResult struct {
	Claimed          bool
	DeviceID         string
	ClaimedTasks     []tasks.Record
	Dispatchable     []tasks.Record
	Preflight        []PreflightResult
	ReuseKey         ReuseKey
	ReuseKeyPresent  bool
	ReuseCurrentChat bool
}

// DispatchExecutionResult is the outcome of sending an executor-ready batch.
type DispatchExecutionResult struct {
	Batch       DispatchBatchResult
	Finalized   []tasks.Record
	LastTargets map[string]ReuseKey
}

// RunOnceResult is one non-loop dispatcher pass.
type RunOnceResult struct {
	Claimed      bool
	ClaimedCount int
	Batch        DispatchBatchResult
	Execution    DispatchExecutionResult
	LastTargets  map[string]ReuseKey
}

// StickyRunResult is bounded same-chat continuation work after one dispatch.
type StickyRunResult struct {
	Rounds       int
	ClaimedCount int
	Executions   []DispatchExecutionResult
	LastTargets  map[string]ReuseKey
}

// HeartbeatRecord is the repository-neutral send worker heartbeat payload.
type HeartbeatRecord struct {
	WorkerID         string
	WorkerRole       string
	WorkerPool       string
	Hostname         string
	VisibleDeviceIDs []string
	OwnedDeviceIDs   []string
	LeaseTTLSeconds  float64
	Now              time.Time
	Metadata         map[string]any
}

// HeartbeatResult is the outcome of one heartbeat attempt.
type HeartbeatResult struct {
	Recorded bool
	Previous map[string]time.Time
	Record   HeartbeatRecord
}

// StaleRecoveryResult is one stale running watchdog pass.
type StaleRecoveryResult struct {
	Scanned   int
	Recovered int
	Records   []tasks.Record
}

// RecoveryLoopTickResult is one stale-running recovery loop body without sleeping.
type RecoveryLoopTickResult struct {
	Recovery  StaleRecoveryResult
	NextDelay time.Duration
}

// ApplyPreflight marks timeout/cancelled tasks before they reach SDK execution.
func (service Service) ApplyPreflight(ctx context.Context, task tasks.Record) (PreflightResult, error) {
	decision, terminal := service.preflightTerminalDecision(ctx, task)
	if !terminal {
		return PreflightResult{Dispatchable: true, Record: task}, nil
	}
	if service.Terminal == nil {
		return PreflightResult{}, fmt.Errorf("task terminal updater is not configured")
	}
	errorText := decision.Error
	record, err := service.Terminal.UpdateTerminalStatus(ctx, task.TaskID, tasks.StatusUpdate{
		Status: decision.Status,
		Error:  &errorText,
	})
	if err != nil {
		return PreflightResult{}, err
	}
	service.syncPreflightTerminalState(ctx, record, decision)
	return PreflightResult{Dispatchable: false, Decision: decision, Record: record}, nil
}

func (service Service) preflightTerminalDecision(ctx context.Context, task tasks.Record) (TerminalDecision, bool) {
	if decision, terminal := PreflightTerminalDecision(task, service.now(), service.maxAgeSeconds()); terminal {
		return decision, true
	}
	if service.DeviceHealthReader == nil {
		return TerminalDecision{}, false
	}
	deviceID := strings.TrimSpace(task.Target.DeviceID)
	if deviceID == "" {
		return TerminalDecision{}, false
	}
	failure, err := service.DeviceHealthReader.GetRecentSDKDeviceTransportFailure(ctx, deviceID)
	if err == nil {
		if decision, terminal := RecentSDKTransportFailureDecision(task, failure); terminal {
			return decision, true
		}
	}
	state, err := service.DeviceHealthReader.GetRecentSDKDeviceUIUnstableState(ctx, deviceID)
	if err != nil {
		return TerminalDecision{}, false
	}
	return SlowQueueUIUnstableCooldownDecision(task, state)
}

func (service Service) syncPreflightTerminalState(ctx context.Context, record tasks.Record, decision TerminalDecision) {
	if service.TerminalSync.Delivery == nil && service.TerminalSync.Revoke == nil && service.TerminalSync.Status == nil && service.TerminalSync.AI == nil {
		return
	}
	options := service.TerminalSync
	if service.Terminal != nil {
		options.Delivery = nil
		options.Revoke = nil
	}
	options.ResultPayload = decision.ResultPayload
	_ = SyncSDKTerminalState(ctx, record, options)
}

// ClaimNext builds a Python-compatible claim request and delegates persistence.
func (service Service) ClaimNext(ctx context.Context, workerID string, deviceIDs []string) (tasks.Record, bool, error) {
	request, ok := BuildClaimRequest(workerID, deviceIDs, service.now())
	if !ok || service.ClaimNextTask == nil {
		return tasks.Record{}, false, nil
	}
	return service.ClaimNextTask(ctx, request)
}

// ClaimAndPreflightNext claims one task and applies pre-SDK terminal gates.
func (service Service) ClaimAndPreflightNext(ctx context.Context, workerID string, deviceIDs []string) (ClaimPreflightResult, error) {
	claimed, ok, err := service.ClaimNext(ctx, workerID, deviceIDs)
	if err != nil || !ok {
		return ClaimPreflightResult{}, err
	}
	preflight, err := service.ApplyPreflight(ctx, claimed)
	if err != nil {
		return ClaimPreflightResult{}, err
	}
	return ClaimPreflightResult{Claimed: true, Task: claimed, Preflight: preflight}, nil
}

// ClaimAndPreflightBatch claims one task, same-chat followups, and returns executor-ready records.
func (service Service) ClaimAndPreflightBatch(ctx context.Context, workerID string, deviceIDs []string, maxSize int) (DispatchBatchResult, error) {
	first, err := service.ClaimAndPreflightNext(ctx, workerID, deviceIDs)
	if err != nil || !first.Claimed {
		return DispatchBatchResult{}, err
	}
	result := DispatchBatchResult{
		Claimed:  true,
		DeviceID: strings.TrimSpace(first.Task.Target.DeviceID),
	}
	if !first.Preflight.Dispatchable {
		result.appendPreflight(first.Preflight)
		return result, nil
	}

	if maxSize <= 0 {
		maxSize = BatchMaxSize(nil)
	}
	batch := []tasks.Record{first.Preflight.Record}
	if maxSize > 1 && BatchableTask(first.Preflight.Record) {
		followups, err := service.ClaimBatchAfter(ctx, first.Preflight.Record, workerID, maxSize-1, false)
		if err != nil {
			return DispatchBatchResult{}, err
		}
		batch = appendUniqueTasks(batch, followups)
	}
	batch = OrderClaimedBatch(batch)
	for _, record := range batch {
		preflight := PreflightResult{}
		if strings.TrimSpace(record.TaskID) == strings.TrimSpace(first.Task.TaskID) {
			preflight = first.Preflight
		} else {
			var err error
			preflight, err = service.ApplyPreflight(ctx, record)
			if err != nil {
				return DispatchBatchResult{}, err
			}
		}
		result.appendPreflight(preflight)
	}
	return result, nil
}

// ClaimAndPreflightStickyFollowups claims same-chat work after a dispatched anchor.
func (service Service) ClaimAndPreflightStickyFollowups(ctx context.Context, anchor tasks.Record, workerID string, maxSize int) (DispatchBatchResult, error) {
	result := DispatchBatchResult{DeviceID: strings.TrimSpace(anchor.Target.DeviceID)}
	if maxSize <= 0 {
		maxSize = BatchMaxSize(nil)
	}
	if maxSize <= 0 || !BatchableTask(anchor) {
		return result, nil
	}
	followups, err := service.ClaimBatchAfter(ctx, anchor, workerID, maxSize, true)
	if err != nil {
		return DispatchBatchResult{}, err
	}
	if len(followups) == 0 {
		return result, nil
	}
	batch := appendUniqueTasks([]tasks.Record{anchor}, followups)
	batch = OrderClaimedBatch(batch)
	for _, record := range batch {
		if strings.TrimSpace(record.TaskID) == strings.TrimSpace(anchor.TaskID) {
			continue
		}
		preflight, err := service.ApplyPreflight(ctx, record)
		if err != nil {
			return DispatchBatchResult{}, err
		}
		result.appendPreflight(preflight)
	}
	result.Claimed = len(result.Preflight) > 0
	return result, nil
}

// ApplyCurrentChatReuse marks executor-ready records when current chat can be reused.
func (service Service) ApplyCurrentChatReuse(result DispatchBatchResult, lastTargets map[string]ReuseKey, force bool) DispatchBatchResult {
	key, ok := BatchReuseKey(result.Dispatchable)
	result.ReuseKey = key
	result.ReuseKeyPresent = ok
	if len(result.Dispatchable) == 0 {
		return result
	}
	if force || ShouldReuseCurrentChat(lastTargets, result.DeviceID, key, ok) {
		result.ReuseCurrentChat = true
		result.Dispatchable = MarkReuseCurrentChat(result.Dispatchable)
	}
	return result
}

// ExecutePreparedBatch sends dispatchable records through the injected executor boundary.
func (service Service) ExecutePreparedBatch(ctx context.Context, result DispatchBatchResult, lastTargets map[string]ReuseKey, forceReuse bool) (DispatchExecutionResult, error) {
	prepared := service.ApplyCurrentChatReuse(result, lastTargets, forceReuse)
	if len(prepared.Dispatchable) == 0 {
		return DispatchExecutionResult{Batch: prepared, LastTargets: lastTargets}, nil
	}
	if service.ExecuteBatch == nil {
		return DispatchExecutionResult{}, fmt.Errorf("send batch executor is not configured")
	}
	deviceID := strings.TrimSpace(prepared.DeviceID)
	if deviceID == "" {
		return DispatchExecutionResult{}, fmt.Errorf("send batch device_id is required")
	}
	lockState := DeviceLockState{}
	if service.DeviceLockStore != nil {
		lockTaskID := ""
		if len(prepared.Dispatchable) > 0 {
			lockTaskID = prepared.Dispatchable[0].TaskID
		}
		lockDeviceID := service.resolveSDKDeviceID(ctx, deviceID)
		acquiredLock, err := AcquireDeviceLock(ctx, service.DeviceLockStore, DeviceLockWaitOptions{
			Request: DeviceLockRequest{
				DeviceID:               lockDeviceID,
				TaskID:                 lockTaskID,
				Nonce:                  service.deviceLockNonce(),
				PID:                    service.DeviceLockPID,
				ExecutorTimeoutSeconds: service.DeviceLockExecutorTimeoutSeconds,
				Env:                    service.Env,
			},
			Now:   service.now,
			Sleep: service.DeviceLockSleep,
		})
		if err != nil {
			return DispatchExecutionResult{}, err
		}
		lockState = acquiredLock
		if lockState.Acquired {
			defer func() {
				_ = ReleaseDeviceLock(ctx, service.DeviceLockStore, lockState)
			}()
		}
	}
	finalized, err := service.ExecuteBatch(ctx, deviceID, prepared.Dispatchable)
	if err != nil {
		return DispatchExecutionResult{}, err
	}
	updatedTargets := RememberLastSendTarget(lastTargets, deviceID, prepared.ReuseKey, prepared.ReuseKeyPresent, finalized)
	return DispatchExecutionResult{Batch: prepared, Finalized: finalized, LastTargets: updatedTargets}, nil
}

// RunOnce claims and executes one initial durable SDK batch without starting a loop.
func (service Service) RunOnce(ctx context.Context, workerID string, deviceIDs []string, maxSize int, lastTargets map[string]ReuseKey) (RunOnceResult, error) {
	batch, err := service.ClaimAndPreflightBatch(ctx, workerID, deviceIDs, maxSize)
	if err != nil || !batch.Claimed {
		return RunOnceResult{LastTargets: lastTargets}, err
	}
	return service.executeClaimedBatch(ctx, batch, lastTargets)
}

// RunOnceWithActiveSet claims and executes one batch while coordinating local device lanes.
func (service Service) RunOnceWithActiveSet(ctx context.Context, workerID string, snapshot StatusSnapshot, activeSet *ActiveDeviceSet, maxSize int, lastTargets map[string]ReuseKey) (RunOnceResult, error) {
	if activeSet == nil {
		return service.RunOnce(ctx, workerID, snapshot.OwnedDeviceIDs, maxSize, lastTargets)
	}
	activeSet.mutex.Lock()
	idleDeviceIDs := activeSet.idleDevicesLocked(snapshot.OwnedDeviceIDs)
	batch, err := service.ClaimAndPreflightBatch(ctx, workerID, idleDeviceIDs, maxSize)
	if err != nil || !batch.Claimed {
		activeSet.mutex.Unlock()
		return RunOnceResult{LastTargets: lastTargets}, err
	}
	deviceID := strings.TrimSpace(batch.DeviceID)
	if deviceID != "" {
		if !activeSet.markActiveLocked(deviceID) {
			activeSet.mutex.Unlock()
			return RunOnceResult{}, fmt.Errorf("claimed device %q is already active", deviceID)
		}
	}
	activeSet.mutex.Unlock()
	if deviceID != "" {
		defer activeSet.Release(deviceID)
	}
	return service.executeClaimedBatch(ctx, batch, lastTargets)
}

func (service Service) executeClaimedBatch(ctx context.Context, batch DispatchBatchResult, lastTargets map[string]ReuseKey) (RunOnceResult, error) {
	result := RunOnceResult{
		Claimed:      true,
		ClaimedCount: len(batch.ClaimedTasks),
		Batch:        batch,
		LastTargets:  lastTargets,
	}
	if len(batch.Dispatchable) == 0 {
		return result, nil
	}
	execution, err := service.ExecutePreparedBatch(ctx, batch, lastTargets, false)
	if err != nil {
		return RunOnceResult{}, err
	}
	result.Execution = execution
	result.LastTargets = execution.LastTargets
	return result, nil
}

// RunStickyFollowups executes bounded same-chat continuation batches.
func (service Service) RunStickyFollowups(ctx context.Context, anchor tasks.Record, workerID string, maxSize int, rounds int, lastTargets map[string]ReuseKey) (StickyRunResult, error) {
	result := StickyRunResult{LastTargets: lastTargets}
	if rounds <= 0 {
		return result, nil
	}
	for round := 0; round < rounds; round++ {
		batch, err := service.ClaimAndPreflightStickyFollowups(ctx, anchor, workerID, maxSize)
		if err != nil {
			return StickyRunResult{}, err
		}
		if !batch.Claimed {
			break
		}
		execution, err := service.ExecutePreparedBatch(ctx, batch, result.LastTargets, true)
		if err != nil {
			return StickyRunResult{}, err
		}
		result.Rounds++
		result.ClaimedCount += len(batch.ClaimedTasks)
		result.Executions = append(result.Executions, execution)
		result.LastTargets = execution.LastTargets
	}
	return result, nil
}

// CaptureStatusSnapshot builds one ownership snapshot through injected runtime readers.
func (service Service) CaptureStatusSnapshot(ctx context.Context, workerID string) (StatusSnapshot, error) {
	resolvedWorkerID := strings.TrimSpace(workerID)
	if resolvedWorkerID == "" {
		resolvedWorkerID = WorkerID(service.Env, 0)
	}
	now := service.now()
	cacheTTLSeconds := StatusSnapshotCacheTTLSeconds(service.Env)
	if service.SnapshotCache != nil && cacheTTLSeconds > 0 {
		if snapshot, ok := service.SnapshotCache.Get(resolvedWorkerID, now); ok {
			return snapshot, nil
		}
	}
	snapshot, err := service.captureStatusSnapshotAt(ctx, resolvedWorkerID, now)
	if err != nil {
		return StatusSnapshot{}, err
	}
	if service.SnapshotCache != nil && cacheTTLSeconds > 0 {
		service.SnapshotCache.Put(snapshot, now, cacheTTLSeconds)
	}
	return snapshot, nil
}

func (service Service) captureStatusSnapshotAt(ctx context.Context, workerID string, now time.Time) (StatusSnapshot, error) {
	visibleDeviceIDs := []string{}
	if service.ListDevices != nil {
		listed, err := service.ListDevices(ctx)
		if err != nil {
			return StatusSnapshot{}, err
		}
		visibleDeviceIDs = listed
	}
	allowlist := DeviceAllowlist(service.Env)
	exclude := DeviceExcludeList(service.Env)
	ownedDeviceIDs := FilterOwnedDeviceIDs(visibleDeviceIDs, allowlist, exclude)
	backlog := BacklogSummary{ByDevice: map[string]BacklogDeviceSummary{}}
	if service.SummarizeBacklog != nil && len(ownedDeviceIDs) > 0 {
		summary, err := service.SummarizeBacklog(ctx, ownedDeviceIDs)
		if err != nil {
			return StatusSnapshot{}, err
		}
		backlog = summary
	}
	return BuildStatusSnapshot(StatusSnapshotInput{
		WorkerID:         workerID,
		VisibleDeviceIDs: visibleDeviceIDs,
		Allowlist:        allowlist,
		Exclude:          exclude,
		Backlog:          backlog,
		CapturedAt:       now,
	}), nil
}

// RecordStatusHeartbeat writes one worker heartbeat when the throttle says it is due.
func (service Service) RecordStatusHeartbeat(ctx context.Context, snapshot StatusSnapshot, previous map[string]time.Time, now time.Time) (HeartbeatResult, error) {
	if now.IsZero() {
		now = service.now()
	} else {
		now = now.UTC()
	}
	workerID := strings.TrimSpace(snapshot.WorkerID)
	if workerID == "" {
		workerID = WorkerID(service.Env, 0)
	}
	result := HeartbeatResult{Previous: previous}
	if service.RecordHeartbeat == nil || !HeartbeatDue(previous, workerID, now, HeartbeatIntervalSeconds(service.Env)) {
		return result, nil
	}
	record := HeartbeatRecord{
		WorkerID:         workerID,
		WorkerRole:       WorkerRole(service.Env),
		WorkerPool:       WorkerPool(service.Env),
		Hostname:         WorkerHostname(service.Env, service.hostname()),
		VisibleDeviceIDs: append([]string(nil), snapshot.VisibleDeviceIDs...),
		OwnedDeviceIDs:   append([]string(nil), snapshot.OwnedDeviceIDs...),
		LeaseTTLSeconds:  WorkerLeaseTTLSeconds(service.Env),
		Now:              now,
		Metadata: map[string]any{
			"runtime":   "sdk_dispatcher",
			"allowlist": append([]string(nil), snapshot.Allowlist...),
			"exclude":   append([]string(nil), snapshot.Exclude...),
			"backlog":   snapshot.Backlog,
		},
	}
	if err := service.RecordHeartbeat(ctx, record); err != nil {
		return HeartbeatResult{}, err
	}
	result.Recorded = true
	result.Record = record
	result.Previous = RememberHeartbeat(previous, workerID, now)
	return result, nil
}

// RecoverStaleRunningTasks closes running SDK tasks left behind by a dead worker.
func (service Service) RecoverStaleRunningTasks(ctx context.Context, now time.Time, timeoutSeconds int) (StaleRecoveryResult, error) {
	result := StaleRecoveryResult{}
	if service.ListRunningTasks == nil {
		return result, nil
	}
	if now.IsZero() {
		now = service.now()
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = RunningTaskRecoveryTimeoutSeconds(service.Env)
	}
	records, err := service.ListRunningTasks(ctx)
	if err != nil {
		return result, err
	}
	result.Scanned = len(records)
	for _, record := range records {
		decision, terminal := StaleRunningTerminalDecision(record, now, timeoutSeconds)
		if !terminal {
			continue
		}
		if service.Terminal == nil {
			return result, fmt.Errorf("task terminal updater is not configured")
		}
		errorText := decision.Error
		updated, err := service.Terminal.UpdateTerminalStatus(ctx, record.TaskID, tasks.StatusUpdate{
			Status: decision.Status,
			Error:  &errorText,
		})
		if err != nil {
			return result, err
		}
		result.Recovered++
		result.Records = append(result.Records, updated)
	}
	return result, nil
}

// RunRecoveryTick executes one stale-running recovery loop body.
func (service Service) RunRecoveryTick(ctx context.Context) (RecoveryLoopTickResult, error) {
	nextDelay := time.Duration(RunningTaskRecoveryIntervalSeconds(service.Env) * float64(time.Second))
	recovery, err := service.RecoverStaleRunningTasks(ctx, service.now(), 0)
	return RecoveryLoopTickResult{Recovery: recovery, NextDelay: nextDelay}, err
}

// ClaimBatchAfter builds a followup claim request and delegates persistence.
func (service Service) ClaimBatchAfter(ctx context.Context, firstTask tasks.Record, workerID string, maxSize int, skipInterleaved bool) ([]tasks.Record, error) {
	request, ok := BuildBatchClaimRequest(firstTask, workerID, maxSize, skipInterleaved, service.now())
	if !ok || service.ClaimBatchAfterTask == nil {
		return []tasks.Record{}, nil
	}
	return service.ClaimBatchAfterTask(ctx, request)
}

func (result *DispatchBatchResult) appendPreflight(preflight PreflightResult) {
	result.Preflight = append(result.Preflight, preflight)
	result.ClaimedTasks = append(result.ClaimedTasks, preflight.Record)
	if preflight.Dispatchable {
		result.Dispatchable = append(result.Dispatchable, preflight.Record)
	}
}

func appendUniqueTasks(records []tasks.Record, additions []tasks.Record) []tasks.Record {
	seen := make(map[string]struct{}, len(records)+len(additions))
	for _, record := range records {
		taskID := strings.TrimSpace(record.TaskID)
		if taskID != "" {
			seen[taskID] = struct{}{}
		}
	}
	for _, record := range additions {
		taskID := strings.TrimSpace(record.TaskID)
		if taskID != "" {
			if _, ok := seen[taskID]; ok {
				continue
			}
			seen[taskID] = struct{}{}
		}
		records = append(records, record)
	}
	return records
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) deviceLockNonce() string {
	if service.NewDeviceLockNonce != nil {
		return strings.TrimSpace(service.NewDeviceLockNonce())
	}
	return NewDeviceLockNonce()
}

func (service Service) resolveSDKDeviceID(ctx context.Context, deviceID string) string {
	normalized := strings.TrimSpace(deviceID)
	if normalized == "" || service.DeviceIDResolver == nil {
		return normalized
	}
	resolved, err := service.DeviceIDResolver.ResolveSDKDeviceID(ctx, normalized)
	if err != nil {
		return normalized
	}
	if canonical := strings.TrimSpace(resolved); canonical != "" {
		return canonical
	}
	return normalized
}

func (service Service) hostname() string {
	if service.Hostname != nil {
		return service.Hostname()
	}
	hostname, _ := os.Hostname()
	return hostname
}

func (service Service) maxAgeSeconds() float64 {
	if service.MaxAgeSeconds != nil {
		return service.MaxAgeSeconds()
	}
	return MaxAcceptedAgeSeconds(nil)
}
