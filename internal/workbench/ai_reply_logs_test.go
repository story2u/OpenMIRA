package workbench

import (
	"context"
	"net/url"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestNewAIReplyLogsRequestValidatesFilters(t *testing.T) {
	request, err := NewAIReplyLogsRequest(url.Values{
		"keyword":   []string{" 客户 "},
		"status":    []string{""},
		"date":      []string{"2026-06-29"},
		"page":      []string{"2"},
		"page_size": []string{"100"},
	}, auth.Session{Role: "supervisor"})
	if err != nil {
		t.Fatalf("NewAIReplyLogsRequest returned error: %v", err)
	}
	if request.Scope != "local" || request.Query.Keyword != "客户" || request.Query.Status != "all" || request.Query.Page != 2 || request.Query.PageSize != 100 {
		t.Fatalf("request = %+v", request)
	}
	if request.Query.Start == nil || request.Query.Start.Format(time.RFC3339) != "2026-06-29T00:00:00+08:00" {
		t.Fatalf("start = %v", request.Query.Start)
	}
}

func TestNewAIReplyLogsRequestRejectsInvalidBounds(t *testing.T) {
	if _, err := NewAIReplyLogsRequest(url.Values{"date": []string{"2026-13-01"}}, auth.Session{}); err != ErrInvalidAIReplyLogDate {
		t.Fatalf("date error = %v", err)
	}
	if _, err := NewAIReplyLogsRequest(url.Values{"page": []string{"0"}}, auth.Session{}); err != ErrInvalidAIReplyLogPage {
		t.Fatalf("page error = %v", err)
	}
	if _, err := NewAIReplyLogsRequest(url.Values{"page_size": []string{"101"}}, auth.Session{}); err != ErrInvalidAIReplyLogPageSize {
		t.Fatalf("page_size error = %v", err)
	}
}

func TestServiceAIReplyLogsBuildsLocalPayload(t *testing.T) {
	store := &fakeAIReplyLogStore{page: AIReplyLogPage{
		Logs: []ProjectionRow{{
			"attempt_id":                 "attempt-1",
			"account_id":                 "acc-1",
			"wework_user_id":             "wx-1",
			"conversation_id":            "conv-1",
			"incoming_trace_id":          "incoming-1",
			"outgoing_trace_id":          "outgoing-1",
			"task_id":                    "task-1",
			"workflow_id":                "",
			"model":                      "deepseek-chat",
			"trigger_event":              "message.created",
			"status":                     "failed",
			"failure_type":               "llm_timeout",
			"provider_error":             "timeout",
			"user_facing_error":          "稍后再试",
			"started_at":                 "2026-06-29 09:00:00",
			"finished_at":                "2026-06-29 09:00:03",
			"updated_at":                 "2026-06-29 09:00:04",
			"message_trace_id":           "message-1",
			"message_timestamp":          time.Date(2026, 6, 29, 1, 30, 0, 0, time.UTC),
			"message_content":            "AI 回复",
			"customer_message_trace_id":  "",
			"customer_message_content":   "客户问题",
			"conversation_sender_remark": "客户备注",
			"message_sender_name":        "消息发送人",
			"account_name":               "",
			"assignee_id":                "cs-1",
			"assignee_name":              "客服一",
		}},
		Pagination: ProjectionRow{"page": 1, "page_size": 50, "total": 1, "total_pages": 1},
	}}
	service := Service{AIReplyLogStore: store}

	payload, err := service.AIReplyLogs(context.Background(), AIReplyLogsRequest{Scope: "local", Query: AIReplyLogQuery{Status: "failed", Page: 1, PageSize: 50}})
	if err != nil {
		t.Fatalf("AIReplyLogs returned error: %v", err)
	}
	logs := payload["logs"].([]ProjectionRow)
	first := logs[0]
	if !store.query.LocalOnly || first["reply_time"] != "2026-06-29T09:30:00+08:00" || first["receiver_name"] != "客户备注" {
		t.Fatalf("query/log = %+v %+v", store.query, first)
	}
	if first["trace_id"] != "message-1" || first["account_name"] != "wx-1" || first["customer_message_missing"] != true || first["message_missing"] != false {
		t.Fatalf("serialized log = %+v", first)
	}
}

func TestServiceAIReplyLogsResolvesExternalProfile(t *testing.T) {
	store := &fakeAIReplyLogStore{page: AIReplyLogPage{Logs: []ProjectionRow{}, Pagination: ProjectionRow{"page": 1, "page_size": 50, "total": 0, "total_pages": 1}}}
	config := fakeAIConfigStore{values: map[string]string{
		"ai.coze_profiles":          `[{"profile_id":"coze-main","workflow_id":"wf-1","enabled":true}]`,
		"ai.coze_v2_profile_seeded": "true",
	}}
	service := Service{AIReplyLogStore: store, AIConfigStore: config}

	_, err := service.AIReplyLogs(context.Background(), AIReplyLogsRequest{Scope: "coze-main", Query: AIReplyLogQuery{Status: "all", Page: 1, PageSize: 50}})
	if err != nil {
		t.Fatalf("AIReplyLogs returned error: %v", err)
	}
	if store.query.LocalOnly || store.query.WorkflowID != "wf-1" {
		t.Fatalf("external query = %+v", store.query)
	}
}

func TestServiceAIReplyLogsRejectsUnknownScopeAndMissingStore(t *testing.T) {
	if _, err := (Service{}).AIReplyLogs(context.Background(), AIReplyLogsRequest{}); err != ErrAIReplyLogStoreUnavailable {
		t.Fatalf("missing store error = %v", err)
	}
	service := Service{AIReplyLogStore: &fakeAIReplyLogStore{}, AIConfigStore: fakeAIConfigStore{}}
	_, err := service.AIReplyLogs(context.Background(), AIReplyLogsRequest{Scope: "missing", Query: AIReplyLogQuery{Page: 1, PageSize: 50}})
	if err != ErrUnknownAIConfigScope {
		t.Fatalf("unknown scope error = %v", err)
	}
}

type fakeAIReplyLogStore struct {
	page  AIReplyLogPage
	query AIReplyLogQuery
}

func (store *fakeAIReplyLogStore) ListAIReplyLogs(ctx context.Context, query AIReplyLogQuery) (AIReplyLogPage, error) {
	store.query = query
	return store.page, nil
}
