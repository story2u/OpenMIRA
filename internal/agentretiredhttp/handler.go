// Package agentretiredhttp adapts retired legacy App/HTTP-Agent endpoints.
package agentretiredhttp

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"im-go/internal/auth"
)

const (
	heartbeatDisabledDetail  = "legacy App/HTTP-Agent heartbeat is disabled; device status is sourced from the device SDK/provider"
	loginEventDisabledDetail = "legacy App/HTTP-Agent login callback is disabled; use device SDK/provider login status"
)

// Handler owns retired legacy agent route serialization.
type Handler struct {
	Verifier             *auth.Verifier
	AgentToken           string
	AllowLegacyAgentAuth bool
}

// New builds a retired legacy agent HTTP adapter.
func New(verifier *auth.Verifier, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{
		Verifier:             verifier,
		AgentToken:           strings.TrimSpace(agentToken),
		AllowLegacyAgentAuth: allowLegacyAgentAuth,
	}
}

// HeartbeatHandler serializes POST /api/v1/agents/heartbeat.
func (handler Handler) HeartbeatHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, heartbeatDisabledDetail)
}

// LoginEventHandler serializes POST /agents/wework/login/event.
func (handler Handler) LoginEventHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireOptionalAgentAuth(w, r) {
		return
	}
	writeError(w, http.StatusGone, loginEventDisabledDetail)
}

func (handler Handler) requireOptionalAgentAuth(w http.ResponseWriter, r *http.Request) bool {
	token := auth.ParseBearerToken(r.Header.Get("Authorization"))
	if token != "" && handler.Verifier != nil {
		if _, err := handler.Verifier.VerifyContext(r.Context(), token); err == nil {
			return true
		}
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

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
