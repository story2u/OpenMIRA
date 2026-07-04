// Package sendtexthttp serializes the legacy /send/text route.
package sendtexthttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/sendtext"
)

// Service creates send text tasks.
type Service interface {
	Send(ctx context.Context, request sendtext.Request) (map[string]any, error)
}

// Handler owns /send/text serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a send text HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// SendHandler serializes POST /send/text.
func (handler Handler) SendHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "send text service is not configured")
		return
	}
	var request sendtext.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "body must be a JSON object")
		return
	}
	request.Operator = session.AssigneeID
	payload, err := handler.Service.Send(r.Context(), request)
	if err != nil {
		switch {
		case isDeviceOfflineError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isContactIdentityError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isRateLimitError(err):
			writeError(w, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, sendtext.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, sendtext.ErrTaskServiceMissing):
			writeError(w, http.StatusServiceUnavailable, "send text task service is not configured")
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

func isContactIdentityError(err error) bool {
	var contactIdentity sendtarget.ContactIdentityError
	return errors.As(err, &contactIdentity)
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
