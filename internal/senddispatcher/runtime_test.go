package senddispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestRuntimeStateCaptureStatusSnapshotForClaimStoresAndHeartbeats mirrors Python pre-claim status handling.
func TestRuntimeStateCaptureStatusSnapshotForClaimStoresAndHeartbeats(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	var captured HeartbeatRecord
	state := NewRuntimeState()
	service := Service{
		Now: func() time.Time { return now },
		Env: mapLookup(map[string]string{
			"SEND_WORKER_HEARTBEAT_INTERVAL_SEC": "10",
			"SEND_WORKER_HOSTNAME":               "host-a",
		}),
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		RecordHeartbeat: func(_ context.Context, record HeartbeatRecord) error {
			captured = record
			return nil
		},
	}
	result, err := state.CaptureStatusSnapshotForClaim(context.Background(), service, " worker-a ")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshotForClaim returned error: %v", err)
	}
	if result.Snapshot.WorkerID != "worker-a" || !result.Heartbeat.Recorded {
		t.Fatalf("result = %#v", result)
	}
	if captured.WorkerID != "worker-a" || captured.Hostname != "host-a" || len(captured.OwnedDeviceIDs) != 1 {
		t.Fatalf("captured heartbeat = %#v", captured)
	}
	latest, ok := state.LatestStatusSnapshot()
	if !ok || latest.WorkerID != "worker-a" || latest.OwnedDeviceIDs[0] != "zimo" {
		t.Fatalf("latest status = %#v ok=%t", latest, ok)
	}
	if _, ok := state.LastHeartbeat["worker-a"]; !ok {
		t.Fatalf("heartbeat state = %#v", state.LastHeartbeat)
	}
}

// TestRuntimeStateCaptureStatusSnapshotForClaimUsesCacheAndHeartbeatThrottle keeps hot polling cheap.
func TestRuntimeStateCaptureStatusSnapshotForClaimUsesCacheAndHeartbeatThrottle(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	listCalls := 0
	heartbeatCalls := 0
	state := NewRuntimeState()
	service := Service{
		Now: func() time.Time { return now },
		Env: mapLookup(map[string]string{
			"P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC": "10",
			"SEND_WORKER_HEARTBEAT_INTERVAL_SEC":        "10",
		}),
		ListDevices: func(context.Context) ([]string, error) {
			listCalls++
			return []string{"zimo"}, nil
		},
		RecordHeartbeat: func(context.Context, HeartbeatRecord) error {
			heartbeatCalls++
			return nil
		},
	}
	first, err := state.CaptureStatusSnapshotForClaim(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("first CaptureStatusSnapshotForClaim returned error: %v", err)
	}
	now = now.Add(5 * time.Second)
	second, err := state.CaptureStatusSnapshotForClaim(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("second CaptureStatusSnapshotForClaim returned error: %v", err)
	}
	if listCalls != 1 || heartbeatCalls != 1 {
		t.Fatalf("calls list=%d heartbeat=%d", listCalls, heartbeatCalls)
	}
	if !second.Snapshot.CapturedAt.Equal(first.Snapshot.CapturedAt) || second.Heartbeat.Recorded {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

// TestRuntimeStateCaptureStatusSnapshotForClaimKeepsPathOnHeartbeatError mirrors Python best-effort heartbeat.
func TestRuntimeStateCaptureStatusSnapshotForClaimKeepsPathOnHeartbeatError(t *testing.T) {
	heartbeatErr := errors.New("heartbeat failed")
	state := NewRuntimeState()
	service := Service{
		Now: func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) },
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		RecordHeartbeat: func(context.Context, HeartbeatRecord) error {
			return heartbeatErr
		},
	}
	result, err := state.CaptureStatusSnapshotForClaim(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshotForClaim returned error: %v", err)
	}
	if !errors.Is(result.HeartbeatError, heartbeatErr) || result.Snapshot.WorkerID != "worker-a" {
		t.Fatalf("result = %#v", result)
	}
	latest, ok := state.LatestStatusSnapshot()
	if !ok || latest.WorkerID != "worker-a" {
		t.Fatalf("latest status = %#v ok=%t", latest, ok)
	}
	if len(state.LastHeartbeat) != 0 {
		t.Fatalf("heartbeat state updated after error: %#v", state.LastHeartbeat)
	}
}

// TestRuntimeStateRunOnceClaimsOnlyIdleOwnedDevice connects snapshot, active set, and execution.
func TestRuntimeStateRunOnceClaimsOnlyIdleOwnedDevice(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	state := NewRuntimeState()
	if !state.ActiveDevices.TryAcquire("zimo") {
		t.Fatal("failed to mark zimo busy")
	}
	var capturedRequest ClaimRequest
	var capturedDevice string
	service := Service{
		Now: func() time.Time { return now },
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo", "ada"}, nil
		},
		ClaimNextTask: func(_ context.Context, request ClaimRequest) (tasks.Record, bool, error) {
			capturedRequest = request
			record := dispatchBatchRecord("task-0", 0, now)
			record.Target.DeviceID = "ada"
			return record, true, nil
		},
		ExecuteBatch: func(_ context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
			capturedDevice = deviceID
			return []tasks.Record{{TaskID: records[0].TaskID, Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := state.RunOnce(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Run.Claimed || capturedDevice != "ada" {
		t.Fatalf("result=%#v capturedDevice=%q", result, capturedDevice)
	}
	if len(capturedRequest.DeviceIDs) != 1 || capturedRequest.DeviceIDs[0] != "ada" {
		t.Fatalf("captured request = %#v", capturedRequest)
	}
	if _, ok := state.LastTargets["ada"]; !ok {
		t.Fatalf("last targets = %#v", state.LastTargets)
	}
	if !state.ActiveDevices.TryAcquire("ada") {
		t.Fatal("ada was not released after runtime pass")
	}
}

// TestRuntimeStateRunOnceCarriesLastTargetReuse mirrors per-device current-chat memory.
func TestRuntimeStateRunOnceCarriesLastTargetReuse(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	state := NewRuntimeState()
	claimCalls := 0
	var reuseFlags []any
	service := Service{
		Now: func() time.Time { return now },
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			claimCalls++
			return dispatchBatchRecord("task-"+string(rune('0'+claimCalls)), claimCalls, now), true, nil
		},
		ExecuteBatch: func(_ context.Context, _ string, records []tasks.Record) ([]tasks.Record, error) {
			reuseFlags = append(reuseFlags, records[0].Payload["_reuse_current_chat"])
			return []tasks.Record{{TaskID: records[0].TaskID, Status: tasks.StatusSuccess}}, nil
		},
	}
	if _, err := state.RunOnce(context.Background(), service, "worker-a"); err != nil {
		t.Fatalf("first RunOnce returned error: %v", err)
	}
	if _, err := state.RunOnce(context.Background(), service, "worker-a"); err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	if len(reuseFlags) != 2 || reuseFlags[0] == true || reuseFlags[1] != true {
		t.Fatalf("reuse flags = %#v", reuseFlags)
	}
	if _, ok := state.LastTargets["zimo"]; !ok {
		t.Fatalf("last targets = %#v", state.LastTargets)
	}
}

// TestRuntimeStateRunOnceRunsStickyFollowups mirrors bounded same-chat continuation.
func TestRuntimeStateRunOnceRunsStickyFollowups(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	state := NewRuntimeState()
	claimBatchCalls := 0
	var reuseFlags []any
	service := Service{
		Now: func() time.Time { return now },
		Env: mapLookup(map[string]string{
			"P1_SDK_DEVICE_BATCH_MAX_SIZE":    "1",
			"P1_SDK_DEVICE_STICKY_MAX_ROUNDS": "1",
		}),
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-0", 0, now), true, nil
		},
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			claimBatchCalls++
			return []tasks.Record{dispatchBatchRecord("task-1", 1, now)}, nil
		},
		ExecuteBatch: func(_ context.Context, _ string, records []tasks.Record) ([]tasks.Record, error) {
			reuseFlags = append(reuseFlags, records[0].Payload["_reuse_current_chat"])
			return []tasks.Record{{TaskID: records[0].TaskID, Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := state.RunOnce(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Run.ClaimedCount != 1 || result.Sticky.Rounds != 1 || result.Sticky.ClaimedCount != 1 || claimBatchCalls != 1 {
		t.Fatalf("result=%#v claimBatchCalls=%d", result, claimBatchCalls)
	}
	if len(reuseFlags) != 2 || reuseFlags[0] == true || reuseFlags[1] != true {
		t.Fatalf("reuse flags = %#v", reuseFlags)
	}
}

// TestRuntimeStateRunLoopTickDelaysWhenIdle mirrors Python empty-claim sleep.
func TestRuntimeStateRunLoopTickDelaysWhenIdle(t *testing.T) {
	state := NewRuntimeState()
	service := Service{
		Now: func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) },
		Env: mapLookup(map[string]string{"P1_SDK_DISPATCHER_POLL_INTERVAL_SEC": "0.5"}),
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{}, false, nil
		},
	}
	result, err := state.RunLoopTick(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("RunLoopTick returned error: %v", err)
	}
	if result.DispatchedCount != 0 || result.NextDelay != 500*time.Millisecond {
		t.Fatalf("result = %#v", result)
	}
}

// TestRuntimeStateRunLoopTickDoesNotDelayAfterDispatch mirrors Python hot loop yield.
func TestRuntimeStateRunLoopTickDoesNotDelayAfterDispatch(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	state := NewRuntimeState()
	service := Service{
		Now: func() time.Time { return now },
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-0", 0, now), true, nil
		},
		ExecuteBatch: func(_ context.Context, _ string, records []tasks.Record) ([]tasks.Record, error) {
			return []tasks.Record{{TaskID: records[0].TaskID, Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := state.RunLoopTick(context.Background(), service, "worker-a")
	if err != nil {
		t.Fatalf("RunLoopTick returned error: %v", err)
	}
	if result.DispatchedCount != 1 || result.NextDelay != 0 {
		t.Fatalf("result = %#v", result)
	}
}

// TestRuntimeStateRunLoopTickDelaysAfterError mirrors Python loop exception backoff.
func TestRuntimeStateRunLoopTickDelaysAfterError(t *testing.T) {
	listErr := errors.New("list failed")
	state := NewRuntimeState()
	service := Service{
		Env: mapLookup(map[string]string{"P1_SDK_DISPATCHER_POLL_INTERVAL_SEC": "0.25"}),
		ListDevices: func(context.Context) ([]string, error) {
			return nil, listErr
		},
	}
	result, err := state.RunLoopTick(context.Background(), service, "worker-a")
	if !errors.Is(err, listErr) {
		t.Fatalf("error = %v", err)
	}
	if result.NextDelay != 250*time.Millisecond || result.DispatchedCount != 0 {
		t.Fatalf("result = %#v", result)
	}
}
