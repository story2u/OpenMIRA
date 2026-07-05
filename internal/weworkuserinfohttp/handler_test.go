package weworkuserinfohttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"im-go/internal/auth"
	"im-go/internal/weworkuserinfo"
)

func TestLastHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=device-1", nil)

	handler.LastHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestLastHandlerRejectsCSRole(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LastHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("cs response = %d %s", response.Code, response.Body.String())
	}
}

func TestLastHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LastHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestLastHandlerDefaultsToNotFoundWithoutStore(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=%20device-1%20", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LastHandler(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"found":false`) || !strings.Contains(body, `"device_id":"device-1"`) {
		t.Fatalf("not found response = %d %s", response.Code, body)
	}
}

func TestLastHandlerSerializesDebugPayload(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeLastStore{payload: map[string]any{"account_name": "消息端一"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LastHandler(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"found":true`) || !strings.Contains(body, `"account_name":"消息端一"`) {
		t.Fatalf("payload response = %d %s", response.Code, body)
	}
}

func TestLastHandlerReportsStoreError(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeLastStore{err: errors.New("store failed")})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LastHandler(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("store error response = %d %s", response.Code, response.Body.String())
	}
}

func TestCandidatesHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, nil, &fakeCandidatesService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=device-1", nil)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestCandidatesHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, nil, &fakeCandidatesService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestCandidatesHandlerRejectsInvalidLimit(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, nil, &fakeCandidatesService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=device-1&limit=51", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "limit must be between 1 and 50") {
		t.Fatalf("invalid limit response = %d %s", response.Code, response.Body.String())
	}
}

func TestCandidatesHandlerReportsMissingService(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "user info candidate service is unavailable") {
		t.Fatalf("missing service response = %d %s", response.Code, response.Body.String())
	}
}

func TestCandidatesHandlerSerializesCandidates(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	service := &fakeCandidatesService{result: weworkuserinfo.CandidatesResult{
		Success:           true,
		RequiresSelection: true,
		DeviceID:          "device-1",
		AccountName:       "张三",
		OrganizationName:  "企微组织",
		EnterpriseID:      "ent-1",
		Candidates: []weworkuserinfo.CandidatePayload{
			{EnterpriseID: "ent-1", UserID: "zhangsan", Name: "张三", DepartmentJSON: []any{float64(1)}, Position: "dev", Avatar: "avatar"},
		},
	}}
	handler := New(guard, nil, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=%20device-1%20&limit=2", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("response code = %d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["success"] != true || payload["requires_selection"] != true || payload["device_id"] != "device-1" || payload["enterprise_id"] != "ent-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if service.deviceID != "device-1" || service.limit != 2 {
		t.Fatalf("service inputs = %q/%d", service.deviceID, service.limit)
	}
}

func TestCandidatesHandlerReportsServiceError(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, nil, &fakeCandidatesService{err: errors.New("store failed")})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/candidates?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CandidatesHandler(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("service error response = %d %s", response.Code, response.Body.String())
	}
}

func TestRequestHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := NewWithRequest(auth.Guard{}, nil, nil, &fakeRequestService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1"}`))

	handler.RequestHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestRequestHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := NewWithRequest(guard, nil, nil, &fakeRequestService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":" "}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestRequestHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &fakeRequestService{payload: map[string]any{"success": true, "device_id": "device-1", "msg_id": "user-info-1", "task_id": "task-1", "selected_wework_user_id": ""}}
	handler := NewWithRequest(guard, nil, nil, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":" device-1 ","agent_id":"sdk:a","source":"system"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"msg_id":"user-info-1"`) {
		t.Fatalf("request response = %d %s", response.Code, response.Body.String())
	}
	if service.request.DeviceID != " device-1 " || service.request.AgentID != "sdk:a" || service.request.Source != "system" || service.request.Operator != "user-1" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestRequestHandlerReportsUnavailableAndManualSelection(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := NewWithRequest(guard, nil, nil, &fakeRequestService{err: weworkuserinfo.ErrTaskCreatorUnavailable})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "user info request service is unavailable") {
		t.Fatalf("unavailable response = %d %s", response.Code, response.Body.String())
	}

	handler = NewWithRequest(guard, nil, nil, &fakeRequestService{err: weworkuserinfo.ErrManualSelectionUnsupported})
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1","selected_wework_user_id":"wm-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "manual user info selection is not available") {
		t.Fatalf("manual selection response = %d %s", response.Code, response.Body.String())
	}

	handler = NewWithRequest(guard, nil, nil, &fakeRequestService{err: weworkuserinfo.ErrSelectedIdentityMismatch})
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1","selected_wework_user_id":"wm-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "selected wework_user_id does not match current account") {
		t.Fatalf("selection mismatch response = %d %s", response.Code, response.Body.String())
	}

	handler = NewWithRequest(guard, nil, nil, &fakeRequestService{err: weworkuserinfo.ErrSDKRouteUnavailable})
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RequestHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "SDK route is not configured for this device") {
		t.Fatalf("sdk route response = %d %s", response.Code, response.Body.String())
	}
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "im-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "wework-user-info-" + role})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}

type fakeLastStore struct {
	payload map[string]any
	err     error
}

func (store fakeLastStore) LastUserInfoPayload(ctx context.Context, deviceID string) (map[string]any, bool, error) {
	_ = ctx
	_ = deviceID
	if store.err != nil {
		return nil, false, store.err
	}
	if store.payload == nil {
		return nil, false, nil
	}
	return store.payload, true, nil
}

type fakeCandidatesService struct {
	result   weworkuserinfo.CandidatesResult
	err      error
	deviceID string
	limit    int
}

func (service *fakeCandidatesService) Candidates(ctx context.Context, deviceID string, limit int) (weworkuserinfo.CandidatesResult, error) {
	_ = ctx
	service.deviceID = deviceID
	service.limit = limit
	if service.err != nil {
		return weworkuserinfo.CandidatesResult{}, service.err
	}
	return service.result, nil
}

type fakeRequestService struct {
	payload map[string]any
	err     error
	request weworkuserinfo.RequestUserInfoRequest
}

func (service *fakeRequestService) RequestUserInfo(ctx context.Context, request weworkuserinfo.RequestUserInfoRequest) (map[string]any, error) {
	_ = ctx
	service.request = request
	if strings.TrimSpace(request.DeviceID) == "" {
		return nil, weworkuserinfo.ErrDeviceIDRequired
	}
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}
