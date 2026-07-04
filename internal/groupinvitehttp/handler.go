// Package groupinvitehttp serializes the legacy /group/invite route.
package groupinvitehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/groupinvite"
	"wework-go/internal/sendguard"
)

// Service creates group invite tasks.
type Service interface {
	Invite(ctx context.Context, request groupinvite.Request) (map[string]any, error)
}

// Handler owns /group/invite serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a group invite HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// InviteHandler serializes POST /group/invite.
func (handler Handler) InviteHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "group invite service is not configured")
		return
	}
	var request groupinvite.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "body must be a JSON object")
		return
	}
	request.Operator = session.AssigneeID
	payload, err := handler.Service.Invite(r.Context(), request)
	if err != nil {
		switch {
		case isDeviceOfflineError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isRateLimitError(err):
			writeError(w, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, groupinvite.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, groupinvite.ErrTaskServiceMissing):
			writeError(w, http.StatusServiceUnavailable, "group invite task service is not configured")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func isDeviceOfflineError(err error) bool {
	var offline sendguard.DeviceOfflineError
	return errors.As(err, &offline)
}

func isRateLimitError(err error) bool {
	var rateLimit sendguard.RateLimitError
	return errors.As(err, &rateLimit)
}

func decodeJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return io.ErrUnexpectedEOF
	}
	return json.Unmarshal(data, value)
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, auth.ErrBlacklistUnavailable):
		writeError(w, http.StatusServiceUnavailable, "session token blacklist is unavailable")
	default:
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
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
