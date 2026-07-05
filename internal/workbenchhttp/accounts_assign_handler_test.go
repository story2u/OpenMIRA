package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"im-go/internal/workbench"
)

func TestAccountAssignHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountAssignService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "im-cloud",
		"sub":         "admin-001",
		"role":        "admin",
		"assignee_id": "admin-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-assign",
	})

	response := performAccountAssign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/assign", "acc-001", `{"assignee_id":"cs-002","assignee_name":"消息端二"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.assignRequest.AccountID != "acc-001" || service.assignRequest.AssigneeID != "cs-002" || service.assignRequest.AssigneeName != "消息端二" || service.assignRequest.Session.Role != "admin" {
		t.Fatalf("request = %+v", service.assignRequest)
	}
}

func TestAccountUnassignHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountAssignService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "im-cloud",
		"sub":         "sup-001",
		"role":        "supervisor",
		"assignee_id": "sup-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-unassign",
	})

	response := performAccountUnassign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/unassign", "acc-001")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.unassignRequest.AccountID != "acc-001" || service.unassignRequest.Session.Role != "supervisor" {
		t.Fatalf("request = %+v", service.unassignRequest)
	}
}

func TestAccountAssignHandlersRejectCSRole(t *testing.T) {
	service := &fakeAccountAssignService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "im-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-assign-cs",
	})

	response := performAccountAssign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/assign", "acc-001", `{"assignee_id":"cs-002"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("assign response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountUnassign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/unassign", "acc-001")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("unassign response = %d %s", response.Code, response.Body.String())
	}
	if service.assignRequest.AccountID != "" || service.unassignRequest.AccountID != "" {
		t.Fatalf("service should not be called: %+v", service)
	}
}

func TestAccountAssignHandlerMapsServiceErrors(t *testing.T) {
	service := &fakeAccountAssignService{err: workbench.ErrAccountAssigneeRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-assign-errors",
	})

	response := performAccountAssign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/assign", "acc-001", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "assignee_id is required") {
		t.Fatalf("required response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrAccountNotFound
	response = performAccountAssign(handler, "Bearer "+token, "/api/v1/accounts/missing/assign", "missing", `{"assignee_id":"cs-002"}`)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "account not found") {
		t.Fatalf("not found response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountUnassign(handler, "Bearer "+token, "/api/v1/accounts/missing/unassign", "missing")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "account not found") {
		t.Fatalf("unassign not found response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountAssignHandlersRequireConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-assign-missing",
	})

	response := performAccountAssign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/assign", "acc-001", `{"assignee_id":"cs-002"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account assign write service is not configured") {
		t.Fatalf("assign response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountUnassign(handler, "Bearer "+token, "/api/v1/accounts/acc-001/unassign", "acc-001")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account assign write service is not configured") {
		t.Fatalf("unassign response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAccountAssignService struct {
	payload         workbench.Payload
	err             error
	assignRequest   workbench.AccountAssignRequest
	unassignRequest workbench.AccountUnassignRequest
}

func (service *fakeAccountAssignService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeAccountAssignService) AssignAccount(ctx context.Context, request workbench.AccountAssignRequest) (workbench.Payload, error) {
	service.assignRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeAccountAssignService) UnassignAccount(ctx context.Context, request workbench.AccountUnassignRequest) (workbench.Payload, error) {
	service.unassignRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performAccountAssign(handler Handler, authorization string, target string, accountID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.SetPathValue("account_id", accountID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountAssignHandler(response, request)
	return response
}

func performAccountUnassign(handler Handler, authorization string, target string, accountID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("account_id", accountID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountUnassignHandler(response, request)
	return response
}
