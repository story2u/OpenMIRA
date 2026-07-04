package messageshttp

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
	"wework-go/internal/messages"
)

func TestListHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeMessageService{payload: messages.Payload{
		"messages": []any{},
		"limit":    30,
	}}
	handler := New(testGuard(t), service)
	token := signMessagesToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-messages",
	})

	response := performMessages(handler, "Bearer "+token, "/api/v1/conversations/conv-001/messages?limit=30&after_cursor=1000:1:trace-a&fresh=1", "conv-001")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"messages":[]`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.ConversationID != "conv-001" || service.request.Limit != 30 || service.request.AfterCursor != "1000:1:trace-a" || !service.request.Fresh {
		t.Fatalf("unexpected service request: %+v", service.request)
	}
}

func TestListHandlerAllowsAdminSupervisorAndCSRoles(t *testing.T) {
	for _, role := range []string{"admin", "supervisor", "cs"} {
		t.Run(role, func(t *testing.T) {
			handler := New(testGuard(t), &fakeMessageService{payload: messages.Payload{"messages": []any{}}})
			token := signMessagesToken(t, "session-secret", map[string]any{
				"iss":  "wework-cloud",
				"sub":  role + "-001",
				"role": role,
				"exp":  int64(2000),
				"jti":  "jwt-" + role,
			})

			response := performMessages(handler, "Bearer "+token, "/api/v1/conversations/conv-001/messages", "conv-001")

			if response.Code != http.StatusOK {
				t.Fatalf("role %s response = %d %s", role, response.Code, response.Body.String())
			}
		})
	}
}

func TestListHandlerMapsLegacyAuthErrors(t *testing.T) {
	handler := New(testGuard(t), &fakeMessageService{})

	missing := performMessages(handler, "", "/api/v1/conversations/conv-001/messages", "conv-001")
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", missing.Code, missing.Body.String())
	}

	invalid := performMessages(handler, "Bearer invalid", "/api/v1/conversations/conv-001/messages", "conv-001")
	if invalid.Code != http.StatusUnauthorized || !strings.Contains(invalid.Body.String(), "session invalid or expired") {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestListHandlerRejectsOtherRoles(t *testing.T) {
	handler := New(testGuard(t), &fakeMessageService{})
	token := signMessagesToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "agent-001",
		"role": "agent",
		"exp":  int64(2000),
		"jti":  "jwt-agent",
	})

	response := performMessages(handler, "Bearer "+token, "/api/v1/conversations/conv-001/messages", "conv-001")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestListHandlerRequiresConfiguredService(t *testing.T) {
	handler := New(testGuard(t), nil)
	token := signMessagesToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-messages",
	})

	response := performMessages(handler, "Bearer "+token, "/api/v1/conversations/conv-001/messages", "conv-001")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "conversation messages service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestListHandlerMapsServiceErrorsToInternalServerError(t *testing.T) {
	handler := New(testGuard(t), &fakeMessageService{err: errors.New("messages unavailable")})
	token := signMessagesToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-messages",
	})

	response := performMessages(handler, "Bearer "+token, "/api/v1/conversations/conv-001/messages", "conv-001")

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("service error response = %d %s", response.Code, response.Body.String())
	}
}

type fakeMessageService struct {
	payload messages.Payload
	request messages.Request
	err     error
}

func (service *fakeMessageService) List(ctx context.Context, request messages.Request) (messages.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performMessages(handler Handler, authorization string, target string, conversationID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ListHandler(response, request)
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

func signMessagesToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeMessagesTokenPart(t, header)
	encodedClaims := encodeMessagesTokenPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeMessagesTokenPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
