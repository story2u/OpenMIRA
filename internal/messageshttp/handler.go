// Package messageshttp adapts conversation message services to HTTP.
// The adapter is opt-in during phase three and only owns auth, parameter
// normalization, and legacy JSON error shape.
package messageshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/messages"
)

// Service builds the legacy conversation message page payload.
type Service interface {
	List(ctx context.Context, request messages.Request) (messages.Payload, error)
}

// Handler contains conversation message HTTP adapters.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a conversation messages HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// ListHandler serializes /api/v1/conversations/{conversation_id}/messages.
func (handler Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "conversation messages service is not configured")
		return
	}
	request := messages.NewRequest(r.PathValue("conversation_id"), r.URL.Query(), session)
	if request.ConversationID == "" {
		writeError(w, http.StatusBadRequest, "missing conversation_id")
		return
	}
	payload, err := handler.Service.List(r.Context(), request)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
