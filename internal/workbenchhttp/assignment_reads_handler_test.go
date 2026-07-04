// Assignment read handler tests keep list/detail HTTP adaptation thin.
// The handler verifies legacy auth, query/path normalization, and service
// error mapping while assignment scope rules remain in workbench.Service.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestAssignmentsListHandlerSerializesServicePayload verifies query adaptation.
func TestAssignmentsListHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentReadsService{listPayload: workbench.Payload{
		"assignments": []any{map[string]any{"conversation_id": "conv-001", "assignee_id": "cs-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "admin-001",
		"assignee_id": "admin-001",
		"role":        "admin",
		"exp":         int64(2000),
		"jti":         "jwt-assignments-list",
	})

	response := performAssignmentsList(handler, "Bearer "+token, "/api/v1/assignments?assignee_id=cs-001&limit=25")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"conv-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.listRequest.AssigneeID != "cs-001" || service.listRequest.Limit != 25 || service.listRequest.Session.Role != "admin" {
		t.Fatalf("unexpected list request: %+v", service.listRequest)
	}
}

// TestAssignmentsListHandlerRejectsInvalidQuery preserves FastAPI 422 shape.
func TestAssignmentsListHandlerRejectsInvalidQuery(t *testing.T) {
	handler := New(testGuard(t), &fakeAssignmentReadsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignments-list-invalid",
	})

	response := performAssignmentsList(handler, "Bearer "+token, "/api/v1/assignments")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "assignee_id is required") {
		t.Fatalf("invalid query response = %d %s", response.Code, response.Body.String())
	}
}

// TestAssignmentDetailHandlerSerializesServicePayload verifies path adaptation.
func TestAssignmentDetailHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentReadsService{detailPayload: workbench.Payload{
		"assignment": map[string]any{"conversation_id": "conv-001", "assignee_id": "cs-001"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"assignee_id": "cs-001",
		"role":        "cs",
		"exp":         int64(2000),
		"jti":         "jwt-assignment-detail",
	})

	response := performAssignmentDetail(handler, "Bearer "+token, "/api/v1/assignments/conv-001", "conv-001")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignment"`) || service.detailRequest.ConversationID != "conv-001" {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.detailRequest)
	}
}

// TestAssignmentsListHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestAssignmentsListHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignments-list-missing",
	})

	response := performAssignmentsList(handler, "Bearer "+token, "/api/v1/assignments?assignee_id=cs-001")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment reads service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAssignmentReadsService struct {
	listPayload   workbench.Payload
	detailPayload workbench.Payload
	listRequest   workbench.AssignmentsListRequest
	detailRequest workbench.AssignmentDetailRequest
}

// Bootstrap keeps fakeAssignmentReadsService compatible with workbenchhttp.New.
func (service *fakeAssignmentReadsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// AssignmentsList captures list requests for handler assertions.
func (service *fakeAssignmentReadsService) AssignmentsList(ctx context.Context, request workbench.AssignmentsListRequest) (workbench.Payload, error) {
	service.listRequest = request
	return service.listPayload, nil
}

// AssignmentDetail captures detail requests for handler assertions.
func (service *fakeAssignmentReadsService) AssignmentDetail(ctx context.Context, request workbench.AssignmentDetailRequest) (workbench.Payload, error) {
	service.detailRequest = request
	return service.detailPayload, nil
}

// performAssignmentsList executes the list handler with optional authorization.
func performAssignmentsList(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentsListHandler(response, request)
	return response
}

// performAssignmentDetail executes the detail handler with a mux path value.
func performAssignmentDetail(handler Handler, authorization string, target string, conversationID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentDetailHandler(response, request)
	return response
}
