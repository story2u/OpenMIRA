package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticForkedConversationsHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticForkedConversationsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticForkedConversationsService{payload: workbench.Payload{
		"total": 1,
		"items": []workbench.Payload{{"wework_user_id": "ww-a", "external_userid": "ext-a"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-forked-conversations",
	})

	response := performDiagnosticForkedConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/forked-conversations")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"wework_user_id":"ww-a"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticForkedConversationsHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticForkedConversationsHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticForkedConversationsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-forked-conversations",
	})

	response := performDiagnosticForkedConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/forked-conversations")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticForkedConversationsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticForkedConversationsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-forked-conversations",
	})

	response := performDiagnosticForkedConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/forked-conversations")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic forked conversations service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticForkedConversationsService struct {
	payload workbench.Payload
	request workbench.DiagnosticForkedConversationsRequest
	err     error
}

func (service *fakeDiagnosticForkedConversationsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticForkedConversationsService) DiagnosticForkedConversations(ctx context.Context, request workbench.DiagnosticForkedConversationsRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticForkedConversations(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticForkedConversationsHandler(response, request)
	return response
}
