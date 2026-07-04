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

func TestConversationAIHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeConversationAIService{payload: workbench.Payload{"success": true, "ai_auto_reply": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-conversation-ai",
	})

	response := performConversationAI(handler, "Bearer "+token, "/api/v1/conversations/conv-1/ai-auto-reply", "conv-1", `{"enabled":true}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ai_auto_reply":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.Enabled == nil || *service.request.Enabled != true || service.request.Session.Role != "cs" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestConversationAIHandlerMapsServiceErrors(t *testing.T) {
	service := &fakeConversationAIService{err: workbench.ErrConversationAIEnabledRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-conversation-ai-errors",
	})

	response := performConversationAI(handler, "Bearer "+token, "/api/v1/conversations/conv-1/ai-auto-reply", "conv-1", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "enabled is required") {
		t.Fatalf("required response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrConversationNotFound
	response = performConversationAI(handler, "Bearer "+token, "/api/v1/conversations/missing/ai-auto-reply", "missing", `{"enabled":true}`)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "conversation not found") {
		t.Fatalf("not found response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrConversationAIProfileRequired
	response = performConversationAI(handler, "Bearer "+token, "/api/v1/conversations/conv-1/ai-auto-reply", "conv-1", `{"enabled":true}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "请先在 AI 设置中") {
		t.Fatalf("profile response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationAIBulkHandlerMapsPermission(t *testing.T) {
	service := &fakeConversationAIService{err: auth.ErrPermissionDenied}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-conversation-ai-bulk",
	})

	response := performConversationAIBulk(handler, "Bearer "+token, `{"enabled":true,"assignee_id":"cs-002"}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.bulkRequest.AssigneeID != "cs-002" || service.bulkRequest.Enabled == nil || *service.bulkRequest.Enabled != true {
		t.Fatalf("bulk request = %+v", service.bulkRequest)
	}
}

type fakeConversationAIService struct {
	payload     workbench.Payload
	err         error
	request     workbench.ConversationAIRequest
	bulkRequest workbench.ConversationAIBulkRequest
}

func (service *fakeConversationAIService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeConversationAIService) ToggleConversationAI(ctx context.Context, request workbench.ConversationAIRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeConversationAIService) ToggleConversationAIBulk(ctx context.Context, request workbench.ConversationAIBulkRequest) (workbench.Payload, error) {
	service.bulkRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performConversationAI(handler Handler, authorization string, target string, conversationID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationAIHandler(response, request)
	return response
}

func performConversationAIBulk(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/ai-auto-reply/bulk", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationAIBulkHandler(response, request)
	return response
}
