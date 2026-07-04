// Package friendaddedhttp adapts the manual friend-added event endpoint.
package friendaddedhttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/friendadded"
)

// Service is the friend-added behavior required by the HTTP adapter.
type Service interface {
	Ingest(ctx context.Context, request friendadded.Request) (friendadded.Response, error)
}

// Handler owns POST /api/v1/events/friend-added.
type Handler struct {
	Guard                auth.Guard
	Service              Service
	AgentToken           string
	AllowLegacyAgentAuth bool
}

// New builds a friend-added event HTTP adapter.
func New(guard auth.Guard, service Service, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{
		Guard:                guard,
		Service:              service,
		AgentToken:           strings.TrimSpace(agentToken),
		AllowLegacyAgentAuth: allowLegacyAgentAuth,
	}
}

// EventHandler serializes POST /api/v1/events/friend-added.
func (handler Handler) EventHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentOrSession(w, r) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, friendadded.ErrStoreUnavailable.Error())
		return
	}
	request, err := friendadded.DecodeRequestJSON(readBody(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response, err := handler.Service.Ingest(r.Context(), request)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, response)
	case errors.Is(err, friendadded.ErrStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (handler Handler) requireAgentOrSession(w http.ResponseWriter, r *http.Request) bool {
	authorization := r.Header.Get("Authorization")
	if auth.ParseBearerToken(authorization) != "" {
		if _, err := handler.Guard.RequireRoles(r.Context(), authorization); err != nil {
			writeAuthError(w, err)
			return false
		}
		return true
	}
	expectedAgentToken := strings.TrimSpace(handler.AgentToken)
	actualAgentToken := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	if expectedAgentToken != "" && actualAgentToken != "" && subtle.ConstantTimeCompare([]byte(actualAgentToken), []byte(expectedAgentToken)) == 1 {
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
	case errors.Is(err, auth.ErrBlacklistUnavailable):
		writeError(w, http.StatusServiceUnavailable, "session token blacklist is unavailable")
	default:
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	}
}

func readBody(r *http.Request) []byte {
	defer r.Body.Close()
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return payload
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
