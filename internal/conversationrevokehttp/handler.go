// Package conversationrevokehttp serializes the legacy message revoke route.
package conversationrevokehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/conversationrevoke"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
)

type Service interface {
	Revoke(ctx context.Context, conversationID string, traceID string, request conversationrevoke.Request) (conversationrevoke.Response, error)
}

type Handler struct {
	Guard   auth.Guard
	Service Service
}

func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

func (handler Handler) RevokeHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "conversation message revoke service is not configured")
		return
	}
	var request conversationrevoke.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "body must be a JSON object")
		return
	}
	request.Operator = session.AssigneeID
	response, err := handler.Service.Revoke(r.Context(), r.PathValue("conversation_id"), r.PathValue("trace_id"), request)
	if err != nil {
		switch {
		case errors.Is(err, conversationrevoke.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, conversationrevoke.Detail(err))
		case errors.Is(err, conversationrevoke.ErrMessageNotFound):
			writeError(w, http.StatusNotFound, conversationrevoke.Detail(err))
		case errors.Is(err, conversationrevoke.ErrConflict):
			writeError(w, http.StatusConflict, conversationrevoke.Detail(err))
		case isContactIdentityError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isDeviceOfflineError(err):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, conversationrevoke.ErrTaskServiceMissing), errors.Is(err, conversationrevoke.ErrMessageStoreMissing):
			writeError(w, http.StatusServiceUnavailable, conversationrevoke.Detail(err))
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func isDeviceOfflineError(err error) bool {
	var offline sendguard.DeviceOfflineError
	return errors.As(err, &offline)
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
