// Package sessionhttp adapts session services to HTTP handlers.
// The handlers are intentionally not registered in the phase-one mux yet; they
// provide tested response serialization before traffic is cut over.
package sessionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"im-go/internal/session"
)

// CurrentUserService is the service contract needed by the /me handler.
type CurrentUserService interface {
	CurrentUser(ctx context.Context, authorization string) (session.MeResponse, error)
}

// AdminLoginService is the service contract needed by the /admin-login handler.
type AdminLoginService interface {
	AdminLogin(ctx context.Context, username string, password string, metadata ...session.LoginMetadata) (session.LoginResponse, error)
}

// AdminPasswordChangeService is needed by the forced admin password reset flow.
type AdminPasswordChangeService interface {
	ChangeAdminPassword(ctx context.Context, authorization string, request session.AdminPasswordChangeRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error)
}

// AssigneeLoginService is the service contract needed by the /login handler.
type AssigneeLoginService interface {
	AssigneeLogin(ctx context.Context, request session.AssigneeLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error)
}

// CSLoginService is the service contract needed by the /cs-login handler.
type CSLoginService interface {
	CSLogin(ctx context.Context, request session.CSLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error)
}

// GenerateCSTokenService is needed by the /admin/generate-cs-token handler.
type GenerateCSTokenService interface {
	GenerateCSToken(ctx context.Context, authorization string, assigneeID string, metadata ...session.LoginMetadata) (session.GenerateCSTokenResponse, error)
}

// RefreshService is the service contract needed by the /refresh handler.
type RefreshService interface {
	Refresh(ctx context.Context, authorization string) (session.RefreshResponse, error)
}

// LogoutService is the service contract needed by the /logout handler.
type LogoutService interface {
	Logout(ctx context.Context, authorization string, metadata ...session.LoginMetadata) (session.LogoutResponse, error)
}

// Handler contains session endpoint HTTP adapters.
type Handler struct {
	currentUser         CurrentUserService
	adminLogin          AdminLoginService
	adminPasswordChange AdminPasswordChangeService
	login               AssigneeLoginService
	csLogin             CSLoginService
	generateCS          GenerateCSTokenService
	refresh             RefreshService
	logout              LogoutService
}

// New builds a session HTTP adapter.
func New(currentUser CurrentUserService) Handler {
	handler := Handler{currentUser: currentUser}
	if adminLogin, ok := currentUser.(AdminLoginService); ok {
		handler.adminLogin = adminLogin
	}
	if adminPasswordChange, ok := currentUser.(AdminPasswordChangeService); ok {
		handler.adminPasswordChange = adminPasswordChange
	}
	if login, ok := currentUser.(AssigneeLoginService); ok {
		handler.login = login
	}
	if csLogin, ok := currentUser.(CSLoginService); ok {
		handler.csLogin = csLogin
	}
	if generateCS, ok := currentUser.(GenerateCSTokenService); ok {
		handler.generateCS = generateCS
	}
	if refresh, ok := currentUser.(RefreshService); ok {
		handler.refresh = refresh
	}
	if logout, ok := currentUser.(LogoutService); ok {
		handler.logout = logout
	}
	return handler
}

// Me serializes the legacy /api/v1/session/me response and error shape.
func (handler Handler) Me(w http.ResponseWriter, r *http.Request) {
	if handler.currentUser == nil {
		writeError(w, http.StatusServiceUnavailable, "session service is not configured")
		return
	}
	response, err := handler.currentUser.CurrentUser(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// AdminLogin serializes the legacy /api/v1/session/admin-login endpoint.
func (handler Handler) AdminLogin(w http.ResponseWriter, r *http.Request) {
	if handler.adminLogin == nil {
		writeError(w, http.StatusServiceUnavailable, "session admin login service is not configured")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	response, err := handler.adminLogin.AdminLogin(r.Context(), body.Username, body.Password, loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// AdminChangePassword serializes the mandatory admin password reset endpoint.
func (handler Handler) AdminChangePassword(w http.ResponseWriter, r *http.Request) {
	if handler.adminPasswordChange == nil {
		writeError(w, http.StatusServiceUnavailable, "session admin password change service is not configured")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	response, err := handler.adminPasswordChange.ChangeAdminPassword(r.Context(), r.Header.Get("Authorization"), session.AdminPasswordChangeRequest{
		CurrentPassword: body.CurrentPassword,
		NewPassword:     body.NewPassword,
	}, loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// Login serializes the legacy passwordless /api/v1/session/login endpoint.
func (handler Handler) Login(w http.ResponseWriter, r *http.Request) {
	if handler.login == nil {
		writeError(w, http.StatusServiceUnavailable, "session login service is not configured")
		return
	}
	var body struct {
		AssigneeID   string `json:"assignee_id"`
		AssigneeName string `json:"assignee_name"`
		TTLHours     int    `json:"ttl_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	response, err := handler.login.AssigneeLogin(r.Context(), session.AssigneeLoginRequest{
		AssigneeID:   body.AssigneeID,
		AssigneeName: body.AssigneeName,
		TTLHours:     body.TTLHours,
	}, loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// CSLogin serializes the legacy /api/v1/session/cs-login endpoint.
func (handler Handler) CSLogin(w http.ResponseWriter, r *http.Request) {
	if handler.csLogin == nil {
		writeError(w, http.StatusServiceUnavailable, "session cs login service is not configured")
		return
	}
	var body struct {
		AssigneeID string `json:"assignee_id"`
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	response, err := handler.csLogin.CSLogin(r.Context(), session.CSLoginRequest{
		AssigneeID: body.AssigneeID,
		Password:   body.Password,
	}, loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// GenerateCSToken serializes the legacy admin CS token minting endpoint.
func (handler Handler) GenerateCSToken(w http.ResponseWriter, r *http.Request) {
	if handler.generateCS == nil {
		writeError(w, http.StatusServiceUnavailable, "session generate cs token service is not configured")
		return
	}
	var body struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	response, err := handler.generateCS.GenerateCSToken(r.Context(), r.Header.Get("Authorization"), body.AssigneeID, loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// Refresh serializes the legacy /api/v1/session/refresh response and errors.
func (handler Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if handler.refresh == nil {
		writeError(w, http.StatusServiceUnavailable, "session refresh service is not configured")
		return
	}
	response, err := handler.refresh.Refresh(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// Logout serializes the legacy /api/v1/session/logout response and errors.
func (handler Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if handler.logout == nil {
		writeError(w, http.StatusServiceUnavailable, "session logout service is not configured")
		return
	}
	response, err := handler.logout.Logout(r.Context(), r.Header.Get("Authorization"), loginMetadataFromRequest(r))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, session.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, session.ErrAdminLoginNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "admin login is not configured")
	case errors.Is(err, session.ErrAdminLoginMissingCredentials):
		writeError(w, http.StatusUnprocessableEntity, "用户名和密码不能为空")
	case errors.Is(err, session.ErrAdminLoginInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
	case errors.Is(err, session.ErrAdminPasswordChangeMissingCredentials):
		writeError(w, http.StatusUnprocessableEntity, "当前密码和新密码不能为空")
	case errors.Is(err, session.ErrAdminPasswordChangeInvalidCurrent):
		writeError(w, http.StatusUnauthorized, "当前密码错误")
	case errors.Is(err, session.ErrAdminPasswordChangeInvalidNewPassword):
		writeError(w, http.StatusUnprocessableEntity, "新密码至少 10 位且不能和当前密码相同")
	case errors.Is(err, session.ErrPasswordlessLoginDisabled):
		writeError(w, http.StatusForbidden, "passwordless login disabled")
	case errors.Is(err, session.ErrAssigneeIDRequired):
		writeError(w, http.StatusUnprocessableEntity, "assignee_id is required")
	case errors.Is(err, session.ErrAssigneeUserNotFoundOrDisabled):
		writeError(w, http.StatusForbidden, "assignee user not found or disabled")
	case errors.Is(err, session.ErrCSLoginMissingCredentials):
		writeError(w, http.StatusUnprocessableEntity, "账号和密码不能为空")
	case errors.Is(err, session.ErrCSLoginUserNotFoundOrDisabled):
		writeError(w, http.StatusUnauthorized, "账号不存在或已禁用")
	case errors.Is(err, session.ErrCSLoginInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "账号或密码错误")
	case errors.Is(err, session.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, session.ErrCSUserNotFoundOrDisabled):
		writeError(w, http.StatusNotFound, "CS user not found or disabled")
	case errors.Is(err, session.ErrLoginRateLimited):
		writeError(w, http.StatusTooManyRequests, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func loginMetadataFromRequest(r *http.Request) session.LoginMetadata {
	return session.LoginMetadata{ClientIP: clientIP(r)}
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return strings.TrimSpace(parsedHost)
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
