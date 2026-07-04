// Package weworkloginhttp adapts WeWork login status routes.
package weworkloginhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/weworklogin"
)

// Service resolves WeWork login route payloads.
type Service interface {
	Status(ctx context.Context, request weworklogin.StatusRequest) (map[string]any, error)
	QRCode(ctx context.Context, request weworklogin.QRCodeRequest) (map[string]any, error)
	VerifyCode(ctx context.Context, request weworklogin.VerifyCodeRequest) (map[string]any, error)
	Logout(ctx context.Context, request weworklogin.LogoutRequest) (map[string]any, error)
}

// Handler owns WeWork login route serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a WeWork login HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// QRCodeHandler serializes POST /wework/login/qrcode.
func (handler Handler) QRCodeHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework login qrcode service is unavailable")
		return
	}
	var request struct {
		DeviceID       string `json:"device_id"`
		AgentID        string `json:"agent_id"`
		Source         string `json:"source"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Service.QRCode(r.Context(), weworklogin.QRCodeRequest{
		DeviceID:       request.DeviceID,
		AgentID:        request.AgentID,
		Source:         request.Source,
		TimeoutSeconds: request.TimeoutSeconds,
	})
	if err != nil {
		switch {
		case errors.Is(err, weworklogin.ErrDeviceIDRequired):
			writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		case errors.Is(err, weworklogin.ErrStoreUnavailable), errors.Is(err, weworklogin.ErrLoginSessionWriterUnavailable), errors.Is(err, weworklogin.ErrTaskCreatorUnavailable):
			writeError(w, http.StatusServiceUnavailable, "wework login qrcode service is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// VerifyCodeHandler serializes POST /wework/login/verify-code.
func (handler Handler) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework login verify service is unavailable")
		return
	}
	var request struct {
		DeviceID   string `json:"device_id"`
		VerifyCode string `json:"verify_code"`
		VerifyType string `json:"verify_type"`
		AgentID    string `json:"agent_id"`
		Source     string `json:"source"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Service.VerifyCode(r.Context(), weworklogin.VerifyCodeRequest{
		DeviceID:   request.DeviceID,
		VerifyCode: request.VerifyCode,
		VerifyType: request.VerifyType,
		AgentID:    request.AgentID,
		Source:     request.Source,
	})
	if err != nil {
		switch {
		case errors.Is(err, weworklogin.ErrDeviceIDRequired):
			writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		case errors.Is(err, weworklogin.ErrVerifyCodeRequired):
			writeError(w, http.StatusUnprocessableEntity, "verify_code is required")
		case errors.Is(err, weworklogin.ErrStoreUnavailable), errors.Is(err, weworklogin.ErrLoginSessionWriterUnavailable), errors.Is(err, weworklogin.ErrTaskCreatorUnavailable):
			writeError(w, http.StatusServiceUnavailable, "wework login verify service is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// LogoutHandler serializes POST /wework/logout.
func (handler Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework logout service is unavailable")
		return
	}
	var request struct {
		DeviceID string `json:"device_id"`
		AgentID  string `json:"agent_id"`
		Source   string `json:"source"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Service.Logout(r.Context(), weworklogin.LogoutRequest{
		DeviceID: request.DeviceID,
		AgentID:  request.AgentID,
		Source:   request.Source,
		Operator: session.AssigneeID,
	})
	if err != nil {
		switch {
		case errors.Is(err, weworklogin.ErrDeviceIDRequired):
			writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		case errors.Is(err, weworklogin.ErrStoreUnavailable), errors.Is(err, weworklogin.ErrLoginSessionWriterUnavailable), errors.Is(err, weworklogin.ErrTaskCreatorUnavailable):
			writeError(w, http.StatusServiceUnavailable, "wework logout service is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// StatusHandler serializes GET /wework/login/status.
func (handler Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		return
	}
	live, ok := parseBoolQuery(r.URL.Query().Get("live"), false)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "live must be a boolean")
		return
	}
	includeQRCode, ok := parseBoolQuery(r.URL.Query().Get("include_qrcode"), true)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "include_qrcode must be a boolean")
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework login status service is unavailable")
		return
	}
	payload, err := handler.Service.Status(r.Context(), weworklogin.StatusRequest{
		DeviceID:      deviceID,
		Live:          live,
		IncludeQRCode: includeQRCode,
	})
	if err != nil {
		switch {
		case errors.Is(err, weworklogin.ErrDeviceIDRequired):
			writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		case errors.Is(err, weworklogin.ErrStoreUnavailable):
			writeError(w, http.StatusServiceUnavailable, "wework login status service is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func parseBoolQuery(raw string, fallback bool) (bool, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback, true
	}
	parsed, err := strconv.ParseBool(text)
	if err != nil {
		return false, false
	}
	return parsed, true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
