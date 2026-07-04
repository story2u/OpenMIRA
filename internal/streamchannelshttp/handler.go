// Package streamchannelshttp adapts the realtime channel catalog candidate to
// HTTP. It preserves legacy role protection while leaving the WebSocket hub
// migration for the realtime gateway phase.
package streamchannelshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"wework-go/internal/auth"
)

// Service builds the stream channel catalog payload.
type Service interface {
	Channels(ctx context.Context) (map[string]any, error)
}

// Handler owns GET /api/v1/stream/channels.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a realtime stream channel HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// ChannelsHandler serializes GET /api/v1/stream/channels.
func (handler Handler) ChannelsHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "stream channels service is not configured")
		return
	}
	payload, err := handler.Service.Channels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// writeAuthError maps auth guard failures to legacy-compatible JSON errors.
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, auth.ErrBlacklistUnavailable):
		writeError(w, http.StatusServiceUnavailable, "session token blacklist is unavailable")
	default:
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	}
}

// writeJSON serializes compact JSON responses with the legacy charset header.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError serializes FastAPI-style detail errors.
func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
