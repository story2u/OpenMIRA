package workbench

import (
	"context"
	"net/url"
	"testing"

	"wework-go/internal/auth"
)

func TestNewAuditLogsRequestNormalizesFilters(t *testing.T) {
	values := url.Values{}
	values.Set("operator", " admin ")
	values.Set("action_type", " config ")
	values.Set("date", "2026-06-29")
	values.Set("page", "0")
	values.Set("page_size", "200")

	request := NewAuditLogsRequest(values, auth.Session{Role: "supervisor"})

	if request.Query.Operator != "admin" || request.Query.ActionType != "config" || request.Query.Date != "2026-06-29" {
		t.Fatalf("unexpected filters: %+v", request.Query)
	}
	if request.Query.Page != 1 || request.Query.PageSize != 100 || request.Session.Role != "supervisor" {
		t.Fatalf("unexpected pagination/session: %+v", request)
	}
}

func TestServiceAuditLogsBuildsPaginationPayload(t *testing.T) {
	store := fakeAuditLogStore{page: AuditLogPage{
		Total: 41,
		Logs: []AuditLogRecord{
			{LogID: "log-1", Operator: "admin", ActionType: "config", Detail: "更新配置", IP: "127.0.0.1", CreatedAt: "2026-06-29T10:00:00Z"},
		},
	}}
	service := Service{AuditLogStore: store}

	payload, err := service.AuditLogs(context.Background(), AuditLogsRequest{Query: AuditLogQuery{Page: 2, PageSize: 20}})
	if err != nil {
		t.Fatalf("AuditLogs returned error: %v", err)
	}
	logs := payload["logs"].([]ProjectionRow)
	pagination := payload["pagination"].(ProjectionRow)
	if len(logs) != 1 || rowText(logs[0], "log_id") != "log-1" || rowText(logs[0], "detail") != "更新配置" {
		t.Fatalf("logs = %+v", logs)
	}
	if pagination["page"] != 2 || pagination["page_size"] != 20 || pagination["total"] != 41 || pagination["total_pages"] != 3 {
		t.Fatalf("pagination = %+v", pagination)
	}
}

func TestServiceAuditLogsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).AuditLogs(context.Background(), AuditLogsRequest{})
	if err != ErrAuditLogStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrAuditLogStoreUnavailable)
	}
}

type fakeAuditLogStore struct {
	page AuditLogPage
}

func (store fakeAuditLogStore) ListAuditLogs(ctx context.Context, query AuditLogQuery) (AuditLogPage, error) {
	return store.page, nil
}
