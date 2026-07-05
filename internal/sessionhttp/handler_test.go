package sessionhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"im-go/internal/session"
)

func TestMeSerializesCurrentUser(t *testing.T) {
	handler := New(fakeCurrentUserService{
		response: session.MeResponse{
			AssigneeID:             "cs-001",
			AssigneeName:           "消息端一",
			Role:                   "cs",
			AIEnabled:              true,
			ExpiresAt:              "2026-06-28T00:00:00+00:00",
			PasswordChangeRequired: true,
		},
	})

	response := performMe(handler, "Bearer token")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"assignee_id":"cs-001"`, `"ai_enabled":true`, `"expires_at":"2026-06-28T00:00:00+00:00"`, `"password_change_required":true`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestMeMapsLegacyAuthErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		detail string
	}{
		{name: "missing bearer", err: session.ErrMissingBearerToken, detail: "missing bearer token"},
		{name: "invalid session", err: session.ErrInvalidOrExpiredSession, detail: "session invalid or expired"},
		{name: "unknown", err: errors.New("db failed"), detail: "internal server error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeCurrentUserService{err: testCase.err})
			response := performMe(handler, "")
			if response.Code == http.StatusOK {
				t.Fatalf("status = %d, want error; body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestAdminLoginSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{
		adminLoginResponse: session.LoginResponse{
			Success:      true,
			Token:        "jwt-admin",
			AssigneeID:   "admin",
			AssigneeName: "管理员",
			Role:         "admin",
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performAdminLogin(handler, `{"username":"admin","password":"secret"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-admin"`, `"role":"admin"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestAdminLoginPassesClientIPMetadata(t *testing.T) {
	service := &capturingLoginService{
		adminLoginResponse: session.LoginResponse{
			Success:      true,
			Token:        "jwt-admin",
			AssigneeID:   "admin",
			AssigneeName: "管理员",
			Role:         "admin",
		},
	}
	handler := New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin-login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	response := httptest.NewRecorder()

	handler.AdminLogin(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.clientIP != "203.0.113.9" {
		t.Fatalf("client ip metadata = %q, want forwarded ip", service.clientIP)
	}
}

func TestAdminLoginMapsLegacyErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{name: "not configured", err: session.ErrAdminLoginNotConfigured, status: http.StatusServiceUnavailable, detail: "admin login is not configured"},
		{name: "missing credentials", err: session.ErrAdminLoginMissingCredentials, status: http.StatusUnprocessableEntity, detail: "用户名和密码不能为空"},
		{name: "invalid credentials", err: session.ErrAdminLoginInvalidCredentials, status: http.StatusUnauthorized, detail: "用户名或密码错误"},
		{name: "missing password change", err: session.ErrAdminPasswordChangeMissingCredentials, status: http.StatusUnprocessableEntity, detail: "当前密码和新密码不能为空"},
		{name: "bad current password", err: session.ErrAdminPasswordChangeInvalidCurrent, status: http.StatusUnauthorized, detail: "当前密码错误"},
		{name: "bad new password", err: session.ErrAdminPasswordChangeInvalidNewPassword, status: http.StatusUnprocessableEntity, detail: "新密码至少 10 位且不能和当前密码相同"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{adminLoginErr: testCase.err})
			response := performAdminLogin(handler, `{}`)
			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, testCase.status, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestAdminLoginRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performAdminLogin(handler, `{"username":"admin","password":"secret"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session admin login service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestAdminChangePasswordSerializesResponse(t *testing.T) {
	handler := New(fakeSessionService{
		adminPasswordChangeResponse: session.LoginResponse{
			Success:      true,
			Token:        "jwt-admin-new",
			AssigneeID:   "root",
			AssigneeName: "管理员",
			Role:         "admin",
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performAdminChangePassword(handler, "Bearer jwt-change", `{"current_password":"1234567890","new_password":"new-password-123"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-admin-new"`, `"assignee_id":"root"`, `"password_change_required":false`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestLoginSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{
		loginResponse: session.LoginResponse{
			Success:      true,
			Token:        "jwt-cs",
			AssigneeID:   "cs-001",
			AssigneeName: "消息端一",
			Role:         "cs",
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performLogin(handler, `{"assignee_id":"cs-001","ttl_hours":168}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-cs"`, `"assignee_id":"cs-001"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestLoginMapsLegacyErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{name: "disabled", err: session.ErrPasswordlessLoginDisabled, status: http.StatusForbidden, detail: "passwordless login disabled"},
		{name: "missing assignee", err: session.ErrAssigneeIDRequired, status: http.StatusUnprocessableEntity, detail: "assignee_id is required"},
		{name: "not found", err: session.ErrAssigneeUserNotFoundOrDisabled, status: http.StatusForbidden, detail: "assignee user not found or disabled"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{loginErr: testCase.err})
			response := performLogin(handler, `{}`)
			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, testCase.status, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestLoginRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performLogin(handler, `{"assignee_id":"cs-001"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session login service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestCSLoginSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{
		csLoginResponse: session.LoginResponse{
			Success:      true,
			Token:        "jwt-cs",
			AssigneeID:   "cs-001",
			AssigneeName: "消息端一",
			Role:         "cs",
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performCSLogin(handler, `{"assignee_id":"cs-001","password":"secret"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-cs"`, `"assignee_id":"cs-001"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestCSLoginMapsLegacyErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{name: "missing credentials", err: session.ErrCSLoginMissingCredentials, status: http.StatusUnprocessableEntity, detail: "账号和密码不能为空"},
		{name: "not found", err: session.ErrCSLoginUserNotFoundOrDisabled, status: http.StatusUnauthorized, detail: "账号不存在或已禁用"},
		{name: "bad password", err: session.ErrCSLoginInvalidCredentials, status: http.StatusUnauthorized, detail: "账号或密码错误"},
		{name: "rate limited", err: session.LoginRateLimitError{Reason: "too many"}, status: http.StatusTooManyRequests, detail: "too many"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{csLoginErr: testCase.err})
			response := performCSLogin(handler, `{}`)
			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, testCase.status, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestCSLoginRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performCSLogin(handler, `{"assignee_id":"cs-001","password":"secret"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session cs login service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestGenerateCSTokenSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{
		generateCSResponse: session.GenerateCSTokenResponse{
			Success:      true,
			Token:        "jwt-cs",
			AssigneeID:   "cs-001",
			AssigneeName: "消息端一",
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performGenerateCSToken(handler, "Bearer admin-token", `{"assignee_id":"cs-001"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-cs"`, `"assignee_id":"cs-001"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"role"`) {
		t.Fatalf("generate cs token response must not include role: %s", response.Body.String())
	}
}

func TestGenerateCSTokenMapsLegacyErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{name: "missing bearer", err: session.ErrMissingBearerToken, status: http.StatusUnauthorized, detail: "missing bearer token"},
		{name: "invalid session", err: session.ErrInvalidOrExpiredSession, status: http.StatusUnauthorized, detail: "session invalid or expired"},
		{name: "permission denied", err: session.ErrPermissionDenied, status: http.StatusForbidden, detail: "permission denied"},
		{name: "missing assignee", err: session.ErrAssigneeIDRequired, status: http.StatusUnprocessableEntity, detail: "assignee_id is required"},
		{name: "not found", err: session.ErrCSUserNotFoundOrDisabled, status: http.StatusNotFound, detail: "CS user not found or disabled"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{generateCSErr: testCase.err})
			response := performGenerateCSToken(handler, "Bearer admin-token", `{}`)
			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, testCase.status, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestGenerateCSTokenRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performGenerateCSToken(handler, "Bearer admin-token", `{"assignee_id":"cs-001"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session generate cs token service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestRefreshSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{
		refreshResponse: session.RefreshResponse{
			Success:      true,
			Token:        "jwt-new",
			AssigneeID:   "cs-001",
			AssigneeName: "消息端一",
			Role:         "cs",
			AIEnabled:    true,
			ExpiresAt:    "2026-06-28T00:00:00+00:00",
		},
	})

	response := performRefresh(handler, "Bearer old-token")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"token":"jwt-new"`, `"ai_enabled":true`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

func TestRefreshMapsLegacyAuthErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		detail string
	}{
		{name: "missing bearer", err: session.ErrMissingBearerToken, detail: "missing bearer token"},
		{name: "invalid session", err: session.ErrInvalidOrExpiredSession, detail: "session invalid or expired"},
		{name: "unknown", err: errors.New("db failed"), detail: "internal server error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{refreshErr: testCase.err})
			response := performRefresh(handler, "")
			if response.Code == http.StatusOK {
				t.Fatalf("status = %d, want error; body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestRefreshRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performRefresh(handler, "Bearer old-token")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session refresh service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestLogoutSerializesLegacyResponse(t *testing.T) {
	handler := New(fakeSessionService{logoutResponse: session.LogoutResponse{Success: true}})

	response := performLogout(handler, "Bearer old-token")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestLogoutMapsLegacyAuthErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		detail string
	}{
		{name: "missing bearer", err: session.ErrMissingBearerToken, detail: "missing bearer token"},
		{name: "unknown", err: errors.New("db failed"), detail: "internal server error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := New(fakeSessionService{logoutErr: testCase.err})
			response := performLogout(handler, "")
			if response.Code == http.StatusOK {
				t.Fatalf("status = %d, want error; body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), testCase.detail) {
				t.Fatalf("body missing detail %q: %s", testCase.detail, response.Body.String())
			}
		})
	}
}

func TestLogoutRequiresConfiguredService(t *testing.T) {
	handler := New(fakeCurrentUserService{})

	response := performLogout(handler, "Bearer old-token")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session logout service is not configured") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

type fakeCurrentUserService struct {
	response session.MeResponse
	err      error
}

func (service fakeCurrentUserService) CurrentUser(ctx context.Context, authorization string) (session.MeResponse, error) {
	return service.response, service.err
}

type fakeSessionService struct {
	meResponse                  session.MeResponse
	meErr                       error
	adminLoginResponse          session.LoginResponse
	adminLoginErr               error
	adminPasswordChangeResponse session.LoginResponse
	adminPasswordChangeErr      error
	loginResponse               session.LoginResponse
	loginErr                    error
	csLoginResponse             session.LoginResponse
	csLoginErr                  error
	generateCSResponse          session.GenerateCSTokenResponse
	generateCSErr               error
	refreshResponse             session.RefreshResponse
	refreshErr                  error
	logoutResponse              session.LogoutResponse
	logoutErr                   error
}

func (service fakeSessionService) CurrentUser(ctx context.Context, authorization string) (session.MeResponse, error) {
	return service.meResponse, service.meErr
}

func (service fakeSessionService) AdminLogin(ctx context.Context, username string, password string, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return service.adminLoginResponse, service.adminLoginErr
}

func (service fakeSessionService) ChangeAdminPassword(ctx context.Context, authorization string, request session.AdminPasswordChangeRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return service.adminPasswordChangeResponse, service.adminPasswordChangeErr
}

func (service fakeSessionService) AssigneeLogin(ctx context.Context, request session.AssigneeLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return service.loginResponse, service.loginErr
}

func (service fakeSessionService) CSLogin(ctx context.Context, request session.CSLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return service.csLoginResponse, service.csLoginErr
}

func (service fakeSessionService) GenerateCSToken(ctx context.Context, authorization string, assigneeID string, metadata ...session.LoginMetadata) (session.GenerateCSTokenResponse, error) {
	return service.generateCSResponse, service.generateCSErr
}

func (service fakeSessionService) Refresh(ctx context.Context, authorization string) (session.RefreshResponse, error) {
	return service.refreshResponse, service.refreshErr
}

func (service fakeSessionService) Logout(ctx context.Context, authorization string, metadata ...session.LoginMetadata) (session.LogoutResponse, error) {
	return service.logoutResponse, service.logoutErr
}

type capturingLoginService struct {
	fakeSessionService
	adminLoginResponse session.LoginResponse
	clientIP           string
}

func (service *capturingLoginService) CurrentUser(ctx context.Context, authorization string) (session.MeResponse, error) {
	return session.MeResponse{}, nil
}

func (service *capturingLoginService) AdminLogin(ctx context.Context, username string, password string, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	if len(metadata) > 0 {
		service.clientIP = metadata[0].ClientIP
	}
	return service.adminLoginResponse, nil
}

func performMe(handler Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session/me", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.Me(response, request)
	return response
}

func performAdminLogin(handler Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin-login", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.AdminLogin(response, request)
	return response
}

func performAdminChangePassword(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin/change-password", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AdminChangePassword(response, request)
	return response
}

func performLogin(handler Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/login", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.Login(response, request)
	return response
}

func performCSLogin(handler Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/cs-login", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.CSLogin(response, request)
	return response
}

func performGenerateCSToken(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin/generate-cs-token", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.GenerateCSToken(response, request)
	return response
}

func performRefresh(handler Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/refresh", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.Refresh(response, request)
	return response
}

func performLogout(handler Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/logout", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.Logout(response, request)
	return response
}
