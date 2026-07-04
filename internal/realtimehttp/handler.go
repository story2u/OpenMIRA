// Package realtimehttp adapts realtime compensation routes to net/http.
package realtimehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/realtime"
)

type replayService interface {
	ReplayEvents(ctx context.Context, request realtime.ReplayRequest) (realtime.Payload, error)
}

type snapshotService interface {
	SnapshotWorkbench(ctx context.Context) (realtime.Payload, error)
}

// Handler owns read-only realtime compensation route serialization.
type Handler struct {
	Guard    auth.Guard
	Replay   replayService
	Snapshot snapshotService
}

// New builds a realtime compensation HTTP handler.
func New(guard auth.Guard, service any) Handler {
	handler := Handler{Guard: guard}
	if replay, ok := service.(replayService); ok {
		handler.Replay = replay
	}
	if snapshot, ok := service.(snapshotService); ok {
		handler.Snapshot = snapshot
	}
	return handler
}

// ReplayEventsHandler serializes GET /api/v1/realtime/events/replay.
func (handler Handler) ReplayEventsHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Replay == nil {
		writeError(w, http.StatusServiceUnavailable, "realtime replay service is not configured")
		return
	}
	query := r.URL.Query()
	afterCursor, ok := queryInt64(query.Get("after_cursor"), 0)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid after_cursor")
		return
	}
	limit, ok := boundedLimit(query.Get("limit"), realtime.DefaultReplayLimit, realtime.MaxReplayLimit)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "invalid limit")
		return
	}
	payload, err := handler.Replay.ReplayEvents(r.Context(), realtime.ReplayRequest{
		Scope:       query.Get("scope"),
		AfterCursor: afterCursor,
		Limit:       limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SnapshotWorkbenchHandler serializes GET /api/v1/realtime/snapshot/workbench.
func (handler Handler) SnapshotWorkbenchHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Snapshot == nil {
		writeError(w, http.StatusServiceUnavailable, "realtime snapshot service is not configured")
		return
	}
	payload, err := handler.Snapshot.SnapshotWorkbench(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func queryInt64(raw string, fallback int64) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func boundedLimit(raw string, fallback int, maxValue int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maxValue {
		return 0, false
	}
	return value, true
}
