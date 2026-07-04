// SOP analytics service tests pin route-level defaults and payload shapes.
// They keep the candidate scoped to read-only fact summaries and detail pages.
package workbench

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"wework-go/internal/auth"
)

// TestServiceSOPAnalyticsStageStatsBuildsPayload checks date default and shape.
func TestServiceSOPAnalyticsStageStatsBuildsPayload(t *testing.T) {
	store := &fakeSOPAnalyticsStore{stageItems: []ProjectionRow{{
		"flow_id":                  "formal",
		"stage_unique_id":          "stage-1",
		"delivered_customer_count": 10,
		"customer_open_rate":       0.3,
	}}}
	service := Service{
		SOPAnalyticsStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.SOPAnalyticsStageStats(context.Background(), NewSOPStageStatsRequest(url.Values{"flow_id": {"formal"}}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("SOPAnalyticsStageStats returned error: %v", err)
	}
	if payload["date"] != "2026-06-29" || payload["flow_id"] != "formal" {
		t.Fatalf("payload metadata = %+v", payload)
	}
	if store.stageQuery.Date != "2026-06-29" || store.stageQuery.FlowID != "formal" {
		t.Fatalf("stage query = %+v", store.stageQuery)
	}
	items := payload["items"].([]ProjectionRow)
	if len(items) != 1 || rowText(items[0], "stage_unique_id") != "stage-1" {
		t.Fatalf("items = %+v", items)
	}
}

// TestServiceSOPAnalyticsFactsBuildsPayload keeps facts pagination stable.
func TestServiceSOPAnalyticsFactsBuildsPayload(t *testing.T) {
	store := &fakeSOPAnalyticsStore{factsPage: SOPFactsPage{
		Items: []ProjectionRow{{"fact_id": "fact-1", "delivery_status": "success"}},
		Pagination: ProjectionRow{
			"page":        2,
			"page_size":   30,
			"total":       31,
			"total_pages": 2,
		},
	}}
	service := Service{SOPAnalyticsStore: store}
	request, err := NewSOPFactsRequest(url.Values{
		"date":            {"2026-06-28Tignored"},
		"flow_id":         {"formal"},
		"stage_unique_id": {"stage-1"},
		"status":          {"opened"},
		"keyword":         {"trace"},
		"page":            {"2"},
		"page_size":       {"30"},
	}, auth.Session{Role: "supervisor"})
	if err != nil {
		t.Fatalf("NewSOPFactsRequest returned error: %v", err)
	}

	payload, err := service.SOPAnalyticsFacts(context.Background(), request)
	if err != nil {
		t.Fatalf("SOPAnalyticsFacts returned error: %v", err)
	}
	if store.factsQuery.Date != "2026-06-28Tignored" || store.factsQuery.Status != "opened" || store.factsQuery.Page != 2 || store.factsQuery.PageSize != 30 {
		t.Fatalf("facts query = %+v", store.factsQuery)
	}
	items := payload["items"].([]ProjectionRow)
	if len(items) != 1 || rowText(items[0], "fact_id") != "fact-1" {
		t.Fatalf("items = %+v", items)
	}
	if payload["pagination"].(ProjectionRow)["total"] != 31 {
		t.Fatalf("pagination = %+v", payload["pagination"])
	}
}

// TestServiceSOPDispatchTasksBuildsPayload maps fact groups to legacy batches.
func TestServiceSOPDispatchTasksBuildsPayload(t *testing.T) {
	store := &fakeSOPAnalyticsStore{taskBatchesPage: SOPTaskBatchesPage{
		Items: []SOPTaskBatchGroup{{
			BatchKey: "task-1",
			Rows: []ProjectionRow{{
				"fact_id":             "fact-1",
				"task_id":             "task-1",
				"batch_id":            "batch-1",
				"flow_id":             "formal",
				"flow_name":           "正式 SOP",
				"conversation_id":     "ww:wu-1:external-1",
				"conversation_key":    "客户 A",
				"external_userid":     "external-1",
				"delivery_status":     "failed",
				"delivery_error":      "network timeout",
				"message_count":       int64(1),
				"source_payload_json": `{"actions":[{"type":"text","content":"hello"}],"original_task_id":"task-0","auto_resend_attempt":1,"auto_resend_reason":"retry"}`,
				"queued_at":           "2026-06-29T02:00:00Z",
				"failed_at":           "2026-06-29T02:01:00Z",
			}},
		}},
		Pagination: ProjectionRow{"page": 1, "page_size": 30, "total": 1, "total_pages": 1},
	}}
	service := Service{
		SOPAnalyticsStore: store,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 0, 0, 0, time.UTC)
		},
	}
	request, err := NewSOPDispatchTasksRequest(url.Values{
		"flow_id":   {"formal"},
		"status":    {"FAILED"},
		"keyword":   {"客户"},
		"page":      {"1"},
		"page_size": {"30"},
	}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewSOPDispatchTasksRequest returned error: %v", err)
	}

	payload, err := service.SOPDispatchTasks(context.Background(), request)
	if err != nil {
		t.Fatalf("SOPDispatchTasks returned error: %v", err)
	}
	if store.taskBatchesQuery.Date != "2026-06-29" || store.taskBatchesQuery.Status != "failed" || store.taskBatchesQuery.Keyword != "客户" {
		t.Fatalf("query = %+v", store.taskBatchesQuery)
	}
	batches := payload["batches"].([]ProjectionRow)
	if len(batches) != 1 || batches[0]["task_id"] != "task-1" || batches[0]["task_status"] != "failed" || batches[0]["can_resend"] != true || batches[0]["trigger_event"] != "auto_resend" {
		t.Fatalf("batches = %+v", batches)
	}
	if batches[0]["created_at"] != "2026-06-29T10:00:00+08:00" || batches[0]["completed_at"] != "2026-06-29T10:01:00+08:00" {
		t.Fatalf("batch times = %+v", batches[0])
	}
	details := batches[0]["details"].([]ProjectionRow)
	if len(details) != 1 || details[0]["task_id"] != "task-1" || details[0]["customer_replied"] != false {
		t.Fatalf("details = %+v", details)
	}
	messageDetails := details[0]["message_details"].([]ProjectionRow)
	if len(messageDetails) != 1 || messageDetails[0]["content"] != "hello" {
		t.Fatalf("message details = %+v", messageDetails)
	}
	tasks := payload["tasks"].([]ProjectionRow)
	if len(tasks) != 1 || tasks[0]["task_id"] != "task-1" {
		t.Fatalf("tasks = %+v", tasks)
	}
}

// TestServiceSOPDispatchTasksEnrichesReceiverAndAccountDisplay keeps local display hydration compatible.
func TestServiceSOPDispatchTasksEnrichesReceiverAndAccountDisplay(t *testing.T) {
	store := &fakeSOPAnalyticsStore{taskBatchesPage: SOPTaskBatchesPage{
		Items: []SOPTaskBatchGroup{{
			BatchKey: "task-1",
			Rows: []ProjectionRow{{
				"fact_id":             "fact-1",
				"task_id":             "task-1",
				"flow_id":             "formal",
				"conversation_id":     "ww:wu-1:external-1",
				"conversation_key":    "external-1",
				"external_userid":     "external-1",
				"delivery_status":     "success",
				"message_count":       int64(1),
				"source_payload_json": `{"actions":[{"type":"text","content":"hello"}]}`,
				"queued_at":           "2026-06-29T02:00:00Z",
			}},
		}},
		Pagination: ProjectionRow{"page": 1, "page_size": 30, "total": 1, "total_pages": 1},
	}}
	projection := &fakeProjectionStore{rows: []ProjectionRow{{
		"conversation_id":   "ww:wu-1:external-1",
		"sender_remark":     "客户备注",
		"sender_name":       "客户昵称",
		"conversation_name": "客户会话",
		"external_userid":   "external-1",
		"sender_id":         "external-1",
	}}}
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-1",
		AccountName:  "企微张三",
		DeviceID:     "device-1",
		WeWorkUserID: "wu-1",
	}}}
	service := Service{
		SOPAnalyticsStore: store,
		Projection:        projection,
		Accounts:          accounts,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 3, 0, 0, 0, time.UTC)
		},
	}

	request, err := NewSOPDispatchTasksRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewSOPDispatchTasksRequest returned error: %v", err)
	}
	payload, err := service.SOPDispatchTasks(context.Background(), request)
	if err != nil {
		t.Fatalf("SOPDispatchTasks returned error: %v", err)
	}
	if len(projection.listQueries) != 1 || len(projection.listQueries[0].ConversationIDs) != 1 || projection.listQueries[0].ConversationIDs[0] != "ww:wu-1:external-1" {
		t.Fatalf("projection query = %+v", projection.listQueries)
	}
	batch := payload["batches"].([]ProjectionRow)[0]
	if batch["sender_name"] != "客户备注" || batch["receiver_display_name"] != "客户备注" || batch["receiver_name"] != "客户昵称" || batch["receiver_remark"] != "客户备注" {
		t.Fatalf("receiver display = %+v", batch)
	}
	if batch["account_id"] != "acc-1" || batch["device_id"] != "device-1" || batch["account_name"] != "企微张三" || batch["account_display_name"] != "企微张三-wu-1" {
		t.Fatalf("account display = %+v", batch)
	}
	detail := payload["tasks"].([]ProjectionRow)[0]
	if detail["sender_name"] != "客户备注" || detail["account_display_name"] != "企微张三-wu-1" {
		t.Fatalf("detail display = %+v", detail)
	}
}

// TestServiceSOPDispatchTasksDisablesManualResendWhenAutoResendPending mirrors Python deferred retry guard.
func TestServiceSOPDispatchTasksDisablesManualResendWhenAutoResendPending(t *testing.T) {
	store := &fakeSOPAnalyticsStore{taskBatchesPage: SOPTaskBatchesPage{
		Items: []SOPTaskBatchGroup{{
			BatchKey: "task-1",
			Rows: []ProjectionRow{{
				"fact_id":             "fact-1",
				"task_id":             "task-1",
				"conversation_id":     "ww:wu-1:external-1",
				"delivery_status":     "failed",
				"message_count":       int64(1),
				"source_payload_json": `{"actions":[{"type":"text","content":"hello"}]}`,
			}},
		}},
		Pagination: ProjectionRow{"page": 1, "page_size": 30, "total": 1, "total_pages": 1},
	}}
	pending := &fakeSOPAutoResendPendingStore{pending: map[string]bool{"task-1": true}}
	service := Service{SOPAnalyticsStore: store, SOPAutoResendPendingStore: pending}
	request, err := NewSOPDispatchTasksRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewSOPDispatchTasksRequest returned error: %v", err)
	}

	payload, err := service.SOPDispatchTasks(context.Background(), request)
	if err != nil {
		t.Fatalf("SOPDispatchTasks returned error: %v", err)
	}
	if len(pending.checked) != 1 || pending.checked[0] != "task-1" {
		t.Fatalf("checked = %#v", pending.checked)
	}
	batch := payload["batches"].([]ProjectionRow)[0]
	if batch["can_resend"] != false || batch["resend_block_reason"] != "自动补发排队中，请等待自动补发结果" {
		t.Fatalf("batch resend state = %+v", batch)
	}
	detail := payload["tasks"].([]ProjectionRow)[0]
	if detail["can_resend"] != false || detail["resend_block_reason"] != "自动补发排队中，请等待自动补发结果" {
		t.Fatalf("detail resend state = %+v", detail)
	}
}

// TestNewSOPFactsRequestValidatesPagination preserves FastAPI bounds.
func TestNewSOPFactsRequestValidatesPagination(t *testing.T) {
	if _, err := NewSOPFactsRequest(url.Values{"page": {"0"}}, auth.Session{}); !errors.Is(err, ErrInvalidSOPAnalyticsPage) {
		t.Fatalf("page error = %v", err)
	}
	if _, err := NewSOPFactsRequest(url.Values{"page_size": {"101"}}, auth.Session{}); !errors.Is(err, ErrInvalidSOPAnalyticsPageSize) {
		t.Fatalf("page_size error = %v", err)
	}
}

// TestServiceSOPAnalyticsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceSOPAnalyticsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).SOPAnalyticsStageStats(context.Background(), SOPStageStatsRequest{})
	if !errors.Is(err, ErrSOPAnalyticsStoreUnavailable) {
		t.Fatalf("stage stats error = %v", err)
	}
	_, err = (Service{}).SOPAnalyticsFacts(context.Background(), SOPFactsRequest{})
	if !errors.Is(err, ErrSOPAnalyticsStoreUnavailable) {
		t.Fatalf("facts error = %v", err)
	}
	_, err = (Service{}).SOPDispatchTasks(context.Background(), SOPDispatchTasksRequest{})
	if !errors.Is(err, ErrSOPAnalyticsStoreUnavailable) {
		t.Fatalf("dispatch tasks error = %v", err)
	}
}

type fakeSOPAnalyticsStore struct {
	stageItems       []ProjectionRow
	factsPage        SOPFactsPage
	taskBatchesPage  SOPTaskBatchesPage
	stageQuery       SOPStageStatsQuery
	factsQuery       SOPFactsQuery
	taskBatchesQuery SOPDispatchTasksQuery
	err              error
}

type fakeSOPAutoResendPendingStore struct {
	pending map[string]bool
	err     error
	checked []string
}

func (store *fakeSOPAutoResendPendingStore) IsSOPAutoResendPending(ctx context.Context, originalTaskID string) (bool, error) {
	store.checked = append(store.checked, originalTaskID)
	if store.err != nil {
		return false, store.err
	}
	return store.pending[originalTaskID], nil
}

// SummarizeSOPStageDaily captures stage query input for assertions.
func (store *fakeSOPAnalyticsStore) SummarizeSOPStageDaily(ctx context.Context, query SOPStageStatsQuery) ([]ProjectionRow, error) {
	store.stageQuery = query
	if store.err != nil {
		return nil, store.err
	}
	return store.stageItems, nil
}

// ListSOPFacts captures facts query input for assertions.
func (store *fakeSOPAnalyticsStore) ListSOPFacts(ctx context.Context, query SOPFactsQuery) (SOPFactsPage, error) {
	store.factsQuery = query
	if store.err != nil {
		return SOPFactsPage{}, store.err
	}
	return store.factsPage, nil
}

// ListSOPTaskBatches captures dispatch task query input for assertions.
func (store *fakeSOPAnalyticsStore) ListSOPTaskBatches(ctx context.Context, query SOPDispatchTasksQuery) (SOPTaskBatchesPage, error) {
	store.taskBatchesQuery = query
	if store.err != nil {
		return SOPTaskBatchesPage{}, store.err
	}
	return store.taskBatchesPage, nil
}
