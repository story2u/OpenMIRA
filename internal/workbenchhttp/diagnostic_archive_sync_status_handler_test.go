package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticArchiveSyncStatusHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticArchiveSyncStatusHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticArchiveSyncStatusService{payload: workbench.Payload{
		"total": 1,
		"items": []workbench.Payload{{"enterprise_id": "ent-a", "cursor": "42"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-sync-status",
	})

	response := performDiagnosticArchiveSyncStatus(handler, "Bearer "+token, "/api/v1/admin/diagnostic/archive-sync-status")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"enterprise_id":"ent-a"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticArchiveSyncStatusHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticArchiveSyncStatusHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticArchiveSyncStatusService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-sync-status",
	})

	response := performDiagnosticArchiveSyncStatus(handler, "Bearer "+token, "/api/v1/admin/diagnostic/archive-sync-status")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticArchiveSyncStatusHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticArchiveSyncStatusHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-sync-status",
	})

	response := performDiagnosticArchiveSyncStatus(handler, "Bearer "+token, "/api/v1/admin/diagnostic/archive-sync-status")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic archive sync status service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticArchiveSyncStatusService struct {
	payload workbench.Payload
	request workbench.DiagnosticArchiveSyncStatusRequest
	err     error
}

func (service *fakeDiagnosticArchiveSyncStatusService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticArchiveSyncStatusService) DiagnosticArchiveSyncStatus(ctx context.Context, request workbench.DiagnosticArchiveSyncStatusRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticArchiveSyncStatus(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticArchiveSyncStatusHandler(response, request)
	return response
}
