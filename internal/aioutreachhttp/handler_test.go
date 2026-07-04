package aioutreachhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/aioutreach"
)

func TestConversationHandlerReturnsOKEnvelope(t *testing.T) {
	service := &fakeService{response: aioutreach.ConversationResponse{
		ConversationID: "conv-1",
		Messages: []aioutreach.FormattedMessage{{
			MsgID: "msg-1", From: "customer", Source: "archive_history", MsgType: "text", Content: "hello", MsgTime: 1782892800000,
		}},
	}}
	handler := New(service, "agent-token")

	response := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation?corp_id=corp-a&customer_id=customer-1&external_userid=external-1&wechat=wechat-a&limit=50", "agent-token")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"code":0`, `"msg":"ok"`, `"conversation_id":"conv-1"`, `"msgid":"msg-1"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("response body %s missing %s", response.Body.String(), want)
		}
	}
	if service.request.CorpID != "corp-a" || service.request.CustomerID != "customer-1" || service.request.ExternalUserID != "external-1" || service.request.Wechat != "wechat-a" || service.request.Limit != 50 {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestConversationHandlerMapsBusinessErrorEnvelope(t *testing.T) {
	handler := New(&fakeService{err: aioutreach.Error{StatusCode: http.StatusConflict, Code: aioutreach.CodeConversationCorp, Message: "conversation does not belong to corp_id"}}, "agent-token")

	response := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation?corp_id=corp-a&customer_id=customer-1&wechat=wechat-a", "agent-token")
	if response.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"code":40904`, `"msg":"conversation does not belong to corp_id"`, `"data":{}`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("response body %s missing %s", response.Body.String(), want)
		}
	}
}

func TestConversationHandlerRequiresAgentToken(t *testing.T) {
	handler := New(&fakeService{}, "agent-token")

	missing := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation", "")
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), "missing X-Agent-Token header") {
		t.Fatalf("missing response = %d %s", missing.Code, missing.Body.String())
	}
	invalid := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation", "bad-token")
	if invalid.Code != http.StatusUnauthorized || !strings.Contains(invalid.Body.String(), "invalid agent token") {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestConversationHandlerReportsMissingConfiguredAgentToken(t *testing.T) {
	handler := New(&fakeService{}, "")

	response := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation", "agent-token")
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "AGENT_API_TOKEN") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationHandlerValidatesLimit(t *testing.T) {
	handler := New(&fakeService{}, "agent-token")

	response := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation?limit=abc", "agent-token")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationHandlerMapsUnexpectedServiceError(t *testing.T) {
	handler := New(&fakeService{err: errors.New("db down")}, "agent-token")

	response := performConversation(handler, "/api/v1/platform-agent/ai-outreach/conversation", "agent-token")
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSendHandlerReturnsOKEnvelope(t *testing.T) {
	service := &fakeService{sendResponse: aioutreach.SendResponse{
		SendStatus:     "accepted",
		ConversationID: "conv-1",
		SystemMsgID:    "trace-1",
		SystemMsgIDs:   []string{"trace-1"},
		SystemTaskIDs:  []string{"task-1"},
		SendTime:       "2026-07-01T09:30:00+00:00",
	}}
	handler := New(service, "agent-token")

	response := performSend(handler, `{"corp_id":"corp-a","customer_id":"customer-1","external_userid":"external-1","user_id":42,"wechat":"wechat-a","plan_id":"plan-1","task_id":"task-ext-1","reply_messages":[{"type":"text","content":{"text":"hello"}}]}`, "agent-token")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"code":0`, `"msg":"ok"`, `"send_status":"accepted"`, `"system_task_ids":["task-1"]`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("response body %s missing %s", response.Body.String(), want)
		}
	}
	if service.sendRequest.CorpID != "corp-a" || service.sendRequest.UserID != "42" || len(service.sendRequest.ReplyMessages) != 1 {
		t.Fatalf("send request = %#v", service.sendRequest)
	}
}

func TestSendHandlerMapsBusinessErrorEnvelope(t *testing.T) {
	handler := New(&fakeService{sendErr: aioutreach.Error{StatusCode: http.StatusConflict, Code: aioutreach.CodeAgentMissing, Message: "matched account missing agent_id"}}, "agent-token")

	response := performSend(handler, `{"corp_id":"corp-a","customer_id":"customer-1","wechat":"wechat-a","plan_id":"plan-1","task_id":"task-ext-1","reply_messages":[{"type":"text","content":"hello"}]}`, "agent-token")
	if response.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"code":40906`, `"msg":"matched account missing agent_id"`, `"data":{}`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("response body %s missing %s", response.Body.String(), want)
		}
	}
}

func TestSendHandlerValidatesBody(t *testing.T) {
	handler := New(&fakeService{}, "agent-token")

	invalidJSON := performSend(handler, `{`, "agent-token")
	if invalidJSON.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidJSON.Body.String(), "invalid request body") {
		t.Fatalf("invalid JSON response = %d %s", invalidJSON.Code, invalidJSON.Body.String())
	}
	missingCorp := performSend(handler, `{"customer_id":"customer-1","plan_id":"plan-1","task_id":"task-ext-1"}`, "agent-token")
	if missingCorp.Code != http.StatusUnprocessableEntity || !strings.Contains(missingCorp.Body.String(), "corp_id is required") {
		t.Fatalf("missing field response = %d %s", missingCorp.Code, missingCorp.Body.String())
	}
}

func performConversation(handler Handler, target string, agentToken string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if agentToken != "" {
		request.Header.Set("X-Agent-Token", agentToken)
	}
	response := httptest.NewRecorder()
	handler.ConversationHandler(response, request)
	return response
}

func performSend(handler Handler, body string, agentToken string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform-agent/ai-outreach/send", strings.NewReader(body))
	if agentToken != "" {
		request.Header.Set("X-Agent-Token", agentToken)
	}
	response := httptest.NewRecorder()
	handler.SendHandler(response, request)
	return response
}

type fakeService struct {
	response     aioutreach.ConversationResponse
	request      aioutreach.ConversationRequest
	err          error
	sendResponse aioutreach.SendResponse
	sendRequest  aioutreach.SendRequest
	sendErr      error
}

func (service *fakeService) QueryConversation(ctx context.Context, request aioutreach.ConversationRequest) (aioutreach.ConversationResponse, error) {
	service.request = request
	if service.err != nil {
		return aioutreach.ConversationResponse{}, service.err
	}
	return service.response, nil
}

func (service *fakeService) Send(ctx context.Context, request aioutreach.SendRequest) (aioutreach.SendResponse, error) {
	service.sendRequest = request
	if service.sendErr != nil {
		return aioutreach.SendResponse{}, service.sendErr
	}
	return service.sendResponse, nil
}
