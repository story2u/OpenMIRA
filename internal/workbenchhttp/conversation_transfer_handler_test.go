package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestConversationTransferHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeConversationTransferService{payload: workbench.Payload{"success": true, "transfer": workbench.Payload{"to_assignee_id": "cs-002"}}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "admin-001",
		"role":        "admin",
		"assignee_id": "admin-001",
		"exp":         int64(2000),
		"jti":         "jwt-conversation-transfer",
	})

	response := performConversationTransfer(handler, "Bearer "+token, "/api/v1/conversations/conv-1/transfer", "conv-1", `{"target_assignee_id":" cs-002 ","target_assignee_name":" 客服二 ","from_assignee_id":"cs-001","force":true}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"to_assignee_id":"cs-002"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.TargetAssigneeID != "cs-002" || service.request.TargetAssigneeName != "客服二" || service.request.FromAssigneeID != "cs-001" || !service.request.Force {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestConversationTransferHandlerRejectsCSRole(t *testing.T) {
	service := &fakeConversationTransferService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-conversation-transfer-cs",
	})

	response := performConversationTransfer(handler, "Bearer "+token, "/api/v1/conversations/conv-1/transfer", "conv-1", `{"target_assignee_id":"cs-002"}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.ConversationID != "" {
		t.Fatalf("service should not be called: %+v", service.request)
	}
}

func TestConversationTransferHandlerMapsTargetRequired(t *testing.T) {
	service := &fakeConversationTransferService{err: workbench.ErrConversationTransferTargetRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-conversation-transfer-error",
	})

	response := performConversationTransfer(handler, "Bearer "+token, "/api/v1/conversations/conv-1/transfer", "conv-1", `{}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "target_assignee_id is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeConversationTransferService struct {
	payload workbench.Payload
	err     error
	request workbench.ConversationTransferRequest
}

func (service *fakeConversationTransferService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeConversationTransferService) TransferConversation(ctx context.Context, request workbench.ConversationTransferRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performConversationTransfer(handler Handler, authorization string, target string, conversationID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationTransferHandler(response, request)
	return response
}
