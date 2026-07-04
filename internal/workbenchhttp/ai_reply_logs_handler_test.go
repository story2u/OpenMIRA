// AI reply log handler tests keep the admin candidate isolated from the large
// workbench handler test file while covering auth, validation, and service
// wiring behavior.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestAIReplyLogsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAIReplyLogsService{payload: workbench.Payload{
		"logs":       []any{map[string]any{"attempt_id": "attempt-1"}},
		"pagination": map[string]any{"total": 1},
		"scope":      "local",
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-reply-logs",
	})

	response := performAIReplyLogs(handler, "Bearer "+token, "/api/v1/admin/ai-config/reply-logs?scope=local&status=failed&page=2&page_size=20")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"attempt_id":"attempt-1"`) || service.request.Query.Status != "failed" || service.request.Query.Page != 2 {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.request)
	}
}

func TestAIReplyLogsHandlerRejectsInvalidQuery(t *testing.T) {
	handler := New(testGuard(t), &fakeAIReplyLogsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-reply-logs",
	})

	response := performAIReplyLogs(handler, "Bearer "+token, "/api/v1/admin/ai-config/reply-logs?date=2026-13-01")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "date must be YYYY-MM-DD") {
		t.Fatalf("invalid query response = %d %s", response.Code, response.Body.String())
	}
}

func TestAIReplyLogsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-reply-logs",
	})

	response := performAIReplyLogs(handler, "Bearer "+token, "/api/v1/admin/ai-config/reply-logs")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench ai reply logs service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAIReplyLogsService struct {
	payload workbench.Payload
	request workbench.AIReplyLogsRequest
}

func (service *fakeAIReplyLogsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeAIReplyLogsService) AIReplyLogs(ctx context.Context, request workbench.AIReplyLogsRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

func performAIReplyLogs(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AIReplyLogsHandler(response, request)
	return response
}
