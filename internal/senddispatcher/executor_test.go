package senddispatcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestBuildSDKTaskPayloadMirrorsPythonTaskDict protects the executor input contract.
func TestBuildSDKTaskPayloadMirrorsPythonTaskDict(t *testing.T) {
	traceID := "trace-sdk-1"
	record := tasks.Record{
		TaskID:    "task-sdk-1",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:p1-slot-18", DeviceID: "p1-slot-18"},
		TaskType:  "send_text",
		CreatedAt: time.Date(2026, 6, 30, 9, 2, 3, 123400000, time.UTC),
		TraceID:   &traceID,
		Payload: map[string]any{
			"receiver":        "Qiu",
			"text":            "hi",
			"conversation_id": "conversation-1",
			"task_id":         "payload-task",
			"device_id":       "payload-device",
			"target":          "payload-target",
		},
	}

	payload := BuildSDKTaskPayload(record, " p1-slot-18 ")

	if payload["task_id"] != "task-sdk-1" || payload["source"] != "cloud-web" || payload["task_type"] != "send_text" {
		t.Fatalf("core payload fields = %#v", payload)
	}
	if payload["created_at"] != "2026-06-30T09:02:03.123400+00:00" || payload["trace_id"] != "trace-sdk-1" {
		t.Fatalf("time/trace payload fields = %#v", payload)
	}
	if payload["device_id"] != "p1-slot-18" || payload["receiver"] != "Qiu" || payload["text"] != "hi" {
		t.Fatalf("flat payload fields = %#v", payload)
	}
	target, ok := payload["target"].(map[string]any)
	if !ok || target["agent_id"] != "sdk:p1-slot-18" || target["device_id"] != "p1-slot-18" {
		t.Fatalf("target payload = %#v", payload["target"])
	}
	nested, ok := payload["payload"].(map[string]any)
	if !ok || nested["task_id"] != "payload-task" || nested["device_id"] != "payload-device" {
		t.Fatalf("nested payload = %#v", payload["payload"])
	}
	nested["text"] = "changed"
	if record.Payload["text"] != "hi" {
		t.Fatalf("record payload was mutated: %#v", record.Payload)
	}
}

// TestSDKExecutorBatchFuncUsesSingleExecuteAndFinalizesSuccess mirrors Python len==1 path.
func TestSDKExecutorBatchFuncUsesSingleExecuteAndFinalizesSuccess(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	executor := &recordingSDKExecutor{executeResult: SDKExecutorResult{"success": true, "result": map[string]any{"action": "send_text"}}}
	delivery := &recordingTerminalDelivery{}
	publisher := &recordingTaskStatusPublisher{}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		Now: func() time.Time { return now },
		Terminal: TerminalStateSyncOptions{
			Delivery: delivery,
			Status:   publisher,
		},
	})

	finalized, err := executeBatch(context.Background(), " zimo ", []tasks.Record{executorRecord("task-1", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(executor.executeCalls) != 1 || len(executor.batchCalls) != 0 {
		t.Fatalf("executeCalls=%d batchCalls=%d", len(executor.executeCalls), len(executor.batchCalls))
	}
	if executor.executeCalls[0]["device_id"] != "zimo" || executor.executeCalls[0]["receiver"] != "Qiu" {
		t.Fatalf("captured payload = %#v", executor.executeCalls[0])
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess || finalized[0].Error != nil {
		t.Fatalf("finalized = %#v", finalized)
	}
	if finalized[0].DispatchedAt == nil || !finalized[0].DispatchedAt.Equal(now) || finalized[0].ScriptStartedAt == nil || !finalized[0].UpdatedAt.Equal(now) {
		t.Fatalf("finalized timestamps = %#v", finalized[0])
	}
	if len(delivery.updates) != 1 || delivery.updates[0].SendStatus != "success" {
		t.Fatalf("delivery updates = %#v", delivery.updates)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %#v", publisher.events)
	}
	resultPayload := publisher.events[0].Payload["result_payload"].(map[string]any)
	if resultPayload["source"] != "sdk_executor" || resultPayload["success"] != true || resultPayload["sdk_failure_stage"] != nil {
		t.Fatalf("result payload = %#v", resultPayload)
	}
}

// TestSDKExecutorBatchFuncUsesBatchAndMarksMissingResultsFailed mirrors Python batch fallback.
func TestSDKExecutorBatchFuncUsesBatchAndMarksMissingResultsFailed(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	executor := &recordingSDKExecutor{batchResult: []SDKExecutorResult{{"success": true}}}
	publisher := &recordingTaskStatusPublisher{}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		Now:      func() time.Time { return now },
		Terminal: TerminalStateSyncOptions{Status: publisher},
	})

	finalized, err := executeBatch(context.Background(), "zimo", []tasks.Record{
		executorRecord("task-1", 0),
		executorRecord("task-2", 1),
	})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(executor.executeCalls) != 0 || len(executor.batchCalls) != 1 || len(executor.batchCalls[0]) != 2 {
		t.Fatalf("executeCalls=%d batchCalls=%#v", len(executor.executeCalls), executor.batchCalls)
	}
	if len(finalized) != 2 || finalized[0].Status != tasks.StatusSuccess || finalized[1].Status != tasks.StatusFailed {
		t.Fatalf("finalized = %#v", finalized)
	}
	if finalized[1].Error == nil || *finalized[1].Error != "sdk batch result missing" {
		t.Fatalf("missing result error = %#v", finalized[1].Error)
	}
	if len(publisher.events) != 2 {
		t.Fatalf("published events = %#v", publisher.events)
	}
	firstPayload := publisher.events[0].Payload["result_payload"].(map[string]any)
	secondPayload := publisher.events[1].Payload["result_payload"].(map[string]any)
	if firstPayload["source"] != "sdk_executor_batch" || firstPayload["success"] != true {
		t.Fatalf("first result payload = %#v", firstPayload)
	}
	if secondPayload["source"] != "sdk_executor_batch" || secondPayload["error"] != "sdk batch result missing" {
		t.Fatalf("second result payload = %#v", secondPayload)
	}
}

// TestSDKExecutorBatchFuncWritesTerminalStatusBeforePublish mirrors DB finalization order.
func TestSDKExecutorBatchFuncWritesTerminalStatusBeforePublish(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	events := []string{}
	writer := &recordingSDKStatusWriter{events: &events}
	health := &recordingSDKDeviceHealthRecorder{events: &events}
	publisher := &recordingTaskStatusPublisher{order: &events}
	delivery := &recordingTerminalDelivery{events: &events}
	executor := &recordingSDKExecutor{executeResult: SDKExecutorResult{"success": false, "error": "phone offline"}}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		Now:          func() time.Time { return now },
		StatusWriter: writer,
		DeviceHealth: health,
		Terminal: TerminalStateSyncOptions{
			Delivery: delivery,
			Status:   publisher,
		},
	})

	finalized, err := executeBatch(context.Background(), "zimo", []tasks.Record{executorRecord("task-1", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusFailed || finalized[0].Error == nil || *finalized[0].Error != "phone offline" {
		t.Fatalf("finalized = %#v", finalized)
	}
	if len(writer.updates) != 1 || writer.updates[0].Status != tasks.StatusFailed || writer.updates[0].Error == nil {
		t.Fatalf("writer updates = %#v", writer.updates)
	}
	if writer.updates[0].DispatchedAt == nil || !writer.updates[0].DispatchedAt.Equal(now) || writer.updates[0].UpdatedAt == nil || !writer.updates[0].UpdatedAt.Equal(now) {
		t.Fatalf("writer timestamps = %#v", writer.updates[0])
	}
	if len(events) != 3 || events[0] != "update:failed" || events[1] != "health:failed" || events[2] != "publish:task.status" {
		t.Fatalf("events = %#v", events)
	}
	if len(health.records) != 1 || health.records[0].DeviceID != "zimo" || health.records[0].TaskID != "task-1" || health.records[0].Error != "phone offline" {
		t.Fatalf("health records = %#v", health.records)
	}
	if len(delivery.updates) != 0 {
		t.Fatalf("delivery should be handled by status writer, got %#v", delivery.updates)
	}
	resultPayload := publisher.events[0].Payload["result_payload"].(map[string]any)
	if resultPayload["sdk_failure_stage"] != "unknown" || resultPayload["sdk_failure_retry_policy"] != "manual_review" || resultPayload["sdk_failure_commit_risk"] != "unknown" {
		t.Fatalf("result payload = %#v", resultPayload)
	}
}

// TestSDKExecutorBatchFuncSyncsCommitUnknownAIArchiveConfirmation mirrors archive wait semantics.
func TestSDKExecutorBatchFuncSyncsCommitUnknownAIArchiveConfirmation(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	ai := &recordingAITerminalSyncer{}
	executor := &recordingSDKExecutor{executeResult: SDKExecutorResult{
		"success": false,
		"error":   "wait_chat_compose_ready timeout context=album_send",
	}}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		Now:      func() time.Time { return now },
		Terminal: TerminalStateSyncOptions{AI: ai},
	})

	finalized, err := executeBatch(context.Background(), "zimo", []tasks.Record{executorRecord("task-archive-confirming", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusFailed {
		t.Fatalf("finalized = %#v", finalized)
	}
	if len(ai.updates) != 1 {
		t.Fatalf("ai updates = %#v", ai.updates)
	}
	update := ai.updates[0]
	if update.AttemptStatus != "archive_confirming" || update.FailureType != "commit_unknown_wait_archive" || update.FinishedAt != nil {
		t.Fatalf("ai update = %#v", update)
	}
	if update.RuntimeStatus != "sending" || update.RuntimePhase != "archive_confirmation_pending" || !update.KeepProcessingStartedAt {
		t.Fatalf("ai runtime update = %#v", update)
	}
}

// TestSDKExecutorBatchFuncRetriesTransportAcquireOnce mirrors Python safe pre-commit retry.
func TestSDKExecutorBatchFuncRetriesTransportAcquireOnce(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	writer := &recordingSDKStatusWriter{}
	publisher := &recordingTaskStatusPublisher{}
	executor := &sequenceSDKExecutor{results: []SDKExecutorResult{
		{"success": false, "error": "P1 device p1-slot-18 connection failed"},
		{"success": true, "result": map[string]any{"action": "send_text"}},
	}}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		Now:          func() time.Time { return now },
		StatusWriter: writer,
		Terminal:     TerminalStateSyncOptions{Status: publisher},
	})

	finalized, err := executeBatch(context.Background(), "p1-slot-18", []tasks.Record{executorRecord("task-transport-retry", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(executor.calls) != 2 || executor.calls[1]["sdk_transport_retry_attempted"] != true {
		t.Fatalf("executor calls = %#v", executor.calls)
	}
	if len(writer.updates) != 2 || writer.updates[0].Status != tasks.StatusRunning || writer.updates[1].Status != tasks.StatusSuccess {
		t.Fatalf("writer updates = %#v", writer.updates)
	}
	if writer.updates[0].Error == nil || *writer.updates[0].Error != "sdk transport_acquire retrying after: P1 device p1-slot-18 connection failed" {
		t.Fatalf("retry status error = %#v", writer.updates[0].Error)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
	resultPayload := publisher.events[0].Payload["result_payload"].(map[string]any)
	if resultPayload["source"] != "sdk_executor" || resultPayload["pre_commit_retry"] != true || resultPayload["pre_commit_retry_kind"] != "transport_acquire" {
		t.Fatalf("result payload = %#v", resultPayload)
	}
}

// TestSDKExecutorBatchFuncRetriesComposeSurfaceOnce marks compose retry payloads.
func TestSDKExecutorBatchFuncRetriesComposeSurfaceOnce(t *testing.T) {
	executor := &sequenceSDKExecutor{results: []SDKExecutorResult{
		{"success": false, "error": "type_message input box not found"},
		{"success": true, "result": map[string]any{"action": "send_text"}},
	}}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{})

	finalized, err := executeBatch(context.Background(), "p1-slot-18", []tasks.Record{executorRecord("task-compose-retry", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(executor.calls) != 2 || executor.calls[1]["sdk_compose_surface_retry_attempted"] != true {
		t.Fatalf("executor calls = %#v", executor.calls)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
}

// TestSDKExecutorBatchFuncRetriesTransientNavigationOnce mirrors retry_same_payload_once.
func TestSDKExecutorBatchFuncRetriesTransientNavigationOnce(t *testing.T) {
	writer := &recordingSDKStatusWriter{}
	executor := &sequenceSDKExecutor{results: []SDKExecutorResult{
		{"success": false, "error": "navigate_to_chat input_search failed receiver=Qiu"},
		{"success": true, "result": map[string]any{"action": "send_text"}},
	}}
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{StatusWriter: writer})

	finalized, err := executeBatch(context.Background(), "p1-slot-18", []tasks.Record{executorRecord("task-navigation-retry", 0)})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(executor.calls) != 2 || executor.calls[1]["sdk_navigation_retry_attempted"] != true {
		t.Fatalf("executor calls = %#v", executor.calls)
	}
	if len(writer.updates) != 2 || writer.updates[0].Status != tasks.StatusRunning || writer.updates[1].Status != tasks.StatusSuccess {
		t.Fatalf("writer updates = %#v", writer.updates)
	}
	if writer.updates[0].Error == nil || *writer.updates[0].Error != "sdk transient navigation retrying after: navigate_to_chat input_search failed receiver=Qiu" {
		t.Fatalf("retry status error = %#v", writer.updates[0].Error)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
}

// TestSDKExecutorBatchFuncRetriesAfterContactRefresh mirrors refreshed receiver retry.
func TestSDKExecutorBatchFuncRetriesAfterContactRefresh(t *testing.T) {
	writer := &recordingSDKStatusWriter{}
	publisher := &recordingTaskStatusPublisher{}
	resolver := &recordingSDKContactRetryResolver{target: SDKContactRetryTarget{Receiver: "fresh", Aliases: "fresh-alias"}}
	executor := &sequenceSDKExecutor{results: []SDKExecutorResult{
		{"success": false, "error": "navigate_to_chat search_result not found receiver=old tried=old"},
		{"success": true, "result": map[string]any{"action": "send_text"}},
	}}
	record := executorRecord("task-contact-retry", 0)
	record.Payload["receiver"] = "old"
	record.Payload["username"] = "old"
	executeBatch := NewSDKExecutorBatchFunc(executor, SDKExecutorAdapterOptions{
		StatusWriter: writer,
		ContactRetry: resolver,
		Terminal:     TerminalStateSyncOptions{Status: publisher},
	})

	finalized, err := executeBatch(context.Background(), "p1-slot-18", []tasks.Record{record})
	if err != nil {
		t.Fatalf("execute batch returned error: %v", err)
	}
	if len(resolver.requests) != 1 || resolver.requests[0].OriginalError != "navigate_to_chat search_result not found receiver=old tried=old" {
		t.Fatalf("resolver requests = %#v", resolver.requests)
	}
	if len(executor.calls) != 2 || executor.calls[1]["receiver"] != "fresh" || executor.calls[1]["aliases"] != "fresh-alias" || executor.calls[1]["sdk_contact_retry_attempted"] != true {
		t.Fatalf("executor calls = %#v", executor.calls)
	}
	if len(writer.updates) != 2 || writer.updates[0].Status != tasks.StatusRunning || writer.updates[1].Status != tasks.StatusSuccess {
		t.Fatalf("writer updates = %#v", writer.updates)
	}
	if writer.updates[0].Error == nil || *writer.updates[0].Error != "sdk contact refresh retrying after: navigate_to_chat search_result not found receiver=old tried=old" {
		t.Fatalf("retry status error = %#v", writer.updates[0].Error)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
	resultPayload := publisher.events[0].Payload["result_payload"].(map[string]any)
	if resultPayload["contact_retry"] != true || resultPayload["contact_retry_receiver"] != "fresh" {
		t.Fatalf("result payload = %#v", resultPayload)
	}
}

// TestSDKExecutorBatchFuncRequiresBatchExecutor keeps multi-task bursts fail-closed.
func TestSDKExecutorBatchFuncRequiresBatchExecutor(t *testing.T) {
	executeBatch := NewSDKExecutorBatchFunc(singleOnlySDKExecutor{}, SDKExecutorAdapterOptions{})

	_, err := executeBatch(context.Background(), "zimo", []tasks.Record{executorRecord("task-1", 0), executorRecord("task-2", 1)})
	if err == nil || !strings.Contains(err.Error(), "execute_batch") {
		t.Fatalf("error = %v", err)
	}
}

func executorRecord(taskID string, index int) tasks.Record {
	return tasks.Record{
		TaskID:    taskID,
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:zimo", DeviceID: "zimo"},
		TaskType:  "send_text",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 30, 9, 0, index, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 30, 9, 0, index, 0, time.UTC),
		Payload: map[string]any{
			"receiver":           "Qiu",
			"text":               "hi",
			"conversation_id":    "conversation-1",
			"sender_id":          "sender-1",
			"client_batch_index": index,
		},
	}
}

type recordingSDKExecutor struct {
	executeResult SDKExecutorResult
	batchResult   []SDKExecutorResult
	executeCalls  []SDKTaskPayload
	batchCalls    [][]SDKTaskPayload
}

func (executor *recordingSDKExecutor) Execute(_ context.Context, task SDKTaskPayload) (SDKExecutorResult, error) {
	executor.executeCalls = append(executor.executeCalls, task)
	return executor.executeResult, nil
}

func (executor *recordingSDKExecutor) ExecuteBatch(_ context.Context, tasks []SDKTaskPayload) ([]SDKExecutorResult, error) {
	executor.batchCalls = append(executor.batchCalls, tasks)
	return executor.batchResult, nil
}

type sequenceSDKExecutor struct {
	results []SDKExecutorResult
	calls   []SDKTaskPayload
}

func (executor *sequenceSDKExecutor) Execute(_ context.Context, task SDKTaskPayload) (SDKExecutorResult, error) {
	executor.calls = append(executor.calls, task)
	index := len(executor.calls) - 1
	if index >= 0 && index < len(executor.results) {
		return executor.results[index], nil
	}
	return SDKExecutorResult{"success": false, "error": "sdk sequence result missing"}, nil
}

type recordingSDKContactRetryResolver struct {
	target   SDKContactRetryTarget
	err      error
	requests []SDKContactRetryRequest
}

func (resolver *recordingSDKContactRetryResolver) ResolveSDKContactRetry(_ context.Context, request SDKContactRetryRequest) (SDKContactRetryTarget, error) {
	resolver.requests = append(resolver.requests, request)
	return resolver.target, resolver.err
}

type singleOnlySDKExecutor struct{}

func (singleOnlySDKExecutor) Execute(context.Context, SDKTaskPayload) (SDKExecutorResult, error) {
	return SDKExecutorResult{"success": true}, nil
}

type recordingSDKStatusWriter struct {
	taskIDs []string
	updates []tasks.StatusUpdate
	events  *[]string
}

func (writer *recordingSDKStatusWriter) UpdateTerminalStatus(_ context.Context, taskID string, update tasks.StatusUpdate) (tasks.Record, error) {
	writer.taskIDs = append(writer.taskIDs, taskID)
	writer.updates = append(writer.updates, update)
	if writer.events != nil {
		*writer.events = append(*writer.events, "update:"+string(update.Status))
	}
	record := tasks.Record{
		TaskID:          taskID,
		Status:          update.Status,
		Error:           update.Error,
		UpdatedAt:       dereferenceTime(update.UpdatedAt),
		DispatchedAt:    cloneTimePointer(update.DispatchedAt),
		ScriptStartedAt: cloneTimePointer(update.ScriptStartedAt),
	}
	return record, nil
}

func dereferenceTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

type recordingSDKDeviceHealthRecorder struct {
	records []SDKDeviceTaskResult
	events  *[]string
}

func (recorder *recordingSDKDeviceHealthRecorder) RecordSDKDeviceTaskResult(_ context.Context, record SDKDeviceTaskResult) error {
	recorder.records = append(recorder.records, record)
	if recorder.events != nil {
		status := "success"
		if !record.Success {
			status = "failed"
		}
		*recorder.events = append(*recorder.events, "health:"+status)
	}
	return nil
}
