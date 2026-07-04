package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestAIConfigWriteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAIConfigWriteService{payload: workbench.Payload{"success": true, "config": map[string]any{"model": "deepseek-chat"}}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-write",
	})

	response := performAIConfigWrite(handler, "Bearer "+token, "/api/v1/admin/ai-config", `{"enabled":true,"base_url":" https://ai.example ","model":" deepseek-chat ","timeout_sec":20,"temperature":0.7}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.Body.BaseURL == nil || *service.request.Body.BaseURL != " https://ai.example " || service.request.Body.Model == nil || service.request.Session.Role != "admin" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestAIConfigWriteHandlerMapsValidationErrors(t *testing.T) {
	service := &fakeAIConfigWriteService{err: workbench.ErrAIConfigBaseURLRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-write-error",
	})

	response := performAIConfigWrite(handler, "Bearer "+token, "/api/v1/admin/ai-config", `{"model":"deepseek-chat"}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "base_url is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.AIConfigValidationError{Detail: "一个客服只能分配一个 AI 自动回复逻辑"}
	response = performAIConfigWrite(handler, "Bearer "+token, "/api/v1/admin/ai-config", `{"base_url":"x","model":"m"}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "一个客服只能分配一个 AI 自动回复逻辑") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

func TestAIConfigWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-write-missing",
	})

	response := performAIConfigWrite(handler, "Bearer "+token, "/api/v1/admin/ai-config", `{"base_url":"x","model":"m"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench ai config write service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAIConfigWriteService struct {
	payload workbench.Payload
	err     error
	request workbench.AIConfigUpdateRequest
}

func (service *fakeAIConfigWriteService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeAIConfigWriteService) UpdateAIConfig(ctx context.Context, request workbench.AIConfigUpdateRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performAIConfigWrite(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AIConfigWriteHandler(response, request)
	return response
}
