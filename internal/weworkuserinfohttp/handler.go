// Package weworkuserinfohttp adapts read-only WeWork user-info debug routes.
package weworkuserinfohttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/weworkuserinfo"
)

// LastPayloadStore reads the in-process user-info debug payload for a device.
type LastPayloadStore interface {
	LastUserInfoPayload(ctx context.Context, deviceID string) (map[string]any, bool, error)
}

// CandidatesService resolves possible internal userid choices for a device.
type CandidatesService interface {
	Candidates(ctx context.Context, deviceID string, limit int) (weworkuserinfo.CandidatesResult, error)
}

// RequestService submits SDK-backed user-info requests.
type RequestService interface {
	RequestUserInfo(ctx context.Context, request weworkuserinfo.RequestUserInfoRequest) (map[string]any, error)
}

// Handler owns WeWork user-info debug route serialization.
type Handler struct {
	Guard      auth.Guard
	Store      LastPayloadStore
	Candidates CandidatesService
	Request    RequestService
}

// New builds a read-only user-info debug HTTP adapter.
func New(guard auth.Guard, store LastPayloadStore, candidates ...CandidatesService) Handler {
	handler := Handler{Guard: guard, Store: store}
	if len(candidates) > 0 {
		handler.Candidates = candidates[0]
	}
	return handler
}

// NewWithRequest builds a user-info adapter with both read and write services.
func NewWithRequest(guard auth.Guard, store LastPayloadStore, candidates CandidatesService, request RequestService) Handler {
	handler := New(guard, store, candidates)
	handler.Request = request
	return handler
}

// LastHandler serializes GET /wework/user-info/last.
func (handler Handler) LastHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		return
	}
	if handler.Store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "device_id": deviceID})
		return
	}
	payload, found, err := handler.Store.LastUserInfoPayload(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "device_id": deviceID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"found":     true,
		"device_id": deviceID,
		"payload":   payload,
	})
}

// CandidatesHandler serializes GET /wework/user-info/candidates.
func (handler Handler) CandidatesHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		return
	}
	limit, ok := parseLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "limit must be between 1 and 50")
		return
	}
	if handler.Candidates == nil {
		writeError(w, http.StatusServiceUnavailable, "user info candidate service is unavailable")
		return
	}
	result, err := handler.Candidates.Candidates(r.Context(), deviceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// RequestHandler serializes POST /wework/user-info/request.
func (handler Handler) RequestHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Request == nil {
		writeError(w, http.StatusServiceUnavailable, "user info request service is unavailable")
		return
	}
	var request struct {
		DeviceID             string `json:"device_id"`
		AgentID              string `json:"agent_id"`
		Source               string `json:"source"`
		SelectedWeWorkUserID string `json:"selected_wework_user_id"`
		SelectedEnterpriseID string `json:"selected_enterprise_id"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Request.RequestUserInfo(r.Context(), weworkuserinfo.RequestUserInfoRequest{
		DeviceID:             request.DeviceID,
		AgentID:              request.AgentID,
		Source:               request.Source,
		SelectedWeWorkUserID: request.SelectedWeWorkUserID,
		SelectedEnterpriseID: request.SelectedEnterpriseID,
		Operator:             session.AssigneeID,
	})
	if err != nil {
		switch {
		case errors.Is(err, weworkuserinfo.ErrDeviceIDRequired):
			writeError(w, http.StatusUnprocessableEntity, "device_id is required")
		case errors.Is(err, weworkuserinfo.ErrSelectedIdentityMismatch):
			writeError(w, http.StatusUnprocessableEntity, "selected wework_user_id does not match current account")
		case errors.Is(err, weworkuserinfo.ErrTaskCreatorUnavailable):
			writeError(w, http.StatusServiceUnavailable, "user info request service is unavailable")
		case errors.Is(err, weworkuserinfo.ErrManualSelectionUnsupported):
			writeError(w, http.StatusConflict, "manual user info selection is not available in go candidate")
		case errors.Is(err, weworkuserinfo.ErrStoreUnavailable):
			writeError(w, http.StatusServiceUnavailable, "user info request service is unavailable")
		case errors.Is(err, weworkuserinfo.ErrSDKRouteUnavailable):
			writeError(w, http.StatusConflict, "SDK route is not configured for this device")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func parseLimit(raw string) (int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 20, true
	}
	limit, err := strconv.Atoi(text)
	if err != nil || limit < 1 || limit > 50 {
		return 0, false
	}
	return limit, true
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
