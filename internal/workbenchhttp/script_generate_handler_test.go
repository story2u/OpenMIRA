package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestScriptGenerateHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeScriptGenerateService{payload: workbench.Payload{"success": true, "content": "生成话术"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-script-generate",
	})

	response := performScriptGenerate(handler, "Bearer "+token, "/api/v1/scripts/generate", `{"prompt":" 预约流程 ","style":" 简洁 ","system_prompt":" 系统 "}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"content":"生成话术"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "cs" || service.request.Body.Prompt != "预约流程" || service.request.Body.Style != "简洁" {
		t.Fatalf("unexpected script generate request: %+v", service.request)
	}
}

func TestScriptGenerateHandlerMapsValidationAndGenerationErrors(t *testing.T) {
	service := &fakeScriptGenerateService{err: workbench.ErrScriptPromptRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-script-generate",
	})

	response := performScriptGenerate(handler, "Bearer "+token, "/api/v1/scripts/generate", `{"prompt":" "}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "prompt is required") {
		t.Fatalf("blank prompt response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrScriptAIAPIKeyMissing
	response = performScriptGenerate(handler, "Bearer "+token, "/api/v1/scripts/generate", `{"prompt":"hello"}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "AI_API_KEY") {
		t.Fatalf("api key response = %d %s", response.Code, response.Body.String())
	}
}

func TestScriptGenerateHandlerRejectsUnknownRoleAndMissingService(t *testing.T) {
	handler := New(testGuard(t), &fakeScriptGenerateService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "guest-001",
		"role": "guest",
		"exp":  int64(2000),
		"jti":  "jwt-script-generate",
	})

	response := performScriptGenerate(handler, "Bearer "+token, "/api/v1/scripts/generate", `{"prompt":"hello"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}

	handler = Handler{Guard: testGuard(t)}
	token = signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-script-generate",
	})
	response = performScriptGenerate(handler, "Bearer "+token, "/api/v1/scripts/generate", `{"prompt":"hello"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "script generation service") {
		t.Fatalf("missing service response = %d %s", response.Code, response.Body.String())
	}
}

type fakeScriptGenerateService struct {
	payload workbench.Payload
	err     error
	request workbench.ScriptGenerateRequest
}

func (service *fakeScriptGenerateService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeScriptGenerateService) GenerateScript(ctx context.Context, request workbench.ScriptGenerateRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, service.err
}

func performScriptGenerate(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ScriptGenerateHandler(response, request)
	return response
}
