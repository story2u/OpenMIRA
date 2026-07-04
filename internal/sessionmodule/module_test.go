// Package sessionmodule tests keep assembly behavior deterministic without a
// database driver. They verify wiring and route-readiness checks while the
// actual /api/v1/session/me route is still owned by Python.
package sessionmodule

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/config"
)

// TestNewRejectsMissingSessionSecret keeps SESSION_JWT_SECRET fail-fast.
func TestNewRejectsMissingSessionSecret(t *testing.T) {
	_, err := New(Options{Config: config.Config{SessionJWTIssuer: "wework-cloud"}})
	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("New error = %v, want %v", err, auth.ErrMissingSecret)
	}
}

// TestNewRequiresProfileStoreWhenRequested protects route-ready assembly.
func TestNewRequiresProfileStoreWhenRequested(t *testing.T) {
	_, err := New(Options{
		Config:              config.Config{SessionJWTSecret: "session-secret"},
		RequireProfileStore: true,
	})
	if !errors.Is(err, ErrProfileStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrProfileStoreRequired)
	}
}

// TestNewRequiresBlacklistStoreWhenRequested protects revocation checks.
func TestNewRequiresBlacklistStoreWhenRequested(t *testing.T) {
	_, err := New(Options{
		Config:                config.Config{SessionJWTSecret: "session-secret"},
		RequireBlacklistStore: true,
	})
	if !errors.Is(err, ErrBlacklistStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrBlacklistStoreRequired)
	}
}

// TestNewBuildsUnmountedSessionHandler verifies service and handler wiring.
func TestNewBuildsUnmountedSessionHandler(t *testing.T) {
	module, err := New(Options{
		Config: config.Config{
			SessionJWTSecret: "session-secret",
			SessionJWTIssuer: "wework-cloud",
		},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Service == nil || module.ProfileRepository != nil {
		t.Fatalf("unexpected module state: service=%v profile=%v", module.Service, module.ProfileRepository)
	}

	token := signModuleToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"name": "客服一",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-test",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	module.Handler.Me(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"assignee_id":"cs-001"`, `"role":"cs"`, `"ai_enabled":false`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

// TestNewWiresAdminLoginCredentials verifies env-backed admin auth assembly.
func TestNewWiresAdminLoginCredentials(t *testing.T) {
	module, err := New(Options{
		Config: config.Config{
			SessionJWTSecret: "session-secret",
			AdminUsername:    "admin",
			AdminPassword:    "secret",
		},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin-login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	response := httptest.NewRecorder()
	module.Handler.AdminLogin(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"success":true`, `"assignee_id":"admin"`, `"role":"admin"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, response.Body.String())
		}
	}
}

// TestNewWiresDisabledPasswordlessLogin verifies login can fail before DB use.
func TestNewWiresDisabledPasswordlessLogin(t *testing.T) {
	module, err := New(Options{Config: config.Config{SessionJWTSecret: "session-secret"}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/login", strings.NewReader(`{"assignee_id":"cs-001"}`))
	response := httptest.NewRecorder()
	module.Handler.Login(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "passwordless login disabled") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

// TestNewWiresBlacklist ensures revoked tokens are rejected by the module.
func TestNewWiresBlacklist(t *testing.T) {
	module, err := New(Options{
		Config: config.Config{SessionJWTSecret: "session-secret"},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
		Blacklist: moduleBlacklist{"jwt-revoked": true},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Service.Revoker == nil {
		t.Fatal("service revoker is nil, want blacklist-backed revoker")
	}
	token := signModuleToken(t, "session-secret", map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-revoked",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	module.Handler.Me(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "session invalid or expired") {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

type moduleBlacklist map[string]bool

// Contains implements auth.Blacklist for module wiring tests.
func (blacklist moduleBlacklist) Contains(ctx context.Context, jti string) (bool, error) {
	return blacklist[jti], nil
}

// Add implements auth.Revoker for module wiring tests.
func (blacklist moduleBlacklist) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	blacklist[jti] = true
	return nil
}

// signModuleToken creates deterministic HS256 tokens for handler tests.
func signModuleToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeModulePart(t, header)
	encodedClaims := encodeModulePart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

// encodeModulePart serializes one JWT segment for local test tokens.
func encodeModulePart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
