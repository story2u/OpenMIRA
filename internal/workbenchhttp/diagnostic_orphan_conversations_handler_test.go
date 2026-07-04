package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticOrphanConversationsHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticOrphanConversationsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticOrphanConversationsService{payload: workbench.Payload{
		"total": 1,
		"items": []workbench.Payload{{"conversation_id": "conv-1", "resolved_account_id": "acc-a"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-orphan-conversations",
	})

	response := performDiagnosticOrphanConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/orphan-conversations")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"conv-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticOrphanConversationsHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticOrphanConversationsHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticOrphanConversationsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-orphan-conversations",
	})

	response := performDiagnosticOrphanConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/orphan-conversations")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticOrphanConversationsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticOrphanConversationsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-orphan-conversations",
	})

	response := performDiagnosticOrphanConversations(handler, "Bearer "+token, "/api/v1/admin/diagnostic/orphan-conversations")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic orphan conversations service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticOrphanConversationsService struct {
	payload workbench.Payload
	request workbench.DiagnosticOrphanConversationsRequest
	err     error
}

func (service *fakeDiagnosticOrphanConversationsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticOrphanConversationsService) DiagnosticOrphanConversations(ctx context.Context, request workbench.DiagnosticOrphanConversationsRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticOrphanConversations(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticOrphanConversationsHandler(response, request)
	return response
}
