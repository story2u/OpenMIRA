package workbench

import (
	"context"
	"net/url"
	"testing"

	"wework-go/internal/auth"
)

func TestNewSystemLogsRequestValidatesFilters(t *testing.T) {
	values := url.Values{}
	values.Set("date", "2026-06-29")
	values.Set("level", "warn,error")
	values.Set("module", "api")
	values.Set("keyword", "timeout")
	values.Set("limit", "500")
	values.Set("offset", "3")

	request, err := NewSystemLogsRequest(values, auth.Session{Role: "supervisor"})
	if err != nil {
		t.Fatalf("NewSystemLogsRequest returned error: %v", err)
	}
	if request.Query.Date != "2026-06-29" || request.Query.Level != "warn,error" || request.Query.Module != "api" || request.Query.Keyword != "timeout" {
		t.Fatalf("unexpected filters: %+v", request.Query)
	}
	if request.Query.Limit != 500 || request.Query.Offset != 3 || request.Session.Role != "supervisor" {
		t.Fatalf("unexpected pagination/session: %+v", request)
	}
}

func TestNewSystemLogsRequestRejectsInvalidBounds(t *testing.T) {
	if _, err := NewSystemLogsRequest(url.Values{"date": []string{"2026-13-01"}}, auth.Session{}); err == nil {
		t.Fatal("invalid date error = nil")
	}
	if _, err := NewSystemLogsRequest(url.Values{"limit": []string{"501"}}, auth.Session{}); err != ErrInvalidSystemLogLimit {
		t.Fatalf("limit error = %v, want %v", err, ErrInvalidSystemLogLimit)
	}
	if _, err := NewSystemLogsRequest(url.Values{"offset": []string{"-1"}}, auth.Session{}); err != ErrInvalidSystemLogOffset {
		t.Fatalf("offset error = %v, want %v", err, ErrInvalidSystemLogOffset)
	}
}

func TestServiceSystemLogsBuildsPayload(t *testing.T) {
	store := fakeSystemLogStore{page: SystemLogPage{
		Date:  "2026-06-29",
		Total: 2,
		Items: []ProjectionRow{{"level": "ERROR", "module": "api", "detail": "失败"}},
	}}
	service := Service{SystemLogStore: store}

	payload, err := service.SystemLogs(context.Background(), SystemLogsRequest{Query: SystemLogQuery{Limit: 1}})
	if err != nil {
		t.Fatalf("SystemLogs returned error: %v", err)
	}
	items := payload["items"].([]ProjectionRow)
	if len(items) != 1 || rowText(items[0], "detail") != "失败" || payload["total"] != 2 || payload["date"] != "2026-06-29" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestServiceSystemLogsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).SystemLogs(context.Background(), SystemLogsRequest{})
	if err != ErrSystemLogStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrSystemLogStoreUnavailable)
	}
}

type fakeSystemLogStore struct {
	page SystemLogPage
}

func (store fakeSystemLogStore) ListSystemLogs(ctx context.Context, query SystemLogQuery) (SystemLogPage, error) {
	return store.page, nil
}
