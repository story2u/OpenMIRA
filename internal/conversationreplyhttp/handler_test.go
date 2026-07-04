package conversationreplyhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/conversationreply"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
)

func TestReplyHandlerSerializesServiceResponse(t *testing.T) {
	service := &fakeReplyService{response: conversationreply.Response{
		Success: true,
		Task: tasks.Record{
			TaskID: "task-1",
			Status: tasks.StatusAccepted,
		},
		Message: conversationreply.MessageEcho{ConversationID: "conv-1", TraceID: "trace-1", SendStatus: "pending"},
	}}
	handler := New(testGuard(t), service)
	token := signReplyToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-reply",
	})

	response := performReply(handler, "Bearer "+token, "conv-1", `{"device_id":"device-1","sender_id":"external-1","sender_name":"客户","message":"hello"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"send_status":"pending"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.conversationID != "conv-1" || service.request.Message != "hello" || service.request.Operator != "cs-001" {
		t.Fatalf("unexpected service call: conversation=%q request=%+v", service.conversationID, service.request)
	}
}

func TestReplyHandlerMapsAuthAndValidationErrors(t *testing.T) {
	handler := New(testGuard(t), &fakeReplyService{})

	missing := performReply(handler, "", "conv-1", `{}`)
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), "missing bearer token") {
		t.Fatalf("missing response = %d %s", missing.Code, missing.Body.String())
	}

	token := signReplyToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "agent",
		"exp":  int64(2000),
		"jti":  "jwt-agent",
	})
	forbidden := performReply(handler, "Bearer "+token, "conv-1", `{}`)
	if forbidden.Code != http.StatusForbidden || !strings.Contains(forbidden.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", forbidden.Code, forbidden.Body.String())
	}
}

func TestReplyHandlerMapsServiceErrors(t *testing.T) {
	token := signReplyToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-reply",
	})

	invalid := New(testGuard(t), &fakeReplyService{err: errors.Join(conversationreply.ErrInvalidRequest, errors.New("message is required"))})
	invalidResponse := performReply(invalid, "Bearer "+token, "conv-1", `{}`)
	if invalidResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidResponse.Body.String(), "message is required") {
		t.Fatalf("invalid response = %d %s", invalidResponse.Code, invalidResponse.Body.String())
	}

	missing := New(testGuard(t), nil)
	missingResponse := performReply(missing, "Bearer "+token, "conv-1", `{}`)
	if missingResponse.Code != http.StatusServiceUnavailable || !strings.Contains(missingResponse.Body.String(), "conversation reply service is not configured") {
		t.Fatalf("missing service response = %d %s", missingResponse.Code, missingResponse.Body.String())
	}

	outgoingMissing := New(testGuard(t), &fakeReplyService{err: conversationreply.ErrOutgoingMissing})
	outgoingResponse := performReply(outgoingMissing, "Bearer "+token, "conv-1", `{}`)
	if outgoingResponse.Code != http.StatusServiceUnavailable || !strings.Contains(outgoingResponse.Body.String(), "conversation reply outgoing recorder is not configured") {
		t.Fatalf("outgoing response = %d %s", outgoingResponse.Code, outgoingResponse.Body.String())
	}

	conflict := New(testGuard(t), &fakeReplyService{err: conversationreply.ErrSuggestionConflict})
	conflictResponse := performReply(conflict, "Bearer "+token, "conv-1", `{}`)
	if conflictResponse.Code != http.StatusConflict || !strings.Contains(conflictResponse.Body.String(), "AI 回复已由其他终端处理") {
		t.Fatalf("conflict response = %d %s", conflictResponse.Code, conflictResponse.Body.String())
	}

	offline := New(testGuard(t), &fakeReplyService{err: sendguard.DeviceOfflineError{Detail: "offline"}})
	offlineResponse := performReply(offline, "Bearer "+token, "conv-1", `{}`)
	if offlineResponse.Code != http.StatusConflict || !strings.Contains(offlineResponse.Body.String(), "offline") {
		t.Fatalf("offline response = %d %s", offlineResponse.Code, offlineResponse.Body.String())
	}

	contactIdentity := New(testGuard(t), &fakeReplyService{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}})
	contactIdentityResponse := performReply(contactIdentity, "Bearer "+token, "conv-1", `{}`)
	if contactIdentityResponse.Code != http.StatusConflict || !strings.Contains(contactIdentityResponse.Body.String(), "refresh failed") {
		t.Fatalf("contact identity response = %d %s", contactIdentityResponse.Code, contactIdentityResponse.Body.String())
	}
}

type fakeReplyService struct {
	response       conversationreply.Response
	request        conversationreply.Request
	conversationID string
	err            error
}

func (service *fakeReplyService) Reply(_ context.Context, conversationID string, request conversationreply.Request) (conversationreply.Response, error) {
	service.conversationID = conversationID
	service.request = request
	if service.err != nil {
		return conversationreply.Response{}, service.err
	}
	return service.response, nil
}

func performReply(handler Handler, authorization string, conversationID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/reply", strings.NewReader(body))
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReplyHandler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time {
		return time.Unix(1000, 0).UTC()
	}
	return auth.Guard{Verifier: verifier}
}

func signReplyToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeReplyTokenPart(t, header)
	encodedClaims := encodeReplyTokenPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeReplyTokenPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
