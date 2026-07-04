package contactshttp

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

	"wework-go/internal/auth"
	"wework-go/internal/contacts"
)

func TestExternalContactHandlerSerializesPayload(t *testing.T) {
	service := &fakeContactsService{externalPayload: contacts.Payload{"enterprise_id": "ent-1", "external_userid": "wm-1"}}
	handler := New(testGuard(t), service)
	response := perform(handler.ExternalContactHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-external",
	}), "/api/v1/contacts/external/wm-1?enterprise_id=ent-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"external_userid":"wm-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.externalRequest.EnterpriseID != "ent-1" || service.externalRequest.ExternalUserID != "wm-1" {
		t.Fatalf("external request = %#v", service.externalRequest)
	}
}

func TestCorpUserHandlerSerializesPayload(t *testing.T) {
	service := &fakeContactsService{corpPayload: contacts.Payload{"enterprise_id": "ent-1", "userid": "zhangsan"}}
	handler := New(testGuard(t), service)
	response := perform(handler.CorpUserHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-corp",
	}), "/api/v1/contacts/corp-user/zhangsan?enterprise_id=ent-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"userid":"zhangsan"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.corpRequest.EnterpriseID != "ent-1" || service.corpRequest.UserID != "zhangsan" {
		t.Fatalf("corp request = %#v", service.corpRequest)
	}
}

func TestSyncExternalContactHandlerSerializesPayload(t *testing.T) {
	service := &fakeContactsService{syncPayload: contacts.Payload{"enterprise_id": "ent-1", "external_userid": "wm-1"}}
	handler := New(testGuard(t), service)
	response := performMethod(handler.SyncExternalContactHandler, http.MethodPost, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-sync",
	}), "/api/v1/contacts/sync/external-contacts?enterprise_id=ent-1&external_userid=wm-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"external_userid":"wm-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.syncRequest.EnterpriseID != "ent-1" || service.syncRequest.ExternalUserID != "wm-1" || service.syncRequest.Source != "manual" {
		t.Fatalf("sync request = %#v", service.syncRequest)
	}
}

func TestSyncFullHandlerSerializesPayload(t *testing.T) {
	service := &fakeContactsService{syncFullPayload: contacts.Payload{
		"enterprise_id":             "ent-1",
		"corp_users_synced":         2,
		"external_contacts_synced":  3,
		"external_contacts_skipped": 1,
	}}
	handler := New(testGuard(t), service)
	response := performMethod(handler.SyncFullHandler, http.MethodPost, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-sync-full",
	}), "/api/v1/contacts/sync/full?enterprise_id=ent-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"corp_users_synced":2`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.syncFullRequest.EnterpriseID != "ent-1" {
		t.Fatalf("sync full request = %#v", service.syncFullRequest)
	}
}

func TestRefreshStaleHandlerSerializesPayload(t *testing.T) {
	service := &fakeContactsService{refreshStalePayload: contacts.Payload{
		"enterprise_id":               "ent-1",
		"external_contacts_refreshed": 2,
		"external_contacts_skipped":   1,
		"corp_users_refreshed":        3,
	}}
	handler := New(testGuard(t), service)
	response := performMethod(handler.RefreshStaleHandler, http.MethodPost, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-refresh-stale",
	}), "/api/v1/contacts/sync/refresh-stale?enterprise_id=ent-1&limit=25")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"external_contacts_refreshed":2`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.refreshStaleRequest.EnterpriseID != "ent-1" || service.refreshStaleRequest.Limit != 25 {
		t.Fatalf("refresh stale request = %#v", service.refreshStaleRequest)
	}
}

func TestRefreshStaleHandlerUsesDefaultLimit(t *testing.T) {
	service := &fakeContactsService{refreshStalePayload: contacts.Payload{"enterprise_id": nil}}
	handler := New(testGuard(t), service)
	response := performMethod(handler.RefreshStaleHandler, http.MethodPost, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-refresh-default",
	}), "/api/v1/contacts/sync/refresh-stale")

	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.refreshStaleRequest.Limit != 50 {
		t.Fatalf("refresh stale limit = %d, want 50", service.refreshStaleRequest.Limit)
	}
}

func TestHandlersMapMissingRows(t *testing.T) {
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-missing",
	})
	handler := New(testGuard(t), &fakeContactsService{externalErr: contacts.ErrExternalContactNotFound, corpErr: contacts.ErrCorpUserNotFound})

	response := perform(handler.ExternalContactHandler, token, "/api/v1/contacts/external/wm-missing?enterprise_id=ent-1")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "external contact not found") {
		t.Fatalf("external response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.CorpUserHandler, token, "/api/v1/contacts/corp-user/missing?enterprise_id=ent-1")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "corp user not found") {
		t.Fatalf("corp response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersRequireAdminOrSupervisor(t *testing.T) {
	handler := New(testGuard(t), &fakeContactsService{})
	response := perform(handler.ExternalContactHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-cs",
	}), "/api/v1/contacts/external/wm-1")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersReturnServiceUnavailableWhenUnconfigured(t *testing.T) {
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-contact-unconfigured",
	})
	handler := New(testGuard(t), nil)

	response := perform(handler.ExternalContactHandler, token, "/api/v1/contacts/external/wm-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "contact sync service unavailable") {
		t.Fatalf("external response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.CorpUserHandler, token, "/api/v1/contacts/corp-user/zhangsan")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "contact sync service unavailable") {
		t.Fatalf("corp response = %d %s", response.Code, response.Body.String())
	}
	response = performMethod(handler.SyncExternalContactHandler, http.MethodPost, token, "/api/v1/contacts/sync/external-contacts?enterprise_id=ent-1&external_userid=wm-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "contact sync service unavailable") {
		t.Fatalf("sync response = %d %s", response.Code, response.Body.String())
	}
	response = performMethod(handler.SyncFullHandler, http.MethodPost, token, "/api/v1/contacts/sync/full?enterprise_id=ent-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "contact sync service unavailable") {
		t.Fatalf("sync full response = %d %s", response.Code, response.Body.String())
	}
	response = performMethod(handler.RefreshStaleHandler, http.MethodPost, token, "/api/v1/contacts/sync/refresh-stale?enterprise_id=ent-1")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "contact sync service unavailable") {
		t.Fatalf("refresh stale response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersRequireBearer(t *testing.T) {
	handler := New(testGuard(t), &fakeContactsService{})
	response := perform(handler.ExternalContactHandler, "", "/api/v1/contacts/external/wm-1")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeContactsService struct {
	externalPayload     contacts.Payload
	externalRequest     contacts.ExternalContactRequest
	externalErr         error
	corpPayload         contacts.Payload
	corpRequest         contacts.CorpUserRequest
	corpErr             error
	syncPayload         contacts.Payload
	syncRequest         contacts.SyncExternalContactRequest
	syncErr             error
	syncFullPayload     contacts.Payload
	syncFullRequest     contacts.SyncFullRequest
	syncFullErr         error
	refreshStalePayload contacts.Payload
	refreshStaleRequest contacts.RefreshStaleRequest
	refreshStaleErr     error
}

func (service *fakeContactsService) ExternalContact(ctx context.Context, request contacts.ExternalContactRequest) (contacts.Payload, error) {
	service.externalRequest = request
	if service.externalErr != nil {
		return nil, service.externalErr
	}
	return service.externalPayload, nil
}

func (service *fakeContactsService) CorpUser(ctx context.Context, request contacts.CorpUserRequest) (contacts.Payload, error) {
	service.corpRequest = request
	if service.corpErr != nil {
		return nil, service.corpErr
	}
	return service.corpPayload, nil
}

func (service *fakeContactsService) SyncExternalContact(ctx context.Context, request contacts.SyncExternalContactRequest) (contacts.Payload, error) {
	service.syncRequest = request
	if service.syncErr != nil {
		return nil, service.syncErr
	}
	return service.syncPayload, nil
}

func (service *fakeContactsService) SyncFull(ctx context.Context, request contacts.SyncFullRequest) (contacts.Payload, error) {
	service.syncFullRequest = request
	if service.syncFullErr != nil {
		return nil, service.syncFullErr
	}
	return service.syncFullPayload, nil
}

func (service *fakeContactsService) RefreshStale(ctx context.Context, request contacts.RefreshStaleRequest) (contacts.Payload, error) {
	service.refreshStaleRequest = request
	if service.refreshStaleErr != nil {
		return nil, service.refreshStaleErr
	}
	return service.refreshStalePayload, nil
}

func perform(handler http.HandlerFunc, authorization string, target string) *httptest.ResponseRecorder {
	return performMethod(handler, http.MethodGet, authorization, target)
}

func performMethod(handler http.HandlerFunc, method string, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if after, ok := strings.CutPrefix(request.URL.Path, "/api/v1/contacts/external/"); ok {
		request.SetPathValue("external_userid", strings.TrimSpace(after))
	}
	if after, ok := strings.CutPrefix(request.URL.Path, "/api/v1/contacts/corp-user/"); ok {
		request.SetPathValue("userid", strings.TrimSpace(after))
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}
}

func signToken(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	body := base64.RawURLEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + body))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + body + "." + signature
}
