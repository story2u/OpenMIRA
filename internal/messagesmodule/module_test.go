package messagesmodule

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
	"wework-go/internal/config"
	"wework-go/internal/messages"
)

func TestNewRejectsMissingSessionSecret(t *testing.T) {
	_, err := New(Options{Config: config.Config{SessionJWTIssuer: "wework-cloud"}})
	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("New error = %v, want %v", err, auth.ErrMissingSecret)
	}
}

func TestNewRequiresStore(t *testing.T) {
	_, err := New(Options{Config: config.Config{SessionJWTSecret: "session-secret"}})
	if !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrStoreRequired)
	}
}

func TestNewRequiresBlacklistWhenRequested(t *testing.T) {
	_, err := New(Options{
		Config:                config.Config{SessionJWTSecret: "session-secret"},
		Store:                 moduleStore{},
		RequireBlacklistStore: true,
	})
	if !errors.Is(err, ErrBlacklistStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrBlacklistStoreRequired)
	}
}

func TestNewBuildsUnmountedMessagesHandler(t *testing.T) {
	messageID := int64(42)
	module, err := New(Options{
		Config: config.Config{
			SessionJWTSecret: "session-secret",
			SessionJWTIssuer: "wework-cloud",
		},
		Store: moduleStore{page: messages.Page{Records: []messages.Record{{
			MessageID:      &messageID,
			TraceID:        "trace-001",
			ConversationID: "conv-001",
			SenderID:       "external-001",
			SenderName:     "客户一",
			Content:        "hello",
			MsgType:        "text",
			Direction:      "incoming",
			Timestamp:      time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC),
			CreatedAt:      time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC),
		}}, Total: 1}},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Service == nil || module.StoreRepository != nil {
		t.Fatalf("unexpected module state: service=%v store_repo=%v", module.Service, module.StoreRepository)
	}

	token := signMessagesModuleToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-messages",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/conv-001/messages", nil)
	request.SetPathValue("conversation_id", "conv-001")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	module.Handler.ListHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"messages":[`) || !strings.Contains(response.Body.String(), `"trace_id":"trace-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

type moduleStore struct {
	page messages.Page
}

func (store moduleStore) List(ctx context.Context, query messages.Query) (messages.Page, error) {
	return store.page, nil
}

func signMessagesModuleToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeMessagesModulePart(t, header)
	encodedClaims := encodeMessagesModulePart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeMessagesModulePart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
