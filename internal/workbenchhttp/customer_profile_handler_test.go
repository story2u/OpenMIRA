package workbenchhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestCustomerProfileHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeCustomerProfileService{payload: workbench.Payload{"editor_update": workbench.ProjectionRow{"remark_name": "新备注"}}}
	handler := Handler{Guard: testGuard(t), CustomerProfile: service}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "admin-1",
		"role":        "admin",
		"exp":         int64(2000),
		"jti":         "jwt-customer-profile",
		"assignee_id": "admin-1",
	})

	response := performCustomerProfile(handler, "Bearer "+token, "/api/v1/conversations/conv-1/customer-profile", " conv-1 ", `{"remark_name":" 新备注 ","description":"说明","mobile":"138","backup_mobiles":["139"],"tags":["VIP"]}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"remark_name":"新备注"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.Body.RemarkName != "新备注" || service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

func TestCustomerProfileHandlerMapsRemoteError(t *testing.T) {
	service := &fakeCustomerProfileService{err: workbench.CustomerProfileRemoteError{Operation: "externalcontact/remark", Err: errors.New("bad gateway")}}
	handler := Handler{Guard: testGuard(t), CustomerProfile: service}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-1",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-customer-profile-remote",
	})

	response := performCustomerProfile(handler, "Bearer "+token, "/api/v1/conversations/conv-1/customer-profile", "conv-1", `{"remark_name":"新备注"}`)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "externalcontact/remark") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCustomerProfileHandlerRejectsMissingService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-1",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-customer-profile-missing",
	})

	response := performCustomerProfile(handler, "Bearer "+token, "/api/v1/conversations/conv-1/customer-profile", "conv-1", `{"remark_name":"新备注"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestContactProfileResolveHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeContactProfileResolveService{payload: workbench.Payload{"conversation_id": "conv-1", "changed_fields": []string{"sender_remark"}}}
	handler := Handler{Guard: testGuard(t), ContactResolve: service}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-1",
		"role":        "cs",
		"exp":         int64(2000),
		"jti":         "jwt-contact-profile-resolve",
		"assignee_id": "cs-1",
	})

	response := performContactProfileResolve(handler, "Bearer "+token, "/api/v1/conversations/conv-1/contact-profile/resolve", " conv-1 ")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"conv-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.Session.Role != "cs" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

func TestContactProfileResolveHandlerMapsUnavailable(t *testing.T) {
	service := &fakeContactProfileResolveService{err: workbench.ErrContactProfileResolveUnavailable}
	handler := Handler{Guard: testGuard(t), ContactResolve: service}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-1",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-contact-profile-resolve-unavailable",
	})

	response := performContactProfileResolve(handler, "Bearer "+token, "/api/v1/conversations/conv-1/contact-profile/resolve", "conv-1")

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "remote enterprise contact lookup unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestContactProfileRefreshHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeContactProfileRefreshService{payload: workbench.Payload{"conversation_id": "conv-1", "conversation_ids": []string{"conv-1"}}}
	handler := Handler{Guard: testGuard(t), ContactRefresh: service}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-1",
		"role":        "cs",
		"exp":         int64(2000),
		"jti":         "jwt-contact-profile-refresh",
		"assignee_id": "cs-1",
	})

	response := performContactProfileRefresh(handler, "Bearer "+token, "/api/v1/conversations/conv-1/contact-profile/refresh", " conv-1 ")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"conv-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.ConversationID != "conv-1" || service.request.Session.Role != "cs" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

type fakeCustomerProfileService struct {
	request workbench.CustomerProfileUpdateRequest
	payload workbench.Payload
	err     error
}

func (service *fakeCustomerProfileService) UpdateConversationCustomerProfile(ctx context.Context, request workbench.CustomerProfileUpdateRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

type fakeContactProfileResolveService struct {
	request workbench.ContactProfileResolveRequest
	payload workbench.Payload
	err     error
}

func (service *fakeContactProfileResolveService) ResolveConversationContactProfile(ctx context.Context, request workbench.ContactProfileResolveRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

type fakeContactProfileRefreshService struct {
	request workbench.ContactProfileRefreshRequest
	payload workbench.Payload
	err     error
}

func (service *fakeContactProfileRefreshService) RefreshConversationContactProfile(ctx context.Context, request workbench.ContactProfileRefreshRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performCustomerProfile(handler Handler, authorization string, target string, conversationID string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPatch, target, strings.NewReader(body))
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.CustomerProfileHandler(response, request)
	return response
}

func performContactProfileResolve(handler Handler, authorization string, target string, conversationID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ContactProfileResolveHandler(response, request)
	return response
}

func performContactProfileRefresh(handler Handler, authorization string, target string, conversationID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("conversation_id", conversationID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ContactProfileRefreshHandler(response, request)
	return response
}
