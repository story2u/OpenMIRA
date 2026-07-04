package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGuardRequireRolesAcceptsAllowedRole(t *testing.T) {
	verifier := testVerifier(t)
	guard := Guard{Verifier: verifier}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"name": "客服一",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-guard",
	})

	session, err := guard.RequireRoles(context.Background(), "Bearer "+token, "cs")
	if err != nil {
		t.Fatalf("RequireRoles returned error: %v", err)
	}
	if session.AssigneeID != "cs-001" || session.Role != "cs" {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestGuardRequireRolesMapsLegacyAuthErrors(t *testing.T) {
	verifier := testVerifier(t)
	guard := Guard{Verifier: verifier}

	if _, err := guard.RequireRoles(context.Background(), "", "cs"); !errors.Is(err, ErrMissingBearerToken) {
		t.Fatalf("missing bearer error = %v", err)
	}
	if _, err := guard.RequireRoles(context.Background(), "Bearer invalid", "cs"); !errors.Is(err, ErrInvalidOrExpiredSession) {
		t.Fatalf("invalid session error = %v", err)
	}
}

func TestGuardRequireRolesRejectsForbiddenRole(t *testing.T) {
	verifier := testVerifier(t)
	guard := Guard{Verifier: verifier}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-guard",
	})

	_, err := guard.RequireRoles(context.Background(), "Bearer "+token, "admin", "supervisor")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("RequireRoles error = %v, want %v", err, ErrPermissionDenied)
	}
}

func TestGuardRequireRolesPropagatesBlacklistStoreErrors(t *testing.T) {
	verifier := testVerifier(t)
	storeErr := errors.New("db unavailable")
	verifier.Blacklist = failingBlacklist{err: storeErr}
	guard := Guard{Verifier: verifier}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-guard",
	})

	_, err := guard.RequireRoles(context.Background(), "Bearer "+token, "cs")
	if !errors.Is(err, ErrBlacklistUnavailable) || !errors.Is(err, storeErr) {
		t.Fatalf("RequireRoles error = %v, want blacklist unavailable wrapping store error", err)
	}
}

func TestGuardRequireRolesAllowsAnyRoleWhenNoRolesSpecified(t *testing.T) {
	verifier := testVerifier(t)
	guard := Guard{Verifier: verifier}
	token := signTestToken(t, verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin",
		"role": "admin",
		"exp":  time.Unix(2000, 0).Unix(),
		"jti":  "jwt-admin",
	})

	if _, err := guard.RequireRoles(context.Background(), "Bearer "+token); err != nil {
		t.Fatalf("RequireRoles returned error: %v", err)
	}
}
