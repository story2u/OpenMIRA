// Package devicebridgehttp adapts MYT call-audio bridge status endpoints.
package devicebridgehttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/devicebridge"
)

// Service reads and writes bridge status entries.
type Service interface {
	Read(deviceID string) map[string]any
	StatusForRow(row map[string]any) map[string]any
	Write(deviceID string, payload map[string]any) (map[string]any, error)
}

// TargetService lists bridge targets for external supervisors.
type TargetService interface {
	ListTargets(ctx context.Context) ([]devicebridge.Target, error)
}

// Handler owns the call-audio bridge status routes.
type Handler struct {
	Service              Service
	Targets              TargetService
	MediaConfig          devicebridge.MediaConfig
	Guard                auth.Guard
	AgentToken           string
	AllowLegacyAgentAuth bool
}

// New builds a device bridge HTTP adapter.
func New(service Service, guard auth.Guard, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{Service: service, Guard: guard, AgentToken: agentToken, AllowLegacyAgentAuth: allowLegacyAgentAuth}
}

// NewWithTargets builds a device bridge HTTP adapter with target discovery.
func NewWithTargets(service Service, targets TargetService, mediaConfig devicebridge.MediaConfig, guard auth.Guard, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{Service: service, Targets: targets, MediaConfig: mediaConfig, Guard: guard, AgentToken: agentToken, AllowLegacyAgentAuth: allowLegacyAgentAuth}
}

// TargetsHandler serializes GET /api/v1/devices/call-audio-bridge/targets.
func (handler Handler) TargetsHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAnyAuth(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Agent-Token"), w) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device bridge service is not configured")
		return
	}
	var targets []devicebridge.Target
	var err error
	if handler.Targets != nil {
		targets, err = handler.Targets.ListTargets(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}
	payloadTargets := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		payloadTargets = append(payloadTargets, target.Payload(handler.Service.StatusForRow(target.Row())))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":             true,
		"targets":             payloadTargets,
		"media_stream_config": handler.MediaConfig.Status(),
	})
}

// StatusHandler serializes GET /api/v1/devices/{device_id}/call-audio-bridge/status.
func (handler Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device bridge service is not configured")
		return
	}
	deviceID := strings.TrimSpace(r.PathValue("device_id"))
	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"device_id":         deviceID,
		"call_audio_bridge": handler.Service.Read(deviceID),
	})
}

// ReportStatusHandler serializes POST /api/v1/devices/{device_id}/call-audio-bridge/status.
func (handler Handler) ReportStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAnyAuth(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Agent-Token"), w) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device bridge service is not configured")
		return
	}
	var payload map[string]any
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	status, err := handler.Service.Write(r.PathValue("device_id"), payload)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{
			"success":           true,
			"device_id":         strings.TrimSpace(r.PathValue("device_id")),
			"call_audio_bridge": status,
		})
	case errors.Is(err, devicebridge.ErrDeviceIDRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (handler Handler) requireAnyAuth(ctx context.Context, authorization string, agentToken string, w http.ResponseWriter) bool {
	if auth.ParseBearerToken(authorization) != "" {
		if _, err := handler.Guard.RequireRoles(ctx, authorization); err != nil {
			writeAuthError(w, err)
			return false
		}
		return true
	}
	expectedAgentToken := strings.TrimSpace(handler.AgentToken)
	if expectedAgentToken != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(agentToken)), []byte(expectedAgentToken)) == 1 {
		return true
	}
	if handler.AllowLegacyAgentAuth {
		return true
	}
	writeError(w, http.StatusUnauthorized, "authentication required")
	return false
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
