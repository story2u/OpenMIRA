package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestCSUserUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeCSUserWriteService{upsertPayload: workbench.Payload{
		"success": true,
		"user":    map[string]any{"assignee_id": "cs-001"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-user-upsert",
	})

	response := performCSUserUpsert(handler, "Bearer "+token, "/api/v1/cs-users", `{"assignee_id":" cs-001 ","assignee_name":" 客服A ","role":"","enabled":false,"ai_enabled":true,"max_sessions":3,"password":" secret1 "}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignee_id":"cs-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	command := service.upsertRequest.Command
	if command.AssigneeID != "cs-001" || command.AssigneeName != "客服A" || command.Role != "cs" || command.Enabled || !command.AIEnabled || command.MaxSessions != 3 || command.Password != "secret1" {
		t.Fatalf("unexpected upsert request: %+v", service.upsertRequest)
	}
}

func TestCSUserUpsertHandlerMapsValidationAndConflict(t *testing.T) {
	service := &fakeCSUserWriteService{err: workbench.ErrCSUserPasswordTooShort}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-user-validation",
	})

	response := performCSUserUpsert(handler, "Bearer "+token, "/api/v1/cs-users", `{"assignee_id":"cs-001","assignee_name":"客服A","password":"12345"}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "密码长度不得少于6位") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.CSUserConflictError{Detail: "客服ID已存在：cs-001"}
	response = performCSUserUpsert(handler, "Bearer "+token, "/api/v1/cs-users", `{"assignee_id":"cs-001","assignee_name":"客服A","create_only":true}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "客服ID已存在") {
		t.Fatalf("conflict response = %d %s", response.Code, response.Body.String())
	}
}

func TestCSUserDeleteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeCSUserWriteService{deletePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-user-delete",
	})

	response := performCSUserDelete(handler, "Bearer "+token, "/api/v1/cs-users/cs-001", "cs-001")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteRequest.AssigneeID != "cs-001" {
		t.Fatalf("delete request = %+v", service.deleteRequest)
	}
}

func TestCSUserWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-user-write-missing",
	})

	response := performCSUserUpsert(handler, "Bearer "+token, "/api/v1/cs-users", `{"assignee_id":"cs-001","assignee_name":"客服A"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench cs users write service is not configured") {
		t.Fatalf("write service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeCSUserWriteService struct {
	upsertPayload workbench.Payload
	deletePayload workbench.Payload
	upsertRequest workbench.CSUserUpsertRequest
	deleteRequest workbench.CSUserDeleteRequest
	err           error
}

func (service *fakeCSUserWriteService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeCSUserWriteService) UpsertCSUser(ctx context.Context, request workbench.CSUserUpsertRequest) (workbench.Payload, error) {
	service.upsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.upsertPayload, nil
}

func (service *fakeCSUserWriteService) DeleteCSUser(ctx context.Context, request workbench.CSUserDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.deletePayload, nil
}

func performCSUserUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.CSUserUpsertHandler(response, request)
	return response
}

func performCSUserDelete(handler Handler, authorization string, target string, assigneeID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("assignee_id", assigneeID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.CSUserDeleteHandler(response, request)
	return response
}
