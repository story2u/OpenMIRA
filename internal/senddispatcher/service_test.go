package senddispatcher

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestServiceApplyPreflightPassesDispatchableTask keeps SDK path untouched.
func TestServiceApplyPreflightPassesDispatchableTask(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
	}
	task := tasks.Record{
		TaskID:    "task-golden-0001",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if !result.Dispatchable || result.Record.TaskID != "task-golden-0001" {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 0 {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
}

// TestServiceApplyPreflightWritesExpiredTerminalStatus mirrors sdk_dispatcher_expired.
func TestServiceApplyPreflightWritesExpiredTerminalStatus(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
	}
	task := tasks.Record{
		TaskID:    "task-golden-0001",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if result.Dispatchable || result.Decision.Status != tasks.StatusTimeout || result.Record.Status != tasks.StatusTimeout {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.updates[0].Status != tasks.StatusTimeout {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
	if terminal.taskIDs[0] != "task-golden-0001" || terminal.updates[0].Error == nil {
		t.Fatalf("terminal taskIDs=%#v updates=%#v", terminal.taskIDs, terminal.updates)
	}
}

// TestServiceApplyPreflightWritesDisabledSourceTerminalStatus mirrors source disable.
func TestServiceApplyPreflightWritesDisabledSourceTerminalStatus(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
	}
	task := tasks.Record{
		TaskID:    "task-golden-0002",
		Source:    "cloud-web",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"_send_policy": map[string]any{"source_enabled": false, "reason": "paused"},
		},
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if result.Dispatchable || result.Decision.Status != tasks.StatusCancelled || result.Record.Status != tasks.StatusCancelled {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.updates[0].Status != tasks.StatusCancelled {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
	if terminal.updates[0].Error == nil || *terminal.updates[0].Error != "send source disabled before dispatch: origin=cloud-web, reason=paused" {
		t.Fatalf("terminal error = %#v", terminal.updates[0].Error)
	}
}

// TestServiceApplyPreflightFailsSlowQueueDuringUIUnstableCooldown mirrors detached SDK slow queue rejection.
func TestServiceApplyPreflightFailsSlowQueueDuringUIUnstableCooldown(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	publisher := &recordingTaskStatusPublisher{}
	service := Service{
		Terminal: terminal,
		DeviceHealthReader: &recordingSDKDeviceHealthReader{ui: &SDKDeviceUIUnstableState{
			Count:       3,
			Threshold:   3,
			CoolingDown: true,
			Stage:       "compose_surface",
			Error:       "click_plus_button plus button not found",
		}},
		TerminalSync: TerminalStateSyncOptions{Status: publisher},
		Now:          func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 {
			return 600
		},
	}
	task := tasks.Record{
		TaskID:    "task-slow-ui-cooldown",
		Target:    tasks.Target{DeviceID: "p1-slot-18"},
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"queue": "slow", "receiver": "Qiu"},
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if result.Dispatchable || result.Decision.Status != tasks.StatusFailed || result.Decision.Source != "sdk_device_ui_unstable_cooldown" {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.updates[0].Status != tasks.StatusFailed || terminal.updates[0].Error == nil {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
	if !strings.Contains(*terminal.updates[0].Error, "sdk device UI unstable cooldown") || !strings.Contains(*terminal.updates[0].Error, "channel=slow") {
		t.Fatalf("terminal error = %#v", terminal.updates[0].Error)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("publisher events = %#v", publisher.events)
	}
	payload, ok := publisher.events[0].Payload["result_payload"].(map[string]any)
	if !ok || payload["source"] != "sdk_device_ui_unstable_cooldown" || payload["sdk_ui_unstable_stage"] != "compose_surface" {
		t.Fatalf("published payload = %#v", publisher.events[0].Payload)
	}
}

// TestServiceApplyPreflightIgnoresUIUnstableCooldownForFastQueue keeps manual sends dispatchable.
func TestServiceApplyPreflightIgnoresUIUnstableCooldownForFastQueue(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal: terminal,
		DeviceHealthReader: &recordingSDKDeviceHealthReader{ui: &SDKDeviceUIUnstableState{
			Count:       3,
			Threshold:   3,
			CoolingDown: true,
			Stage:       "compose_surface",
		}},
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
	}
	task := tasks.Record{
		TaskID:    "task-fast-ui-cooldown",
		Target:    tasks.Target{DeviceID: "p1-slot-18"},
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"queue": "fast"},
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if !result.Dispatchable || len(terminal.updates) != 0 {
		t.Fatalf("result=%#v terminal=%#v", result, terminal.updates)
	}
}

// TestServiceApplyPreflightFailsDuringRecentTransportFailure avoids opening an unavailable SDK transport.
func TestServiceApplyPreflightFailsDuringRecentTransportFailure(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	publisher := &recordingTaskStatusPublisher{}
	service := Service{
		Terminal: terminal,
		DeviceHealthReader: &recordingSDKDeviceHealthReader{transport: &SDKDeviceTransportFailure{
			DeviceID: "p1-slot-18",
			Error:    "P1 device p1-slot-18 connection failed",
		}},
		TerminalSync:  TerminalStateSyncOptions{Status: publisher},
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
	}
	task := tasks.Record{
		TaskID:    "task-sdk-recent-failure",
		Target:    tasks.Target{DeviceID: "p1-slot-18"},
		TaskType:  "send_text",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Payload:   map[string]any{"receiver": "Qiu", "text": "hello"},
	}

	result, err := service.ApplyPreflight(context.Background(), task)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	want := "recent SDK transport failure for p1-slot-18: P1 device p1-slot-18 connection failed"
	if result.Dispatchable || result.Decision.Status != tasks.StatusFailed || result.Decision.Error != want {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.updates[0].Error == nil || *terminal.updates[0].Error != want {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("publisher events = %#v", publisher.events)
	}
	payload, ok := publisher.events[0].Payload["result_payload"].(map[string]any)
	if !ok || payload["source"] != "sdk_executor" || payload["success"] != false || payload["error"] != want {
		t.Fatalf("published payload = %#v", publisher.events[0].Payload)
	}
}

// TestServiceClaimNextDelegatesNormalizedRequest keeps persistence outside service.
func TestServiceClaimNextDelegatesNormalizedRequest(t *testing.T) {
	var captured ClaimRequest
	service := Service{
		Now: func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) },
		ClaimNextTask: func(_ context.Context, request ClaimRequest) (tasks.Record, bool, error) {
			captured = request
			return tasks.Record{TaskID: "task-golden-0001", Status: tasks.StatusRunning}, true, nil
		},
	}

	record, ok, err := service.ClaimNext(context.Background(), " worker-1 ", []string{" zimo ", " "})
	if err != nil || !ok {
		t.Fatalf("ClaimNext returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if record.TaskID != "task-golden-0001" || captured.WorkerID != "worker-1" {
		t.Fatalf("record=%#v captured=%#v", record, captured)
	}
	if len(captured.DeviceIDs) != 1 || captured.DeviceIDs[0] != "zimo" || captured.TaskTypes[0] != "appointment_billing" {
		t.Fatalf("captured request = %#v", captured)
	}
}

// TestServiceClaimNextNoOpsWithoutDevicesOrClaimer mirrors Python missing claim behavior.
func TestServiceClaimNextNoOpsWithoutDevicesOrClaimer(t *testing.T) {
	called := false
	service := Service{
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			called = true
			return tasks.Record{}, false, nil
		},
	}
	if _, ok, err := service.ClaimNext(context.Background(), "worker-1", []string{" "}); err != nil || ok || called {
		t.Fatalf("empty devices ok=%t err=%v called=%t", ok, err, called)
	}
	service.ClaimNextTask = nil
	if _, ok, err := service.ClaimNext(context.Background(), "worker-1", []string{"zimo"}); err != nil || ok {
		t.Fatalf("missing claimer ok=%t err=%v", ok, err)
	}
}

// TestServiceClaimAndPreflightNextNoOpsWhenNoTaskClaimed keeps empty claim inert.
func TestServiceClaimAndPreflightNextNoOpsWhenNoTaskClaimed(t *testing.T) {
	service := Service{
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{}, false, nil
		},
	}
	result, err := service.ClaimAndPreflightNext(context.Background(), "worker-1", []string{"zimo"})
	if err != nil {
		t.Fatalf("ClaimAndPreflightNext returned error: %v", err)
	}
	if result.Claimed {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceClaimAndPreflightNextReturnsDispatchableClaimedTask keeps executor boundary separate.
func TestServiceClaimAndPreflightNextReturnsDispatchableClaimedTask(t *testing.T) {
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{
				TaskID:    "task-golden-0001",
				Status:    tasks.StatusRunning,
				CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
			}, true, nil
		},
	}
	result, err := service.ClaimAndPreflightNext(context.Background(), "worker-1", []string{"zimo"})
	if err != nil {
		t.Fatalf("ClaimAndPreflightNext returned error: %v", err)
	}
	if !result.Claimed || !result.Preflight.Dispatchable || result.Task.TaskID != "task-golden-0001" {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceClaimAndPreflightNextWritesTerminalDecision protects pre-SDK stop path.
func TestServiceClaimAndPreflightNextWritesTerminalDecision(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{
				TaskID:    "task-golden-0001",
				Status:    tasks.StatusRunning,
				CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
			}, true, nil
		},
	}
	result, err := service.ClaimAndPreflightNext(context.Background(), "worker-1", []string{"zimo"})
	if err != nil {
		t.Fatalf("ClaimAndPreflightNext returned error: %v", err)
	}
	if !result.Claimed || result.Preflight.Dispatchable || result.Preflight.Decision.Status != tasks.StatusTimeout {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.updates[0].Status != tasks.StatusTimeout {
		t.Fatalf("terminal updates = %#v", terminal.updates)
	}
}

// TestServiceClaimAndPreflightBatchKeepsNonBatchableFirstSingle avoids image batching.
func TestServiceClaimAndPreflightBatchKeepsNonBatchableFirstSingle(t *testing.T) {
	calledBatch := false
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{
				TaskID:    "task-image",
				TaskType:  "send_image",
				Target:    tasks.Target{DeviceID: "zimo"},
				Status:    tasks.StatusRunning,
				CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
				Payload:   map[string]any{"receiver": "Qiu"},
			}, true, nil
		},
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			calledBatch = true
			return nil, nil
		},
	}
	result, err := service.ClaimAndPreflightBatch(context.Background(), "worker-1", []string{"zimo"}, 3)
	if err != nil {
		t.Fatalf("ClaimAndPreflightBatch returned error: %v", err)
	}
	if calledBatch {
		t.Fatal("non-batchable first task called followup claimer")
	}
	if !result.Claimed || result.DeviceID != "zimo" || len(result.ClaimedTasks) != 1 || len(result.Dispatchable) != 1 || result.Dispatchable[0].TaskID != "task-image" {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceClaimAndPreflightBatchStopsWhenFirstTerminal avoids collecting doomed batches.
func TestServiceClaimAndPreflightBatchStopsWhenFirstTerminal(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	calledBatch := false
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)), true, nil
		},
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			calledBatch = true
			return nil, nil
		},
	}
	result, err := service.ClaimAndPreflightBatch(context.Background(), "worker-1", []string{"zimo"}, 3)
	if err != nil {
		t.Fatalf("ClaimAndPreflightBatch returned error: %v", err)
	}
	if calledBatch || len(result.Dispatchable) != 0 || len(terminal.updates) != 1 {
		t.Fatalf("result=%#v calledBatch=%t terminal=%#v", result, calledBatch, terminal.updates)
	}
}

// TestServiceClaimAndPreflightBatchOrdersAndFiltersFollowups mirrors final pre-SDK gates.
func TestServiceClaimAndPreflightBatchOrdersAndFiltersFollowups(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	var captured BatchClaimRequest
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 1, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-2", 2, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)), true, nil
		},
		ClaimBatchAfterTask: func(_ context.Context, request BatchClaimRequest) ([]tasks.Record, error) {
			captured = request
			return []tasks.Record{
				dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)),
				dispatchBatchRecord("task-1", 1, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)),
			}, nil
		},
	}
	result, err := service.ClaimAndPreflightBatch(context.Background(), " worker-1 ", []string{"zimo"}, 3)
	if err != nil {
		t.Fatalf("ClaimAndPreflightBatch returned error: %v", err)
	}
	if captured.WorkerID != "worker-1" || captured.MaxSize != 2 || captured.FirstTask.TaskID != "task-2" || captured.SkipInterleaved {
		t.Fatalf("captured = %#v", captured)
	}
	if got := taskIDs(result.ClaimedTasks); len(got) != 3 || got[0] != "task-0" || got[1] != "task-1" || got[2] != "task-2" {
		t.Fatalf("claimed task ids = %#v", got)
	}
	if got := taskIDs(result.Dispatchable); len(got) != 2 || got[0] != "task-0" || got[1] != "task-2" {
		t.Fatalf("dispatchable task ids = %#v", got)
	}
	if len(result.Preflight) != 3 || result.Preflight[1].Decision.Status != tasks.StatusTimeout {
		t.Fatalf("preflight = %#v", result.Preflight)
	}
	if len(terminal.taskIDs) != 1 || terminal.taskIDs[0] != "task-1" {
		t.Fatalf("terminal task IDs = %#v", terminal.taskIDs)
	}
}

// TestServiceClaimAndPreflightStickyFollowupsNoOpsForNonBatchableAnchor mirrors image boundary.
func TestServiceClaimAndPreflightStickyFollowupsNoOpsForNonBatchableAnchor(t *testing.T) {
	calledBatch := false
	service := Service{
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			calledBatch = true
			return nil, nil
		},
	}
	anchor := tasks.Record{TaskID: "task-image", TaskType: "send_image", Target: tasks.Target{DeviceID: "zimo"}}
	result, err := service.ClaimAndPreflightStickyFollowups(context.Background(), anchor, "worker-1", 3)
	if err != nil {
		t.Fatalf("ClaimAndPreflightStickyFollowups returned error: %v", err)
	}
	if calledBatch || result.Claimed || result.DeviceID != "zimo" {
		t.Fatalf("result=%#v calledBatch=%t", result, calledBatch)
	}
}

// TestServiceClaimAndPreflightStickyFollowupsUsesSkipInterleaved mirrors continuation claim.
func TestServiceClaimAndPreflightStickyFollowupsUsesSkipInterleaved(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	var captured BatchClaimRequest
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 1, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimBatchAfterTask: func(_ context.Context, request BatchClaimRequest) ([]tasks.Record, error) {
			captured = request
			return []tasks.Record{
				dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)),
				dispatchBatchRecord("task-2", 2, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)),
			}, nil
		},
	}
	anchor := dispatchBatchRecord("task-1", 1, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))
	result, err := service.ClaimAndPreflightStickyFollowups(context.Background(), anchor, " worker-1 ", 2)
	if err != nil {
		t.Fatalf("ClaimAndPreflightStickyFollowups returned error: %v", err)
	}
	if captured.WorkerID != "worker-1" || captured.MaxSize != 2 || !captured.SkipInterleaved || captured.FirstTask.TaskID != "task-1" {
		t.Fatalf("captured = %#v", captured)
	}
	if got := taskIDs(result.ClaimedTasks); len(got) != 2 || got[0] != "task-0" || got[1] != "task-2" {
		t.Fatalf("claimed task ids = %#v", got)
	}
	if got := taskIDs(result.Dispatchable); len(got) != 1 || got[0] != "task-0" {
		t.Fatalf("dispatchable task ids = %#v", got)
	}
	if !result.Claimed || len(terminal.taskIDs) != 1 || terminal.taskIDs[0] != "task-2" {
		t.Fatalf("result=%#v terminal=%#v", result, terminal.taskIDs)
	}
}

// TestServiceApplyCurrentChatReuseMarksKnownInitialBatch mirrors last-target reuse.
func TestServiceApplyCurrentChatReuseMarksKnownInitialBatch(t *testing.T) {
	records := []tasks.Record{
		dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)),
		dispatchBatchRecord("task-1", 1, time.Date(2026, 6, 29, 9, 10, 1, 0, time.UTC)),
	}
	key, ok := BatchReuseKey(records)
	if !ok {
		t.Fatal("test records missing reuse key")
	}
	result := Service{}.ApplyCurrentChatReuse(DispatchBatchResult{
		DeviceID:     "zimo",
		Dispatchable: records,
	}, map[string]ReuseKey{"zimo": key}, false)
	if !result.ReuseCurrentChat || !result.ReuseKeyPresent || result.ReuseKey != key {
		t.Fatalf("result = %#v", result)
	}
	if result.Dispatchable[0].Payload["_reuse_current_chat"] != true || result.Dispatchable[1].Payload["_reuse_current_chat"] != true {
		t.Fatalf("dispatchable payloads = %#v", result.Dispatchable)
	}
	if _, ok := records[0].Payload["_reuse_current_chat"]; ok {
		t.Fatalf("source payload mutated: %#v", records[0].Payload)
	}
}

// TestServiceApplyCurrentChatReuseRequiresLastTargetUnlessForced separates initial and sticky batches.
func TestServiceApplyCurrentChatReuseRequiresLastTargetUnlessForced(t *testing.T) {
	records := []tasks.Record{
		dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)),
	}
	result := DispatchBatchResult{DeviceID: "zimo", Dispatchable: records}
	unmarked := Service{}.ApplyCurrentChatReuse(result, map[string]ReuseKey{"ada": ReuseKey{"Qiu", "", "", "conversation-1", "sender-1"}}, false)
	if unmarked.ReuseCurrentChat || unmarked.Dispatchable[0].Payload["_reuse_current_chat"] == true {
		t.Fatalf("unmarked result = %#v", unmarked)
	}
	forced := Service{}.ApplyCurrentChatReuse(result, nil, true)
	if !forced.ReuseCurrentChat || forced.Dispatchable[0].Payload["_reuse_current_chat"] != true {
		t.Fatalf("forced result = %#v", forced)
	}
}

// TestServiceExecutePreparedBatchAppliesReuseAndRemembersSuccess protects executor boundary.
func TestServiceExecutePreparedBatchAppliesReuseAndRemembersSuccess(t *testing.T) {
	records := []tasks.Record{
		dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)),
	}
	key, ok := BatchReuseKey(records)
	if !ok {
		t.Fatal("test records missing reuse key")
	}
	var capturedDevice string
	var captured []tasks.Record
	service := Service{
		ExecuteBatch: func(_ context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
			capturedDevice = deviceID
			captured = records
			return []tasks.Record{{TaskID: "task-0", Status: tasks.StatusSuccess}}, nil
		},
	}
	executed, err := service.ExecutePreparedBatch(context.Background(), DispatchBatchResult{
		DeviceID:     "zimo",
		Dispatchable: records,
	}, map[string]ReuseKey{"zimo": key}, false)
	if err != nil {
		t.Fatalf("ExecutePreparedBatch returned error: %v", err)
	}
	if capturedDevice != "zimo" || len(captured) != 1 || captured[0].Payload["_reuse_current_chat"] != true {
		t.Fatalf("capturedDevice=%q captured=%#v", capturedDevice, captured)
	}
	if !executed.Batch.ReuseCurrentChat || executed.LastTargets["zimo"] != key || len(executed.Finalized) != 1 {
		t.Fatalf("executed = %#v", executed)
	}
	if _, ok := records[0].Payload["_reuse_current_chat"]; ok {
		t.Fatalf("source payload mutated: %#v", records[0].Payload)
	}
}

// TestServiceExecutePreparedBatchUsesDeviceLock wraps executor calls with Redis device ownership.
func TestServiceExecutePreparedBatchUsesDeviceLock(t *testing.T) {
	records := []tasks.Record{dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))}
	lockStore := &recordingDeviceLockStore{acquireResult: true}
	calledExecutor := false
	service := Service{
		DeviceLockStore:                  lockStore,
		NewDeviceLockNonce:               func() string { return "abc" },
		DeviceLockPID:                    123,
		DeviceLockExecutorTimeoutSeconds: 30,
		Env:                              mapLookup(map[string]string{"P1_SDK_DEVICE_LOCK_TTL_MS": "12000"}),
		ExecuteBatch: func(context.Context, string, []tasks.Record) ([]tasks.Record, error) {
			calledExecutor = true
			return []tasks.Record{{TaskID: "task-0", Status: tasks.StatusSuccess}}, nil
		},
	}
	executed, err := service.ExecutePreparedBatch(context.Background(), DispatchBatchResult{
		DeviceID:     "zimo",
		Dispatchable: records,
	}, nil, false)
	if err != nil {
		t.Fatalf("ExecutePreparedBatch returned error: %v", err)
	}
	if !calledExecutor || len(executed.Finalized) != 1 {
		t.Fatalf("calledExecutor=%t executed=%#v", calledExecutor, executed)
	}
	if lockStore.key != "lock:sdk-device:zimo" || lockStore.token != "123:task-0:abc" || lockStore.ttl != 12*time.Second {
		t.Fatalf("lock acquire = %#v", lockStore)
	}
	if lockStore.releaseCalls != 1 || lockStore.releaseKey != lockStore.key || lockStore.releaseToken != lockStore.token {
		t.Fatalf("lock release = %#v", lockStore)
	}
}

// TestServiceExecutePreparedBatchResolvesDeviceLockAlias keeps Redis lock keys canonical.
func TestServiceExecutePreparedBatchResolvesDeviceLockAlias(t *testing.T) {
	records := []tasks.Record{dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))}
	lockStore := &recordingDeviceLockStore{acquireResult: true}
	capturedDevice := ""
	service := Service{
		DeviceLockStore:    lockStore,
		DeviceIDResolver:   recordingSDKDeviceIDResolver{"slot-18": "p1-slot-18"},
		NewDeviceLockNonce: func() string { return "abc" },
		DeviceLockPID:      123,
		ExecuteBatch: func(_ context.Context, deviceID string, _ []tasks.Record) ([]tasks.Record, error) {
			capturedDevice = deviceID
			return []tasks.Record{{TaskID: "task-0", Status: tasks.StatusSuccess}}, nil
		},
	}
	_, err := service.ExecutePreparedBatch(context.Background(), DispatchBatchResult{
		DeviceID:     "slot-18",
		Dispatchable: records,
	}, nil, false)
	if err != nil {
		t.Fatalf("ExecutePreparedBatch returned error: %v", err)
	}
	if lockStore.key != "lock:sdk-device:p1-slot-18" {
		t.Fatalf("lock key = %q", lockStore.key)
	}
	if capturedDevice != "slot-18" {
		t.Fatalf("executor device = %q", capturedDevice)
	}
}

// TestServiceExecutePreparedBatchClearsLastTargetOnFailure mirrors finalized failure.
func TestServiceExecutePreparedBatchClearsLastTargetOnFailure(t *testing.T) {
	records := []tasks.Record{dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))}
	key, ok := BatchReuseKey(records)
	if !ok {
		t.Fatal("test records missing reuse key")
	}
	service := Service{
		ExecuteBatch: func(context.Context, string, []tasks.Record) ([]tasks.Record, error) {
			return []tasks.Record{{TaskID: "task-0", Status: tasks.StatusFailed}}, nil
		},
	}
	executed, err := service.ExecutePreparedBatch(context.Background(), DispatchBatchResult{
		DeviceID:     "zimo",
		Dispatchable: records,
	}, map[string]ReuseKey{"zimo": key}, false)
	if err != nil {
		t.Fatalf("ExecutePreparedBatch returned error: %v", err)
	}
	if _, ok := executed.LastTargets["zimo"]; ok {
		t.Fatalf("last target kept after failure: %#v", executed.LastTargets)
	}
}

// TestServiceExecutePreparedBatchRequiresExecutor keeps claimed work from disappearing.
func TestServiceExecutePreparedBatchRequiresExecutor(t *testing.T) {
	if executed, err := (Service{}).ExecutePreparedBatch(context.Background(), DispatchBatchResult{DeviceID: "zimo"}, nil, false); err != nil || len(executed.Finalized) != 0 {
		t.Fatalf("empty batch executed=%#v err=%v", executed, err)
	}
	_, err := (Service{}).ExecutePreparedBatch(context.Background(), DispatchBatchResult{
		DeviceID:     "zimo",
		Dispatchable: []tasks.Record{dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))},
	}, nil, false)
	if err == nil {
		t.Fatal("missing executor returned nil error")
	}
}

// TestServiceRunOnceNoOpsWhenNoTaskClaimed keeps idle loops inert.
func TestServiceRunOnceNoOpsWhenNoTaskClaimed(t *testing.T) {
	calledExecutor := false
	service := Service{
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return tasks.Record{}, false, nil
		},
		ExecuteBatch: func(context.Context, string, []tasks.Record) ([]tasks.Record, error) {
			calledExecutor = true
			return nil, nil
		},
	}
	result, err := service.RunOnce(context.Background(), "worker-1", []string{"zimo"}, 3, nil)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Claimed || calledExecutor {
		t.Fatalf("result=%#v calledExecutor=%t", result, calledExecutor)
	}
}

// TestServiceRunOnceStopsAfterTerminalPreflight avoids executor calls for doomed tasks.
func TestServiceRunOnceStopsAfterTerminalPreflight(t *testing.T) {
	terminal := &recordingTerminalUpdater{}
	calledExecutor := false
	service := Service{
		Terminal:      terminal,
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)), true, nil
		},
		ExecuteBatch: func(context.Context, string, []tasks.Record) ([]tasks.Record, error) {
			calledExecutor = true
			return nil, nil
		},
	}
	result, err := service.RunOnce(context.Background(), "worker-1", []string{"zimo"}, 3, nil)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Claimed || result.ClaimedCount != 1 || calledExecutor || len(terminal.updates) != 1 {
		t.Fatalf("result=%#v calledExecutor=%t terminal=%#v", result, calledExecutor, terminal.updates)
	}
}

// TestServiceRunOnceClaimsAndExecutesInitialBatch mirrors one dispatcher pass.
func TestServiceRunOnceClaimsAndExecutesInitialBatch(t *testing.T) {
	var captured []tasks.Record
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 10, 5, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			return dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)), true, nil
		},
		ExecuteBatch: func(_ context.Context, _ string, records []tasks.Record) ([]tasks.Record, error) {
			captured = records
			return []tasks.Record{{TaskID: "task-0", Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := service.RunOnce(context.Background(), "worker-1", []string{"zimo"}, 3, nil)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if !result.Claimed || result.ClaimedCount != 1 || len(result.Execution.Finalized) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(captured) != 1 || captured[0].Payload["_reuse_current_chat"] == true {
		t.Fatalf("captured = %#v", captured)
	}
	key, ok := BatchReuseKey(captured)
	if !ok || result.LastTargets["zimo"] != key {
		t.Fatalf("lastTargets=%#v key=%#v ok=%t", result.LastTargets, key, ok)
	}
}

// TestServiceRunOnceWithActiveSetSkipsBusyDevices keeps local same-device exclusion.
func TestServiceRunOnceWithActiveSetSkipsBusyDevices(t *testing.T) {
	activeSet := NewActiveDeviceSet()
	if !activeSet.TryAcquire("zimo") {
		t.Fatal("failed to acquire test device")
	}
	calledClaim := false
	service := Service{
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			calledClaim = true
			return tasks.Record{}, false, nil
		},
	}
	result, err := service.RunOnceWithActiveSet(context.Background(), "worker-1", StatusSnapshot{OwnedDeviceIDs: []string{"zimo"}}, activeSet, 2, nil)
	if err != nil {
		t.Fatalf("RunOnceWithActiveSet returned error: %v", err)
	}
	if result.Claimed || calledClaim {
		t.Fatalf("result=%#v calledClaim=%t", result, calledClaim)
	}
}

// TestServiceRunOnceWithActiveSetClaimsIdleAndReleasesAfterExecution mirrors coordinator lane.
func TestServiceRunOnceWithActiveSetClaimsIdleAndReleasesAfterExecution(t *testing.T) {
	activeSet := NewActiveDeviceSet()
	if !activeSet.TryAcquire("zimo") {
		t.Fatal("failed to acquire busy device")
	}
	var capturedRequest ClaimRequest
	var capturedDevice string
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 10, 5, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(_ context.Context, request ClaimRequest) (tasks.Record, bool, error) {
			capturedRequest = request
			record := dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))
			record.Target.DeviceID = "ada"
			return record, true, nil
		},
		ExecuteBatch: func(_ context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
			capturedDevice = deviceID
			return []tasks.Record{{TaskID: records[0].TaskID, Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := service.RunOnceWithActiveSet(context.Background(), "worker-1", StatusSnapshot{OwnedDeviceIDs: []string{"zimo", "ada"}}, activeSet, 2, nil)
	if err != nil {
		t.Fatalf("RunOnceWithActiveSet returned error: %v", err)
	}
	if !result.Claimed || result.ClaimedCount != 1 || capturedDevice != "ada" {
		t.Fatalf("result=%#v capturedDevice=%q", result, capturedDevice)
	}
	if len(capturedRequest.DeviceIDs) != 1 || capturedRequest.DeviceIDs[0] != "ada" {
		t.Fatalf("captured request = %#v", capturedRequest)
	}
	if !activeSet.TryAcquire("ada") {
		t.Fatal("idle device was not released after execution")
	}
}

// TestServiceRunOnceWithActiveSetRejectsClaimedBusyDevice defends the service boundary.
func TestServiceRunOnceWithActiveSetRejectsClaimedBusyDevice(t *testing.T) {
	activeSet := NewActiveDeviceSet()
	if !activeSet.TryAcquire("zimo") {
		t.Fatal("failed to acquire busy device")
	}
	calledExecute := false
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 10, 5, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimNextTask: func(context.Context, ClaimRequest) (tasks.Record, bool, error) {
			record := dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC))
			record.Target.DeviceID = "zimo"
			return record, true, nil
		},
		ExecuteBatch: func(context.Context, string, []tasks.Record) ([]tasks.Record, error) {
			calledExecute = true
			return nil, nil
		},
	}
	_, err := service.RunOnceWithActiveSet(context.Background(), "worker-1", StatusSnapshot{OwnedDeviceIDs: []string{"zimo", "ada"}}, activeSet, 2, nil)
	if err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("error = %v", err)
	}
	if calledExecute {
		t.Fatal("ExecuteBatch was called")
	}
	if activeSet.TryAcquire("zimo") {
		t.Fatal("busy device was released after rejected claim")
	}
}

// TestServiceRunStickyFollowupsNoOpsWithoutRounds keeps bounded continuation explicit.
func TestServiceRunStickyFollowupsNoOpsWithoutRounds(t *testing.T) {
	calledClaim := false
	service := Service{
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			calledClaim = true
			return nil, nil
		},
	}
	result, err := service.RunStickyFollowups(context.Background(), dispatchBatchRecord("task-0", 0, time.Now()), "worker-1", 2, 0, nil)
	if err != nil {
		t.Fatalf("RunStickyFollowups returned error: %v", err)
	}
	if calledClaim || result.Rounds != 0 {
		t.Fatalf("result=%#v calledClaim=%t", result, calledClaim)
	}
}

// TestServiceRunStickyFollowupsExecutesBoundedContinuation mirrors sticky rounds.
func TestServiceRunStickyFollowupsExecutesBoundedContinuation(t *testing.T) {
	claimCalls := 0
	var captured []tasks.Record
	service := Service{
		Now:           func() time.Time { return time.Date(2026, 6, 29, 9, 10, 5, 0, time.UTC) },
		MaxAgeSeconds: func() float64 { return 600 },
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			claimCalls++
			if claimCalls > 1 {
				return nil, nil
			}
			return []tasks.Record{dispatchBatchRecord("task-1", 1, time.Date(2026, 6, 29, 9, 10, 1, 0, time.UTC))}, nil
		},
		ExecuteBatch: func(_ context.Context, _ string, records []tasks.Record) ([]tasks.Record, error) {
			captured = records
			return []tasks.Record{{TaskID: "task-1", Status: tasks.StatusSuccess}}, nil
		},
	}
	result, err := service.RunStickyFollowups(context.Background(), dispatchBatchRecord("task-0", 0, time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)), "worker-1", 2, 3, nil)
	if err != nil {
		t.Fatalf("RunStickyFollowups returned error: %v", err)
	}
	if claimCalls != 2 || result.Rounds != 1 || result.ClaimedCount != 1 || len(result.Executions) != 1 {
		t.Fatalf("result=%#v claimCalls=%d", result, claimCalls)
	}
	if len(captured) != 1 || captured[0].Payload["_reuse_current_chat"] != true {
		t.Fatalf("captured = %#v", captured)
	}
	key, ok := BatchReuseKey(captured)
	if !ok || result.LastTargets["zimo"] != key {
		t.Fatalf("lastTargets=%#v key=%#v ok=%t", result.LastTargets, key, ok)
	}
}

// TestServiceCaptureStatusSnapshotNoOpsWithoutRuntimeReaders keeps startup lightweight.
func TestServiceCaptureStatusSnapshotNoOpsWithoutRuntimeReaders(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	service := Service{
		Now: func() time.Time { return now },
		Env: mapLookup(map[string]string{"SEND_WORKER_ID": "worker-1"}),
	}
	snapshot, err := service.CaptureStatusSnapshot(context.Background(), "")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshot returned error: %v", err)
	}
	if snapshot.WorkerID != "worker-1" || snapshot.VisibleDeviceCount != 0 || snapshot.OwnedDeviceCount != 0 || !snapshot.CapturedAt.Equal(now) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Backlog.ByDevice == nil {
		t.Fatalf("snapshot backlog missing by_device map: %#v", snapshot.Backlog)
	}
}

// TestServiceCaptureStatusSnapshotAppliesOwnershipAndBacklog mirrors Python snapshot path.
func TestServiceCaptureStatusSnapshotAppliesOwnershipAndBacklog(t *testing.T) {
	var capturedOwned []string
	service := Service{
		Now: func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) },
		Env: mapLookup(map[string]string{
			"SEND_DEVICE_ALLOWLIST":   "zimo,ada",
			"SEND_DEVICE_EXCLUDELIST": "ada",
		}),
		ListDevices: func(context.Context) ([]string, error) {
			return []string{" zimo ", "ada", "zimo", "bob"}, nil
		},
		SummarizeBacklog: func(_ context.Context, ownedDeviceIDs []string) (BacklogSummary, error) {
			capturedOwned = append([]string(nil), ownedDeviceIDs...)
			return BacklogSummary{
				AcceptedTotal: 1,
				ByDevice:      map[string]BacklogDeviceSummary{"zimo": {Accepted: 1}},
			}, nil
		},
	}
	snapshot, err := service.CaptureStatusSnapshot(context.Background(), " worker-1 ")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshot returned error: %v", err)
	}
	if snapshot.WorkerID != "worker-1" || snapshot.VisibleDeviceCount != 3 || snapshot.OwnedDeviceCount != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(capturedOwned) != 1 || capturedOwned[0] != "zimo" || snapshot.Backlog.AcceptedTotal != 1 {
		t.Fatalf("capturedOwned=%#v snapshot=%#v", capturedOwned, snapshot)
	}
}

// TestServiceCaptureStatusSnapshotUsesShortCache mirrors Python status snapshot caching.
func TestServiceCaptureStatusSnapshotUsesShortCache(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	listCalls := 0
	backlogCalls := 0
	service := Service{
		Now:           func() time.Time { return now },
		SnapshotCache: NewStatusSnapshotCache(),
		Env: mapLookup(map[string]string{
			"SEND_DEVICE_ALLOWLIST":                     "zimo",
			"P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC": "10",
		}),
		ListDevices: func(context.Context) ([]string, error) {
			listCalls++
			return []string{"ada", "zimo"}, nil
		},
		SummarizeBacklog: func(context.Context, []string) (BacklogSummary, error) {
			backlogCalls++
			return BacklogSummary{AcceptedTotal: 1, ByDevice: map[string]BacklogDeviceSummary{"zimo": {Accepted: 1}}}, nil
		},
	}
	first, err := service.CaptureStatusSnapshot(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshot first returned error: %v", err)
	}
	now = now.Add(5 * time.Second)
	second, err := service.CaptureStatusSnapshot(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("CaptureStatusSnapshot second returned error: %v", err)
	}
	if listCalls != 1 || backlogCalls != 1 {
		t.Fatalf("calls list=%d backlog=%d", listCalls, backlogCalls)
	}
	if !second.CapturedAt.Equal(first.CapturedAt) || second.WorkerID != "worker-a" || second.OwnedDeviceIDs[0] != "zimo" {
		t.Fatalf("cached snapshot mismatch: first=%#v second=%#v", first, second)
	}
}

// TestServiceCaptureStatusSnapshotCanDisableCache keeps TTL zero as an explicit bypass.
func TestServiceCaptureStatusSnapshotCanDisableCache(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	listCalls := 0
	service := Service{
		Now:           func() time.Time { return now },
		SnapshotCache: NewStatusSnapshotCache(),
		Env:           mapLookup(map[string]string{"P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC": "0"}),
		ListDevices: func(context.Context) ([]string, error) {
			listCalls++
			return []string{"zimo"}, nil
		},
	}
	if _, err := service.CaptureStatusSnapshot(context.Background(), "worker-a"); err != nil {
		t.Fatalf("CaptureStatusSnapshot first returned error: %v", err)
	}
	now = now.Add(time.Second)
	if _, err := service.CaptureStatusSnapshot(context.Background(), "worker-a"); err != nil {
		t.Fatalf("CaptureStatusSnapshot second returned error: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("list calls = %d", listCalls)
	}
}

// TestServiceRecordStatusHeartbeatNoOpsWhenMissingOrNotDue mirrors optional repository behavior.
func TestServiceRecordStatusHeartbeatNoOpsWhenMissingOrNotDue(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 10, 0, time.UTC)
	snapshot := StatusSnapshot{WorkerID: "worker-1"}
	result, err := Service{}.RecordStatusHeartbeat(context.Background(), snapshot, nil, now)
	if err != nil {
		t.Fatalf("RecordStatusHeartbeat returned error: %v", err)
	}
	if result.Recorded {
		t.Fatalf("missing writer recorded heartbeat: %#v", result)
	}

	called := false
	previous := RememberHeartbeat(nil, "worker-1", now.Add(-9*time.Second))
	service := Service{
		Env: mapLookup(map[string]string{"SEND_WORKER_HEARTBEAT_INTERVAL_SEC": "10"}),
		RecordHeartbeat: func(context.Context, HeartbeatRecord) error {
			called = true
			return nil
		},
	}
	result, err = service.RecordStatusHeartbeat(context.Background(), snapshot, previous, now)
	if err != nil {
		t.Fatalf("RecordStatusHeartbeat returned error: %v", err)
	}
	if result.Recorded || called {
		t.Fatalf("heartbeat recorded before interval: result=%#v called=%t", result, called)
	}
}

// TestServiceRecordStatusHeartbeatWritesDueRecord protects repository-neutral payload.
func TestServiceRecordStatusHeartbeatWritesDueRecord(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 10, 0, time.FixedZone("CST", 8*60*60))
	backlog := BacklogSummary{AcceptedTotal: 1, ByDevice: map[string]BacklogDeviceSummary{"zimo": {Accepted: 1}}}
	var captured HeartbeatRecord
	service := Service{
		Env: mapLookup(map[string]string{
			"SEND_WORKER_ROLE":                   "send-dispatcher",
			"SEND_WORKER_POOL":                   "pool-a",
			"SEND_WORKER_HOSTNAME":               "host-a",
			"SEND_WORKER_LEASE_TTL_SEC":          "45",
			"SEND_WORKER_HEARTBEAT_INTERVAL_SEC": "10",
		}),
		RecordHeartbeat: func(_ context.Context, record HeartbeatRecord) error {
			captured = record
			return nil
		},
	}
	result, err := service.RecordStatusHeartbeat(context.Background(), StatusSnapshot{
		WorkerID:         " worker-1 ",
		VisibleDeviceIDs: []string{"zimo", "ada"},
		OwnedDeviceIDs:   []string{"zimo"},
		Allowlist:        []string{"zimo"},
		Exclude:          []string{"ada"},
		Backlog:          backlog,
	}, nil, now)
	if err != nil {
		t.Fatalf("RecordStatusHeartbeat returned error: %v", err)
	}
	if !result.Recorded || captured.WorkerID != "worker-1" || captured.WorkerRole != "send-dispatcher" || captured.WorkerPool != "pool-a" || captured.Hostname != "host-a" {
		t.Fatalf("result=%#v captured=%#v", result, captured)
	}
	if captured.LeaseTTLSeconds != 45 || !captured.Now.Equal(now.UTC()) {
		t.Fatalf("heartbeat timing = %#v", captured)
	}
	if len(captured.VisibleDeviceIDs) != 2 || len(captured.OwnedDeviceIDs) != 1 || captured.Metadata["runtime"] != "sdk_dispatcher" {
		t.Fatalf("heartbeat devices/metadata = %#v", captured)
	}
	if _, ok := result.Previous["worker-1"]; !ok {
		t.Fatalf("previous heartbeat not updated: %#v", result.Previous)
	}
}

// TestServiceRecoverStaleRunningTasksWritesTerminalTimeout mirrors watchdog recovery.
func TestServiceRecoverStaleRunningTasksWritesTerminalTimeout(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	staleStart := now.Add(-5 * time.Minute)
	freshStart := now.Add(-10 * time.Second)
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal: terminal,
		ListRunningTasks: func(context.Context) ([]tasks.Record, error) {
			return []tasks.Record{
				{TaskID: "task-stale", TaskType: "send_text", Status: tasks.StatusRunning, ScriptStartedAt: &staleStart},
				{TaskID: "task-fresh", TaskType: "send_text", Status: tasks.StatusRunning, ScriptStartedAt: &freshStart},
				{TaskID: "task-other", TaskType: "non_sdk", Status: tasks.StatusRunning, ScriptStartedAt: &staleStart},
			}, nil
		},
	}
	result, err := service.RecoverStaleRunningTasks(context.Background(), now, 60)
	if err != nil {
		t.Fatalf("RecoverStaleRunningTasks returned error: %v", err)
	}
	if result.Scanned != 3 || result.Recovered != 1 || len(result.Records) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(terminal.updates) != 1 || terminal.taskIDs[0] != "task-stale" || terminal.updates[0].Status != tasks.StatusTimeout {
		t.Fatalf("terminal taskIDs=%#v updates=%#v", terminal.taskIDs, terminal.updates)
	}
	if terminal.updates[0].Error == nil || !strings.Contains(*terminal.updates[0].Error, "sdk task stale timeout after running watchdog") {
		t.Fatalf("terminal error = %#v", terminal.updates[0].Error)
	}
}

// TestServiceRecoverStaleRunningTasksNoOpsWithoutReader keeps recovery opt-in.
func TestServiceRecoverStaleRunningTasksNoOpsWithoutReader(t *testing.T) {
	result, err := (Service{}).RecoverStaleRunningTasks(context.Background(), time.Now(), 60)
	if err != nil {
		t.Fatalf("RecoverStaleRunningTasks returned error: %v", err)
	}
	if result.Scanned != 0 || result.Recovered != 0 {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceRunRecoveryTickReturnsInterval keeps recovery loop caller-controlled.
func TestServiceRunRecoveryTickReturnsInterval(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	staleStart := now.Add(-5 * time.Minute)
	terminal := &recordingTerminalUpdater{}
	service := Service{
		Terminal: terminal,
		Now:      func() time.Time { return now },
		Env: mapLookup(map[string]string{
			"P1_SDK_RUNNING_TASK_RECOVERY_INTERVAL_SEC": "7",
			"P1_SDK_RUNNING_TASK_STALE_TIMEOUT_SEC":     "60",
		}),
		ListRunningTasks: func(context.Context) ([]tasks.Record, error) {
			return []tasks.Record{{TaskID: "task-stale", TaskType: "send_text", Status: tasks.StatusRunning, ScriptStartedAt: &staleStart}}, nil
		},
	}
	result, err := service.RunRecoveryTick(context.Background())
	if err != nil {
		t.Fatalf("RunRecoveryTick returned error: %v", err)
	}
	if result.NextDelay != 7*time.Second || result.Recovery.Recovered != 1 {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceRunRecoveryTickReturnsDelayOnError mirrors loop exception backoff.
func TestServiceRunRecoveryTickReturnsDelayOnError(t *testing.T) {
	listErr := fmt.Errorf("list failed")
	service := Service{
		Env: mapLookup(map[string]string{"P1_SDK_RUNNING_TASK_RECOVERY_INTERVAL_SEC": "5"}),
		ListRunningTasks: func(context.Context) ([]tasks.Record, error) {
			return nil, listErr
		},
	}
	result, err := service.RunRecoveryTick(context.Background())
	if err != listErr {
		t.Fatalf("error = %v", err)
	}
	if result.NextDelay != 5*time.Second {
		t.Fatalf("result = %#v", result)
	}
}

// TestServiceClaimBatchAfterDelegatesNormalizedRequest keeps batch persistence injected.
func TestServiceClaimBatchAfterDelegatesNormalizedRequest(t *testing.T) {
	var captured BatchClaimRequest
	first := tasks.Record{TaskID: "task-golden-0001", TaskType: "send_text"}
	service := Service{
		Now: func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) },
		ClaimBatchAfterTask: func(_ context.Context, request BatchClaimRequest) ([]tasks.Record, error) {
			captured = request
			return []tasks.Record{{TaskID: "task-golden-0002", Status: tasks.StatusRunning}}, nil
		},
	}

	records, err := service.ClaimBatchAfter(context.Background(), first, " worker-1 ", 2, true)
	if err != nil {
		t.Fatalf("ClaimBatchAfter returned error: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "task-golden-0002" {
		t.Fatalf("records = %#v", records)
	}
	if captured.FirstTask.TaskID != "task-golden-0001" || captured.WorkerID != "worker-1" || captured.MaxSize != 2 || !captured.SkipInterleaved {
		t.Fatalf("captured = %#v", captured)
	}
}

// TestServiceClaimBatchAfterNoOpsWithoutSizeOrClaimer mirrors optional repository behavior.
func TestServiceClaimBatchAfterNoOpsWithoutSizeOrClaimer(t *testing.T) {
	called := false
	service := Service{
		ClaimBatchAfterTask: func(context.Context, BatchClaimRequest) ([]tasks.Record, error) {
			called = true
			return nil, nil
		},
	}
	if records, err := service.ClaimBatchAfter(context.Background(), tasks.Record{}, "worker-1", 0, false); err != nil || len(records) != 0 || called {
		t.Fatalf("records=%#v err=%v called=%t", records, err, called)
	}
	service.ClaimBatchAfterTask = nil
	if records, err := service.ClaimBatchAfter(context.Background(), tasks.Record{}, "worker-1", 1, false); err != nil || len(records) != 0 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

func dispatchBatchRecord(taskID string, index int, createdAt time.Time) tasks.Record {
	return tasks.Record{
		TaskID:    taskID,
		TaskType:  "send_text",
		Target:    tasks.Target{DeviceID: "zimo"},
		Status:    tasks.StatusRunning,
		CreatedAt: createdAt,
		Payload: map[string]any{
			"receiver":           "Qiu",
			"conversation_id":    "conversation-1",
			"sender_id":          "sender-1",
			"client_batch_id":    "batch-1",
			"client_batch_index": index,
		},
	}
}

func taskIDs(records []tasks.Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.TaskID)
	}
	return ids
}

type recordingTerminalUpdater struct {
	taskIDs []string
	updates []tasks.StatusUpdate
}

func (updater *recordingTerminalUpdater) UpdateTerminalStatus(_ context.Context, taskID string, update tasks.StatusUpdate) (tasks.Record, error) {
	updater.taskIDs = append(updater.taskIDs, taskID)
	updater.updates = append(updater.updates, update)
	return tasks.Record{TaskID: taskID, Status: update.Status, Error: update.Error}, nil
}

type recordingSDKDeviceHealthReader struct {
	transport *SDKDeviceTransportFailure
	ui        *SDKDeviceUIUnstableState
	err       error
}

func (reader *recordingSDKDeviceHealthReader) GetRecentSDKDeviceTransportFailure(context.Context, string) (*SDKDeviceTransportFailure, error) {
	return reader.transport, reader.err
}

func (reader *recordingSDKDeviceHealthReader) GetRecentSDKDeviceUIUnstableState(context.Context, string) (*SDKDeviceUIUnstableState, error) {
	return reader.ui, reader.err
}

type recordingSDKDeviceIDResolver map[string]string

func (resolver recordingSDKDeviceIDResolver) ResolveSDKDeviceID(_ context.Context, deviceID string) (string, error) {
	return resolver[deviceID], nil
}
