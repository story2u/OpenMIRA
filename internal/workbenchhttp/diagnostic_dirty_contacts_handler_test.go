package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticDirtyContactsHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticDirtyContactsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticDirtyContactsService{payload: workbench.Payload{
		"total": 1,
		"items": []workbench.ProjectionRow{{"sender_id": "external-a", "identity_status": "missing"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-dirty-contacts",
	})

	response := performDiagnosticDirtyContacts(handler, "Bearer "+token, "/api/v1/admin/diagnostic/dirty-contacts?limit=20")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"sender_id":"external-a"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" || service.request.Limit != 20 {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticDirtyContactsHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticDirtyContactsHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticDirtyContactsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-dirty-contacts",
	})

	response := performDiagnosticDirtyContacts(handler, "Bearer "+token, "/api/v1/admin/diagnostic/dirty-contacts")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticDirtyContactsHandlerRejectsInvalidLimit keeps FastAPI Query bounds.
func TestDiagnosticDirtyContactsHandlerRejectsInvalidLimit(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticDirtyContactsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-dirty-contacts",
	})

	response := performDiagnosticDirtyContacts(handler, "Bearer "+token, "/api/v1/admin/diagnostic/dirty-contacts?limit=501")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("bad request response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticDirtyContactsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticDirtyContactsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-dirty-contacts",
	})

	response := performDiagnosticDirtyContacts(handler, "Bearer "+token, "/api/v1/admin/diagnostic/dirty-contacts")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic dirty contacts service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticDirtyContactsService struct {
	payload workbench.Payload
	request workbench.DiagnosticDirtyContactsRequest
	err     error
}

func (service *fakeDiagnosticDirtyContactsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticDirtyContactsService) DiagnosticDirtyContacts(ctx context.Context, request workbench.DiagnosticDirtyContactsRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticDirtyContacts(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticDirtyContactsHandler(response, request)
	return response
}
