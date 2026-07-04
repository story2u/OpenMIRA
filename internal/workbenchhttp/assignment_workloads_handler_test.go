// Assignment workloads handler tests cover route auth and service wiring.
// The service layer owns CS self-scope and workload math; the handler only
// adapts legacy auth and response serialization.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestAssignmentWorkloadsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentWorkloadsService{payload: workbench.Payload{
		"workloads": []any{map[string]any{"assignee_id": "cs-001", "current_sessions": 2}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"assignee_id": "cs-001",
		"role":        "cs",
		"exp":         int64(2000),
		"jti":         "jwt-assignment-workloads",
	})

	response := performAssignmentWorkloads(handler, "Bearer "+token, "/api/v1/assignments/workloads")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignee_id":"cs-001"`) || service.request.Session.AssigneeID != "cs-001" {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.request)
	}
}

func TestAssignmentWorkloadsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-workloads",
	})

	response := performAssignmentWorkloads(handler, "Bearer "+token, "/api/v1/assignments/workloads")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment workloads service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAssignmentWorkloadsService struct {
	payload workbench.Payload
	request workbench.AssignmentWorkloadsRequest
}

func (service *fakeAssignmentWorkloadsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeAssignmentWorkloadsService) AssignmentWorkloads(ctx context.Context, request workbench.AssignmentWorkloadsRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

func performAssignmentWorkloads(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentWorkloadsHandler(response, request)
	return response
}
