package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/workbench"
)

func TestAccountAIEnabledHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountAIEnabledService{payload: workbench.Payload{
		"success":       true,
		"enabled":       true,
		"updated_count": 1,
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-ai-enabled",
	})

	response := performAccountAIEnabled(handler, "Bearer "+token, "/api/v1/accounts/acc-001/ai-enabled", "acc-001", `{"enabled":true}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"updated_count":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.AccountID != "acc-001" || service.request.Enabled == nil || *service.request.Enabled != true || service.request.Session.Role != "cs" || service.request.Session.AssigneeID != "cs-001" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestAccountAIEnabledHandlerMapsServiceErrors(t *testing.T) {
	service := &fakeAccountAIEnabledService{err: workbench.ErrAccountAIEnabledRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-ai-errors",
	})

	response := performAccountAIEnabled(handler, "Bearer "+token, "/api/v1/accounts/acc-001/ai-enabled", "acc-001", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "enabled is required") {
		t.Fatalf("required response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrAccountNotFound
	response = performAccountAIEnabled(handler, "Bearer "+token, "/api/v1/accounts/missing/ai-enabled", "missing", `{"enabled":true}`)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "account not found") {
		t.Fatalf("not found response = %d %s", response.Code, response.Body.String())
	}

	service.err = auth.ErrPermissionDenied
	response = performAccountAIEnabled(handler, "Bearer "+token, "/api/v1/accounts/acc-001/ai-enabled", "acc-001", `{"enabled":true}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("permission response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountAIEnabledHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-ai-missing",
	})

	response := performAccountAIEnabled(handler, "Bearer "+token, "/api/v1/accounts/acc-001/ai-enabled", "acc-001", `{"enabled":true}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account ai write service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAccountAIEnabledService struct {
	payload workbench.Payload
	err     error
	request workbench.AccountAIEnabledRequest
}

func (service *fakeAccountAIEnabledService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeAccountAIEnabledService) ToggleAccountAIEnabled(ctx context.Context, request workbench.AccountAIEnabledRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performAccountAIEnabled(handler Handler, authorization string, target string, accountID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.SetPathValue("account_id", accountID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountAIEnabledHandler(response, request)
	return response
}
