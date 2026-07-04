package auth

import (
	"context"
	"errors"
	"strings"
)

var (
	// ErrMissingBearerToken matches legacy protected-route 401 detail.
	ErrMissingBearerToken = errors.New("missing bearer token")
	// ErrInvalidOrExpiredSession matches legacy protected-route 401 detail.
	ErrInvalidOrExpiredSession = errors.New("session invalid or expired")
	// ErrPermissionDenied matches legacy protected-route 403 detail.
	ErrPermissionDenied = errors.New("permission denied")
)

// Guard verifies bearer JWTs and enforces legacy role requirements.
type Guard struct {
	Verifier Verifier
}

// RequireRoles validates Authorization and ensures the session role is allowed.
func (guard Guard) RequireRoles(ctx context.Context, authorization string, roles ...string) (Session, error) {
	token := ParseBearerToken(authorization)
	if token == "" {
		return Session{}, ErrMissingBearerToken
	}
	session, err := guard.Verifier.VerifyContext(ctx, token)
	if err != nil {
		if errors.Is(err, ErrBlacklistUnavailable) {
			return Session{}, err
		}
		return Session{}, ErrInvalidOrExpiredSession
	}
	if len(cleanRoles(roles)) > 0 && !session.HasRole(roles...) {
		return Session{}, ErrPermissionDenied
	}
	return session, nil
}

func cleanRoles(roles []string) []string {
	cleaned := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role != "" {
			cleaned = append(cleaned, role)
		}
	}
	return cleaned
}
