// Package wsgateway implements the candidate /ws/{channel} gateway.
package wsgateway

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrAuthenticationRequired mirrors the legacy websocket close reason.
	ErrAuthenticationRequired = errors.New("authentication required")
)

// Authenticator verifies websocket query-token credentials.
type Authenticator struct {
	SessionVerifier *auth.Verifier
	AgentToken      string
	AllowLegacy     bool
}

// Authenticate accepts either a valid session token, a matching agent token,
// or the explicit legacy bypass used by old local deployments.
func (authenticator Authenticator) Authenticate(ctx context.Context, token string, agentToken string) error {
	token = strings.TrimSpace(token)
	if token != "" && authenticator.SessionVerifier != nil {
		if _, err := authenticator.SessionVerifier.VerifyContext(ctx, token); err == nil {
			return nil
		}
	}
	if constantTimeEqual(agentToken, authenticator.AgentToken) {
		return nil
	}
	if authenticator.AllowLegacy {
		return nil
	}
	return ErrAuthenticationRequired
}

func constantTimeEqual(actual string, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
