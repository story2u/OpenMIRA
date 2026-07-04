package taskstore

import (
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestSDKDispatchBatchGroupKeyMatchesLegacyRules freezes same-chat grouping.
func TestSDKDispatchBatchGroupKeyMatchesLegacyRules(t *testing.T) {
	record := batchTask("task-golden-0001", "send_text", map[string]any{
		"username":        " Qiu ",
		"aliases":         "alias-1",
		"entity":          "person",
		"session_id":      "conversation-1",
		"sender_id":       "sender-1",
		"client_batch_id": "batch-1",
	})

	key, ok := sdkDispatchBatchGroupKey(record)
	if !ok {
		t.Fatalf("expected group key")
	}
	if key != (sdkDispatchTargetKey{"Qiu", "alias-1", "person", "conversation-1", "sender-1"}) {
		t.Fatalf("group key = %#v", key)
	}
	if _, ok := sdkDispatchBatchGroupKey(batchTask("image-1", "send_image", map[string]any{"username": "Qiu"})); ok {
		t.Fatal("send_image was batchable")
	}
	if _, ok := sdkDispatchBatchGroupKey(batchTask("text-2", "send_text", map[string]any{"username": "Qiu", "preserve_individual_send": true})); ok {
		t.Fatal("preserve_individual_send task was batchable")
	}
	if _, ok := sdkDispatchBatchGroupKey(batchTask("text-3", "send_text", map[string]any{"receiver": " "})); ok {
		t.Fatal("missing receiver was batchable")
	}
}

// TestBuildSDKDispatchFollowupSelectMatchesLegacyFilters freezes followup predicates.
func TestBuildSDKDispatchFollowupSelectMatchesLegacyFilters(t *testing.T) {
	sql, args, err := BuildSDKDispatchFollowupSelect(SDKDispatchFollowupQuery{
		FirstTask:           tasks.Record{Target: tasks.Target{DeviceID: " zimo "}},
		TaskTypes:           []string{"send_text", "unsupported", "send_image"},
		Limit:               5,
		ForUpdateSkipLocked: true,
	})
	if err != nil {
		t.Fatalf("BuildSDKDispatchFollowupSelect returned error: %v", err)
	}
	for _, fragment := range []string{
		"FROM tasks",
		"status = ?",
		"target_device_id = ?",
		"target_agent_id LIKE ?",
		"task_type IN (?, ?)",
		"payload_json LIKE ? OR payload_json LIKE ?",
		"created_at ASC",
		"task_id ASC",
		"LIMIT ? FOR UPDATE SKIP LOCKED",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sql missing %q: %s", fragment, sql)
		}
	}
	wantArgs := []any{
		"accepted",
		"zimo",
		"sdk:%",
		"send_text",
		"send_image",
		`%"queue": "fast"%`,
		`%"queue":"fast"%`,
		`%"queue": "slow"%`,
		`%"queue":"slow"%`,
		5,
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("arg[%d] = %#v, want %#v; args=%#v", index, args[index], wantArgs[index], args)
		}
	}
}

// TestBuildSDKDispatchFollowupSelectRejectsAmbiguousInput keeps scope explicit.
func TestBuildSDKDispatchFollowupSelectRejectsAmbiguousInput(t *testing.T) {
	if _, _, err := BuildSDKDispatchFollowupSelect(SDKDispatchFollowupQuery{FirstTask: tasks.Record{Target: tasks.Target{DeviceID: "zimo"}}}); err == nil || !strings.Contains(err.Error(), "batchable task_types is required") {
		t.Fatalf("missing task types error = %v", err)
	}
	if _, _, err := BuildSDKDispatchFollowupSelect(SDKDispatchFollowupQuery{TaskTypes: []string{"send_text"}}); err == nil || !strings.Contains(err.Error(), "first task device_id is required") {
		t.Fatalf("missing device error = %v", err)
	}
}

// TestSDKDispatchSkipBoundaryAndQueueChannel mirrors sticky followup gates.
func TestSDKDispatchSkipBoundaryAndQueueChannel(t *testing.T) {
	first := batchTask("task-1", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1", "queue": "slow"})
	candidates := []tasks.Record{
		batchTaskAt("task-3", "send_text", map[string]any{"username": "Ada", "conversation_id": "c2"}, time.Date(2026, 6, 29, 9, 3, 0, 0, time.UTC)),
		batchTaskAt("task-2", "send_image", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 2, 0, 0, time.UTC)),
		batchTaskAt("task-4", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1", "preserve_individual_send": "false"}, time.Date(2026, 6, 29, 9, 4, 0, 0, time.UTC)),
	}

	order, ok := sdkDispatchFirstSkipBoundaryOrder(first, candidates)
	if !ok || order.taskID != "task-2" {
		t.Fatalf("boundary order = %#v ok=%t", order, ok)
	}
	if channel := sdkDispatchQueueChannel(first); channel != "slow" {
		t.Fatalf("channel = %q", channel)
	}
	if channel := sdkDispatchQueueChannel(batchTask("task-5", "send_text", map[string]any{"username": "Qiu", "queue": "FAST"})); channel != "fast" {
		t.Fatalf("default channel = %q", channel)
	}
}

func batchTask(taskID string, taskType string, payload map[string]any) tasks.Record {
	return batchTaskAt(taskID, taskType, payload, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
}

func batchTaskAt(taskID string, taskType string, payload map[string]any, createdAt time.Time) tasks.Record {
	return tasks.Record{
		TaskID:    taskID,
		TaskType:  taskType,
		Target:    tasks.Target{AgentID: "sdk:zimo", DeviceID: "zimo"},
		Payload:   payload,
		Status:    tasks.StatusAccepted,
		CreatedAt: createdAt,
	}
}
