package weworkloginhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/weworklogin"
)

func TestStatusHandlerRequiresAdminSupervisorOrCS(t *testing.T) {
	handler := New(auth.Guard{}, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/login/status?device_id=device-1", nil)

	handler.StatusHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestStatusHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/login/status?device_id=", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.StatusHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestStatusHandlerRejectsInvalidBool(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/login/status?device_id=device-1&include_qrcode=maybe", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.StatusHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "include_qrcode must be a boolean") {
		t.Fatalf("invalid bool response = %d %s", response.Code, response.Body.String())
	}
}

func TestStatusHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	service := &recordingStatusService{payload: map[string]any{"status": "idle", "logged_in": false}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/login/status?device_id=%20device-1%20&live=true&include_qrcode=false", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.StatusHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"idle"`) {
		t.Fatalf("status response = %d %s", response.Code, response.Body.String())
	}
	if service.request.DeviceID != "device-1" || !service.request.Live || service.request.IncludeQRCode {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestStatusHandlerReportsServiceErrors(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{err: errors.New("store failed")})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/login/status?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.StatusHandler(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("service error response = %d %s", response.Code, response.Body.String())
	}
}

func TestQRCodeHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/qrcode", strings.NewReader(`{"device_id":"device-1"}`))

	handler.QRCodeHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestQRCodeHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/qrcode", strings.NewReader(`{"device_id":" "}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.QRCodeHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestQRCodeHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &recordingStatusService{qrcodePayload: map[string]any{"status": "waiting", "task_id": "task-1", "qrcode_refresh_mode": "background"}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/qrcode", strings.NewReader(`{"device_id":" device-1 ","agent_id":"sdk:a","source":"system","timeout_seconds":30}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.QRCodeHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"qrcode_refresh_mode":"background"`) {
		t.Fatalf("qrcode response = %d %s", response.Code, response.Body.String())
	}
	if service.qrcodeRequest.DeviceID != " device-1 " || service.qrcodeRequest.AgentID != "sdk:a" || service.qrcodeRequest.Source != "system" || service.qrcodeRequest.TimeoutSeconds != 30 {
		t.Fatalf("qrcode request = %+v", service.qrcodeRequest)
	}
}

func TestQRCodeHandlerReportsUnavailableDependencies(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{qrcodeErr: weworklogin.ErrTaskCreatorUnavailable})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/qrcode", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.QRCodeHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "wework login qrcode service is unavailable") {
		t.Fatalf("unavailable response = %d %s", response.Code, response.Body.String())
	}
}

func TestVerifyCodeHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/verify-code", strings.NewReader(`{"device_id":"device-1","verify_code":"123456"}`))

	handler.VerifyCodeHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestVerifyCodeHandlerRejectsMissingVerifyCode(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/verify-code", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.VerifyCodeHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "verify_code is required") {
		t.Fatalf("missing code response = %d %s", response.Code, response.Body.String())
	}
}

func TestVerifyCodeHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &recordingStatusService{verifyPayload: map[string]any{"success": true, "status": "verifying", "task_id": "task-1"}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/verify-code", strings.NewReader(`{"device_id":" device-1 ","verify_code":" 123456 ","verify_type":"sms","agent_id":"sdk:a","source":"system"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.VerifyCodeHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"verifying"`) {
		t.Fatalf("verify response = %d %s", response.Code, response.Body.String())
	}
	if service.verifyRequest.DeviceID != " device-1 " || service.verifyRequest.VerifyCode != " 123456 " || service.verifyRequest.VerifyType != "sms" || service.verifyRequest.AgentID != "sdk:a" || service.verifyRequest.Source != "system" {
		t.Fatalf("verify request = %+v", service.verifyRequest)
	}
}

func TestVerifyCodeHandlerReportsUnavailableDependencies(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{verifyErr: weworklogin.ErrLoginSessionWriterUnavailable})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/login/verify-code", strings.NewReader(`{"device_id":"device-1","verify_code":"123456"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.VerifyCodeHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "wework login verify service is unavailable") {
		t.Fatalf("unavailable response = %d %s", response.Code, response.Body.String())
	}
}

func TestLogoutHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/logout", strings.NewReader(`{"device_id":"device-1"}`))

	handler.LogoutHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestLogoutHandlerRejectsBlankDeviceID(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/logout", strings.NewReader(`{"device_id":" "}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LogoutHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("blank device response = %d %s", response.Code, response.Body.String())
	}
}

func TestLogoutHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &recordingStatusService{logoutPayload: map[string]any{"success": true, "status": "idle", "task_id": "task-1"}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/logout", strings.NewReader(`{"device_id":" device-1 ","agent_id":"sdk:a","source":"system"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LogoutHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"idle"`) {
		t.Fatalf("logout response = %d %s", response.Code, response.Body.String())
	}
	if service.logoutRequest.DeviceID != " device-1 " || service.logoutRequest.AgentID != "sdk:a" || service.logoutRequest.Source != "system" || service.logoutRequest.Operator != "user-1" {
		t.Fatalf("logout request = %+v", service.logoutRequest)
	}
}

func TestLogoutHandlerReportsUnavailableDependencies(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeStatusService{logoutErr: weworklogin.ErrTaskCreatorUnavailable})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/wework/logout", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.LogoutHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "wework logout service is unavailable") {
		t.Fatalf("unavailable response = %d %s", response.Code, response.Body.String())
	}
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "wework-login-status-" + role})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}

type fakeStatusService struct {
	err       error
	qrcodeErr error
	verifyErr error
	logoutErr error
}

func (service fakeStatusService) Status(ctx context.Context, request weworklogin.StatusRequest) (map[string]any, error) {
	_ = ctx
	_ = request
	if service.err != nil {
		return nil, service.err
	}
	return map[string]any{"status": "idle"}, nil
}

func (service fakeStatusService) QRCode(ctx context.Context, request weworklogin.QRCodeRequest) (map[string]any, error) {
	_ = ctx
	if strings.TrimSpace(request.DeviceID) == "" {
		return nil, weworklogin.ErrDeviceIDRequired
	}
	if service.qrcodeErr != nil {
		return nil, service.qrcodeErr
	}
	return map[string]any{"status": "waiting", "task_id": "task-1", "qrcode_refresh_mode": "background"}, nil
}

func (service fakeStatusService) VerifyCode(ctx context.Context, request weworklogin.VerifyCodeRequest) (map[string]any, error) {
	_ = ctx
	if strings.TrimSpace(request.DeviceID) == "" {
		return nil, weworklogin.ErrDeviceIDRequired
	}
	if strings.TrimSpace(request.VerifyCode) == "" {
		return nil, weworklogin.ErrVerifyCodeRequired
	}
	if service.verifyErr != nil {
		return nil, service.verifyErr
	}
	return map[string]any{"success": true, "status": "verifying", "task_id": "task-1"}, nil
}

func (service fakeStatusService) Logout(ctx context.Context, request weworklogin.LogoutRequest) (map[string]any, error) {
	_ = ctx
	if strings.TrimSpace(request.DeviceID) == "" {
		return nil, weworklogin.ErrDeviceIDRequired
	}
	if service.logoutErr != nil {
		return nil, service.logoutErr
	}
	return map[string]any{"success": true, "status": "idle", "task_id": "task-1"}, nil
}

type recordingStatusService struct {
	payload       map[string]any
	request       weworklogin.StatusRequest
	qrcodePayload map[string]any
	qrcodeRequest weworklogin.QRCodeRequest
	verifyPayload map[string]any
	verifyRequest weworklogin.VerifyCodeRequest
	logoutPayload map[string]any
	logoutRequest weworklogin.LogoutRequest
}

func (service *recordingStatusService) Status(ctx context.Context, request weworklogin.StatusRequest) (map[string]any, error) {
	_ = ctx
	service.request = request
	return service.payload, nil
}

func (service *recordingStatusService) QRCode(ctx context.Context, request weworklogin.QRCodeRequest) (map[string]any, error) {
	_ = ctx
	service.qrcodeRequest = request
	return service.qrcodePayload, nil
}

func (service *recordingStatusService) VerifyCode(ctx context.Context, request weworklogin.VerifyCodeRequest) (map[string]any, error) {
	_ = ctx
	service.verifyRequest = request
	return service.verifyPayload, nil
}

func (service *recordingStatusService) Logout(ctx context.Context, request weworklogin.LogoutRequest) (map[string]any, error) {
	_ = ctx
	service.logoutRequest = request
	return service.logoutPayload, nil
}
