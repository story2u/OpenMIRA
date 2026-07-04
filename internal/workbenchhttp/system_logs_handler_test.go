// System log handler tests keep the file-backed admin route local to its
// candidate slice and verify auth, validation, and missing service behavior.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestSystemLogsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSystemLogsService{payload: workbench.Payload{
		"items": []any{map[string]any{"level": "ERROR", "module": "api"}},
		"total": 1,
		"date":  "2026-06-29",
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-system-logs",
	})

	response := performSystemLogs(handler, "Bearer "+token, "/api/v1/admin/system-logs?date=2026-06-29&level=warn&limit=20")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"total":1`) || service.request.Query.Level != "warn" || service.request.Query.Limit != 20 {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.request)
	}
}

func TestSystemLogsHandlerRejectsInvalidQuery(t *testing.T) {
	handler := New(testGuard(t), &fakeSystemLogsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-system-logs",
	})

	response := performSystemLogs(handler, "Bearer "+token, "/api/v1/admin/system-logs?date=2026-13-01")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid date: 2026-13-01") {
		t.Fatalf("invalid query response = %d %s", response.Code, response.Body.String())
	}
}

func TestSystemLogsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-system-logs",
	})

	response := performSystemLogs(handler, "Bearer "+token, "/api/v1/admin/system-logs")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench system logs service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeSystemLogsService struct {
	payload workbench.Payload
	request workbench.SystemLogsRequest
}

func (service *fakeSystemLogsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeSystemLogsService) SystemLogs(ctx context.Context, request workbench.SystemLogsRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

func performSystemLogs(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SystemLogsHandler(response, request)
	return response
}
