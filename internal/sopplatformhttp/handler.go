// Package sopplatformhttp adapts SOP platform test routes to net/http.
package sopplatformhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/sopplatform"
)

// Service validates SOP platform task URLs.
type Service interface {
	TestConnection(ctx context.Context, request sopplatform.Request) (sopplatform.Result, error)
}

// Handler owns SOP platform route serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds an SOP platform HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// TestHandler serializes POST /api/v1/admin/sop/platform/test.
func (handler Handler) TestHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "sop platform test service is not configured")
		return
	}
	var request sopplatform.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	result, err := handler.Service.TestConnection(r.Context(), request)
	if err != nil {
		if errors.Is(err, sopplatform.ErrTaskURLRequired) {
			writeError(w, http.StatusUnprocessableEntity, "task_url is required")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, 1<<20+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return io.ErrUnexpectedEOF
	}
	if len(data) > 1<<20 {
		return io.ErrShortBuffer
	}
	return json.Unmarshal(data, value)
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
