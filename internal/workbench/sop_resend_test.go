package workbench

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/tasks"
)

func TestNewSOPDispatchResendRequestValidatesLegacyBody(t *testing.T) {
	session := auth.Session{Role: "admin"}
	if _, err := NewSOPDispatchResendRequest(SOPDispatchResendBody{}, session); !errors.Is(err, ErrSOPResendFlowIDRequired) {
		t.Fatalf("flow_id error = %v", err)
	}
	if _, err := NewSOPDispatchResendRequest(SOPDispatchResendBody{FlowID: "formal"}, session); !errors.Is(err, ErrSOPResendTaskIDRequired) {
		t.Fatalf("task_id error = %v", err)
	}

	request, err := NewSOPDispatchResendRequest(SOPDispatchResendBody{
		FlowID:  " formal ",
		TaskID:  " task-2 ",
		TaskIDs: []any{" task-1 ", "task-1"},
	}, session)
	if err != nil {
		t.Fatalf("NewSOPDispatchResendRequest returned error: %v", err)
	}
	if request.Query.FlowID != "formal" || request.Query.Limit != 100 || request.Session.Role != "admin" {
		t.Fatalf("request = %+v", request)
	}
	if want := []string{"task-1", "task-2"}; !reflect.DeepEqual(request.Query.TaskIDs, want) {
		t.Fatalf("task ids = %#v, want %#v", request.Query.TaskIDs, want)
	}

	request, err = NewSOPDispatchResendRequest(SOPDispatchResendBody{FlowID: "formal", AllFailed: true, Limit: float64(5)}, session)
	if err != nil {
		t.Fatalf("all_failed request returned error: %v", err)
	}
	if !request.Query.AllFailed || len(request.Query.TaskIDs) != 0 || request.Query.Limit != 5 {
		t.Fatalf("all_failed request = %+v", request)
	}
}

func TestSOPDispatchTasksResendGroupsRowsAndMarksQueued(t *testing.T) {
	store := &fakeSOPResendStore{rows: []ProjectionRow{
		{
			"fact_id":             "fact-1",
			"task_id":             "task-1",
			"source_payload_json": `{"actions":[{"type":"text","content":"hello","stage_unique_id":"stage-1"}]}`,
		},
		{
			"fact_id":             "fact-2",
			"task_id":             "task-1",
			"source_payload_json": `{"actions":[{"stage_unique_id":"stage-1","content":"hello","type":"text"}]}`,
		},
	}}
	executor := &fakeSOPResendExecutor{record: tasks.Record{TaskID: "sop-resend-fixed", Status: tasks.StatusAccepted}}
	service := Service{
		SOPDispatchResendStore:    store,
		SOPDispatchResendExecutor: executor,
		Now: func() time.Time {
			return time.Date(2026, 6, 28, 17, 30, 0, 0, time.UTC)
		},
	}

	payload, err := service.SOPDispatchTasksResend(context.Background(), SOPDispatchResendRequest{Query: SOPDispatchResendQuery{FlowID: "formal", TaskIDs: []string{"task-1"}, Limit: 100}})
	if err != nil {
		t.Fatalf("SOPDispatchTasksResend returned error: %v", err)
	}
	if store.query.Date != "2026-06-29" || store.query.FlowID != "formal" || !reflect.DeepEqual(store.query.TaskIDs, []string{"task-1"}) {
		t.Fatalf("query = %+v", store.query)
	}
	if payload["success"] != true || payload["requested"] != 1 || payload["succeeded"] != 1 || payload["failed"] != 0 {
		t.Fatalf("payload = %+v", payload)
	}
	if len(executor.groups) != 1 || len(executor.groups[0].Actions) != 1 {
		t.Fatalf("groups = %+v", executor.groups)
	}
	if len(store.marked) != 1 || store.marked[0].original != "task-1" || store.marked[0].resend != "sop-resend-fixed" {
		t.Fatalf("marked = %+v", store.marked)
	}
	results := payload["results"].([]ProjectionRow)
	if len(results) != 1 || results[0]["resend_task_id"] != "sop-resend-fixed" || results[0]["status"] != "accepted" {
		t.Fatalf("results = %+v", results)
	}
}

func TestSOPDispatchTaskExecutorBuildsMixedMessageTask(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 3, 0, 0, 0, time.UTC)
	creator := &fakeTaskCreator{}
	counter := 0
	executor := SOPDispatchTaskExecutor{
		Tasks: creator,
		Now: func() time.Time {
			return createdAt
		},
		NewID: func(prefix string) string {
			counter++
			return prefix + string(rune('0'+counter))
		},
	}

	record, err := executor.ResendSOPDispatch(context.Background(), SOPDispatchResendGroup{
		Row: ProjectionRow{
			"fact_id":             "fact-1",
			"task_id":             "task-1",
			"device_id":           "dev-1",
			"agent_id":            "agent-1",
			"conversation_key":    "receiver-1",
			"external_userid":     "external-1",
			"conversation_id":     "conv-1",
			"enterprise_id":       "ent-1",
			"flow_id":             "formal",
			"flow_name":           "正式 SOP",
			"assignee_id":         "assignee-1",
			"assignee_name":       "客服 A",
			"day_stage":           "day3",
			"customer_state":      "silent",
			"customer_stage_tag":  "tag-a",
			"source_payload_json": `{"aliases":"客户备注"}`,
		},
		Actions: []map[string]string{
			{"type": "text", "content": "hello", "stage_unique_id": "stage-1"},
			{"type": "image", "content": "pic", "url": "https://example.test/pic.jpg"},
		},
	})
	if err != nil {
		t.Fatalf("ResendSOPDispatch returned error: %v", err)
	}
	if record.TaskID != "sop-resend-1" || record.Status != tasks.StatusAccepted {
		t.Fatalf("record = %+v", record)
	}
	if len(creator.requests) != 1 {
		t.Fatalf("requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "sop-resend-1" || request.TaskType != "send_mixed_messages" || request.Source != "system" || request.Target.AgentID != "agent-1" || request.Target.DeviceID != "dev-1" {
		t.Fatalf("task request = %+v", request)
	}
	if request.TraceID == nil || *request.TraceID != "platform-pull-send-2" || request.EnterpriseID == nil || *request.EnterpriseID != "ent-1" {
		t.Fatalf("trace/enterprise = trace=%v enterprise=%v", request.TraceID, request.EnterpriseID)
	}
	payload := request.Payload
	if payload["username"] != "receiver-1" || payload["receiver"] != "receiver-1" || payload["entity"] != "ent-1" || payload["msg_id"] != "sop-resend-1" || payload["queue"] != "slow" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["aliases"] != "客户备注" {
		t.Fatalf("aliases = %v", payload["aliases"])
	}
	policy := payload["_send_policy"].(map[string]any)
	if policy["origin"] != "sop" || policy["source_enabled"] != true || policy["trigger_event"] != "manual_resend" || policy["flow_id"] != "formal" {
		t.Fatalf("send policy = %+v", policy)
	}
	audit := payload["sop_audit"].(map[string]any)
	if audit["source"] != "platform_pull" || audit["original_task_id"] != "task-1" || audit["ai_trace_id"] != "sop-resend-3" || audit["trigger_event"] != "manual_resend" {
		t.Fatalf("audit = %+v", audit)
	}
	messages := payload["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["content"] != "hello" || messages[1].(map[string]any)["url"] != "https://example.test/pic.jpg" {
		t.Fatalf("messages = %+v", messages)
	}
}

type fakeSOPResendStore struct {
	rows   []ProjectionRow
	query  SOPDispatchResendQuery
	marked []struct {
		original string
		resend   string
	}
}

func (store *fakeSOPResendStore) ListFailedSOPResendCandidates(ctx context.Context, query SOPDispatchResendQuery) ([]ProjectionRow, error) {
	store.query = query
	return store.rows, nil
}

func (store *fakeSOPResendStore) MarkSOPResendQueued(ctx context.Context, originalTaskID string, resendTaskID string) error {
	store.marked = append(store.marked, struct {
		original string
		resend   string
	}{original: originalTaskID, resend: resendTaskID})
	return nil
}

type fakeSOPResendExecutor struct {
	record tasks.Record
	groups []SOPDispatchResendGroup
}

func (executor *fakeSOPResendExecutor) ResendSOPDispatch(ctx context.Context, group SOPDispatchResendGroup) (tasks.Record, error) {
	executor.groups = append(executor.groups, group)
	return executor.record, nil
}

type fakeTaskCreator struct {
	requests []tasks.CreateRequest
}

func (creator *fakeTaskCreator) Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	creator.requests = append(creator.requests, request)
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}
