package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestConversationReadHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeConversationReadService{payload: workbench.Payload{"success": true, "already_read": false}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-conversation-read",
	})

	response := performConversationRead(handler, "Bearer "+token, "/api/v1/conversations/conv-1/read", "conv-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"already_read":false`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.Session.Role != "cs" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestConversationReadHandlerMapsNotFound(t *testing.T) {
	service := &fakeConversationReadService{err: workbench.ErrConversationNotFound}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-conversation-read-errors",
	})

	response := performConversationRead(handler, "Bearer "+token, "/api/v1/conversations/missing/read", "missing")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "conversation not found") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeConversationReadService struct {
	payload workbench.Payload
	err     error
	request workbench.ConversationReadRequest
}

func (service *fakeConversationReadService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeConversationReadService) MarkConversationRead(ctx context.Context, request workbench.ConversationReadRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performConversationRead(handler Handler, authorization string, target string, conversationID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationReadHandler(response, request)
	return response
}
