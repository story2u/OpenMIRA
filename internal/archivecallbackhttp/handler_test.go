package archivecallbackhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/auth"
)

func TestVerifyHandlerReturnsPlainEcho(t *testing.T) {
	service := &fakeService{verifyPlain: "hello"}
	handler := New(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/archive/callback/ent-1?msg_signature=sig&timestamp=123&nonce=n&echostr=encrypted", nil)
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.VerifyHandler(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "hello" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if service.verify.EnterpriseKey != "ent-1" || service.verify.Signature != "sig" || service.verify.EchoStr != "encrypted" {
		t.Fatalf("verify request = %#v", service.verify)
	}
}

func TestEventHandlerReturnsSuccess(t *testing.T) {
	service := &fakeService{}
	handler := New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/callback/ent-1?msg_signature=sig&timestamp=123&nonce=n", strings.NewReader(`<xml><Encrypt>encrypted</Encrypt></xml>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "success" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if service.event.EnterpriseKey != "ent-1" || service.event.Signature != "sig" || !strings.Contains(service.event.XMLBody, "Encrypt") {
		t.Fatalf("event request = %#v", service.event)
	}
}

func TestEventHandlerMapsArchiveErrors(t *testing.T) {
	service := &fakeService{eventErr: archivecallback.HTTPError{StatusCode: http.StatusBadRequest, Detail: "missing signature query: msg_signature/timestamp/nonce"}}
	handler := New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/callback/ent-1", strings.NewReader(`<xml/>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "missing signature query") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	handler := New(nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/archive/callback/ent-1", strings.NewReader(`<xml/>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "not configured") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestReceiptsHandlerSerializesPaginatedPayload(t *testing.T) {
	receipts := &fakeReceiptStore{
		total: 5,
		receipts: []archivecallback.Receipt{
			{
				ReceiptID:        "acr-1",
				EnterpriseID:     "ent-1",
				EventName:        "change_external_contact",
				CallbackEventKey: "cb-1",
				MsgSignature:     "sig",
				Timestamp:        "1710000000",
				Nonce:            "nonce",
				EncryptHash:      "hash",
				PlainPayload:     "<xml/>",
				Status:           "processed",
				DuplicateCount:   1,
				CreatedAt:        time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
				UpdatedAt:        time.Date(2026, 6, 30, 10, 1, 0, 0, time.UTC),
			},
		},
	}
	handler := NewWithReceipts(nil, testGuard(t), receipts)
	response := performReceipts(handler, "Bearer "+signArchiveCallbackToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-archive-callback-receipts",
	}), "/api/v1/archive/callback/receipts?enterprise_id=ent-1&event_name=change_external_contact&page=2&limit=2")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"receipt_id":"acr-1"`) || !strings.Contains(body, `"page":2`) || !strings.Contains(body, `"total_pages":3`) {
		t.Fatalf("unexpected body: %s", body)
	}
	if receipts.countFilter.EnterpriseID != "ent-1" || receipts.listFilter.EventName != "change_external_contact" || receipts.listFilter.Limit != 2 || receipts.listFilter.Offset != 2 {
		t.Fatalf("filters count=%#v list=%#v", receipts.countFilter, receipts.listFilter)
	}
}

func TestReceiptsHandlerRejectsNonAdminRole(t *testing.T) {
	handler := NewWithReceipts(nil, testGuard(t), &fakeReceiptStore{})
	response := performReceipts(handler, "Bearer "+signArchiveCallbackToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-archive-callback-receipts-cs",
	}), "/api/v1/archive/callback/receipts")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReceiptsHandlerRequiresConfiguredStore(t *testing.T) {
	handler := NewWithReceipts(nil, testGuard(t), nil)
	response := performReceipts(handler, "Bearer "+signArchiveCallbackToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-archive-callback-receipts-missing",
	}), "/api/v1/archive/callback/receipts")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "receipt service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReceiptsHandlerRejectsInvalidLimit(t *testing.T) {
	handler := NewWithReceipts(nil, testGuard(t), &fakeReceiptStore{})
	response := performReceipts(handler, "Bearer "+signArchiveCallbackToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-archive-callback-receipts-invalid",
	}), "/api/v1/archive/callback/receipts?limit=501")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	verify      archivecallback.VerifyRequest
	verifyPlain string
	verifyErr   error
	event       archivecallback.EventRequest
	eventErr    error
}

func (service *fakeService) VerifyURL(ctx context.Context, request archivecallback.VerifyRequest) (string, error) {
	service.verify = request
	if service.verifyErr != nil {
		return "", service.verifyErr
	}
	return service.verifyPlain, nil
}

func (service *fakeService) HandleEvent(ctx context.Context, request archivecallback.EventRequest) (archivecallback.Result, error) {
	service.event = request
	if service.eventErr != nil {
		return archivecallback.Result{}, service.eventErr
	}
	return archivecallback.Result{Created: true}, nil
}

type fakeReceiptStore struct {
	total       int
	receipts    []archivecallback.Receipt
	countFilter archivecallback.ReceiptListFilter
	listFilter  archivecallback.ReceiptListFilter
}

func (store *fakeReceiptStore) CountRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) (int, error) {
	store.countFilter = filter
	return store.total, nil
}

func (store *fakeReceiptStore) ListRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) ([]archivecallback.Receipt, error) {
	store.listFilter = filter
	return store.receipts, nil
}

func performReceipts(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReceiptsHandler(response, request)
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

func signArchiveCallbackToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeArchiveCallbackTokenPart(t, header)
	encodedClaims := encodeArchiveCallbackTokenPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeArchiveCallbackTokenPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
