// Package devicesmanualhttp adapts manual device writes to HTTP.
package devicesmanualhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/devicesmanual"
)

// Service stores manual devices and builds legacy response payloads.
type Service interface {
	UpsertManualDevice(ctx context.Context, command devicesmanual.UpsertCommand) (map[string]any, error)
	DeleteManualDevice(ctx context.Context, agentID string, deviceID string) (map[string]any, error)
}

// Handler owns /api/v1/devices/manual serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a manual devices HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// UpsertHandler serializes POST /api/v1/devices/manual.
func (handler Handler) UpsertHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "manual device service is not configured")
		return
	}
	var request struct {
		AgentID        string `json:"agent_id"`
		DeviceID       string `json:"device_id"`
		Model          string `json:"model"`
		AndroidVersion string `json:"android_version"`
		WeWorkLoggedIn *bool  `json:"wework_logged_in"`
		Online         *bool  `json:"online"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	online := true
	if request.Online != nil {
		online = *request.Online
	}
	payload, err := handler.Service.UpsertManualDevice(r.Context(), devicesmanual.UpsertCommand{
		AgentID:        request.AgentID,
		DeviceID:       request.DeviceID,
		Online:         online,
		WeWorkLoggedIn: request.WeWorkLoggedIn,
		Model:          request.Model,
		AndroidVersion: request.AndroidVersion,
	})
	writeServiceResult(w, payload, err)
}

// DeleteHandler serializes DELETE /api/v1/devices/manual.
func (handler Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "manual device service is not configured")
		return
	}
	payload, err := handler.Service.DeleteManualDevice(r.Context(), r.URL.Query().Get("agent_id"), r.URL.Query().Get("device_id"))
	writeServiceResult(w, payload, err)
}

func writeServiceResult(w http.ResponseWriter, payload map[string]any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesmanual.ErrAgentIDRequired), errors.Is(err, devicesmanual.ErrDeviceIDRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, devicesmanual.ErrStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
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
