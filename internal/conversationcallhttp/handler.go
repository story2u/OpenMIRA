// Package conversationcallhttp serializes conversation call candidate routes.
package conversationcallhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/conversationcall"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
)

type Service interface {
	Availability(ctx context.Context, conversationID string, request conversationcall.Request, operator string) (map[string]any, error)
	ReleaseReservation(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error)
	Call(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error)
	Hangup(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error)
}

type Handler struct {
	Guard   auth.Guard
	Service Service
}

func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

func (handler Handler) CallHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, "call")
}

func (handler Handler) HangupHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, "hangup")
}

func (handler Handler) AvailabilityHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, "availability")
}

func (handler Handler) ReservationReleaseHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, "release")
}

func (handler Handler) handle(w http.ResponseWriter, r *http.Request, action string) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "conversation call service is not configured")
		return
	}
	var request conversationcall.Request
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "body must be a JSON object")
		return
	}
	request.Operator = session.AssigneeID
	conversationID := r.PathValue("conversation_id")
	var payload map[string]any
	switch action {
	case "availability":
		payload, err = handler.Service.Availability(r.Context(), conversationID, request, session.AssigneeID)
	case "release":
		payload, err = handler.Service.ReleaseReservation(r.Context(), conversationID, request)
	case "call":
		payload, err = handler.Service.Call(r.Context(), conversationID, request)
	default:
		payload, err = handler.Service.Hangup(r.Context(), conversationID, request)
	}
	if err != nil {
		switch {
		case errors.Is(err, conversationcall.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, conversationcall.ErrConversationNotFound):
			writeError(w, http.StatusNotFound, "conversation not found")
		case errors.Is(err, conversationcall.ErrTargetNotReady):
			writeError(w, http.StatusConflict, "contact identity is not ready; please retry later")
		case errors.As(err, new(sendtarget.ContactIdentityError)):
			writeError(w, http.StatusConflict, err.Error())
		case errors.As(err, new(sendguard.DeviceOfflineError)):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, conversationcall.ErrTaskServiceMissing):
			writeError(w, http.StatusServiceUnavailable, "conversation call task service is not configured")
		case errors.Is(err, conversationcall.ErrLockStoreMissing):
			writeError(w, http.StatusServiceUnavailable, "conversation call lock store is not configured")
		case errors.Is(err, conversationcall.ErrCallSlotBusy):
			writeError(w, http.StatusConflict, "该账号正在通话中，请稍后再试或先挂断当前通话。")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
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
