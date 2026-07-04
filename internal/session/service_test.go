package session

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestAdminLoginIssuesLegacyAdminToken(t *testing.T) {
	service := testService(t)
	service.AdminCredentials = AdminCredentials{Username: "root", Password: "secret"}

	response, err := service.AdminLogin(context.Background(), " root ", "secret")
	if err != nil {
		t.Fatalf("AdminLogin returned error: %v", err)
	}
	if !response.Success || response.Token == "" || response.AssigneeID != "admin" || response.AssigneeName != "管理员" || response.Role != "admin" {
		t.Fatalf("unexpected admin login response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-08T00:16:40+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
	verified, err := service.Verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify admin token returned error: %v", err)
	}
	if verified.AssigneeID != "admin" || verified.AssigneeName != "管理员" || verified.Role != "admin" || verified.JTI == "" {
		t.Fatalf("unexpected admin token claims: %+v", verified)
	}
}

func TestAdminLoginMapsLegacyFailures(t *testing.T) {
	service := testService(t)
	if _, err := service.AdminLogin(context.Background(), "admin", "secret"); !errors.Is(err, ErrAdminLoginNotConfigured) {
		t.Fatalf("missing config error = %v", err)
	}

	service.AdminCredentials = AdminCredentials{Username: "admin", Password: "secret"}
	if _, err := service.AdminLogin(context.Background(), "", "secret"); !errors.Is(err, ErrAdminLoginMissingCredentials) {
		t.Fatalf("missing username error = %v", err)
	}
	if _, err := service.AdminLogin(context.Background(), "admin", "bad"); !errors.Is(err, ErrAdminLoginInvalidCredentials) {
		t.Fatalf("bad credentials error = %v", err)
	}
}

func TestAdminLoginAuditsSuccessAndRecordsAttempts(t *testing.T) {
	service := testService(t)
	service.AdminCredentials = AdminCredentials{Username: "root", Password: "secret"}
	audit := &auditRecorder{}
	service.AuditLogs = audit
	service.LoginLimiter = NewLoginRateLimiter(LoginRateLimiterOptions{
		Window:      5 * time.Minute,
		MaxAttempts: 1,
		Burst:       10,
		BurstWindow: time.Minute,
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})

	_, err := service.AdminLogin(context.Background(), "root", "secret", LoginMetadata{ClientIP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("AdminLogin returned error: %v", err)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin" || audit.entries[0].ActionType != "login" || audit.entries[0].Detail != "管理员账密登录" || audit.entries[0].IP != "127.0.0.1" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if _, err := service.AdminLogin(context.Background(), "root", "secret", LoginMetadata{ClientIP: "127.0.0.1"}); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("rate limit error = %v", err)
	}
}

func TestAdminLoginRecordsFailedAttemptsWithoutAudit(t *testing.T) {
	service := testService(t)
	service.AdminCredentials = AdminCredentials{Username: "root", Password: "secret"}
	audit := &auditRecorder{}
	service.AuditLogs = audit
	service.LoginLimiter = NewLoginRateLimiter(LoginRateLimiterOptions{
		Window:      5 * time.Minute,
		MaxAttempts: 1,
		Burst:       10,
		BurstWindow: time.Minute,
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})

	if _, err := service.AdminLogin(context.Background(), "", "secret", LoginMetadata{ClientIP: "127.0.0.2"}); !errors.Is(err, ErrAdminLoginMissingCredentials) {
		t.Fatalf("missing credentials error = %v", err)
	}
	if len(audit.entries) != 0 {
		t.Fatalf("audit entries = %+v, want none", audit.entries)
	}
	if _, err := service.AdminLogin(context.Background(), "root", "secret", LoginMetadata{ClientIP: "127.0.0.2"}); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("rate limit error = %v", err)
	}
}

func TestAssigneeLoginIssuesLegacyToken(t *testing.T) {
	service := testService(t)
	service.PasswordlessLogin = true
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "supervisor",
		Enabled:      true,
	}}

	response, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{
		AssigneeID:   " cs-001 ",
		AssigneeName: "客服一号",
		TTLHours:     2,
	})
	if err != nil {
		t.Fatalf("AssigneeLogin returned error: %v", err)
	}
	if !response.Success || response.Token == "" || response.AssigneeID != "cs-001" || response.AssigneeName != "客服一号" || response.Role != "supervisor" {
		t.Fatalf("unexpected assignee login response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-01T02:16:40+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
	verified, err := service.Verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify assignee token returned error: %v", err)
	}
	if verified.AssigneeID != "cs-001" || verified.AssigneeName != "客服一号" || verified.Role != "supervisor" {
		t.Fatalf("unexpected assignee token claims: %+v", verified)
	}
}

func TestAssigneeLoginWritesLegacyAudit(t *testing.T) {
	service := testService(t)
	service.PasswordlessLogin = true
	service.AuditLogs = &auditRecorder{}
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		Enabled:      true,
	}}

	_, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{AssigneeID: "cs-001"}, LoginMetadata{ClientIP: "10.0.0.1"})
	if err != nil {
		t.Fatalf("AssigneeLogin returned error: %v", err)
	}
	audit := service.AuditLogs.(*auditRecorder)
	if len(audit.entries) != 1 || audit.entries[0].Operator != "cs-001" || audit.entries[0].Detail != "用户 客服一(cs-001) 登录" || audit.entries[0].IP != "10.0.0.1" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestAssigneeLoginMapsLegacyFailures(t *testing.T) {
	service := testService(t)
	if _, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{AssigneeID: "cs-001"}); !errors.Is(err, ErrPasswordlessLoginDisabled) {
		t.Fatalf("disabled error = %v", err)
	}

	service.PasswordlessLogin = true
	if _, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{}); !errors.Is(err, ErrAssigneeIDRequired) {
		t.Fatalf("missing assignee error = %v", err)
	}
	if _, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{AssigneeID: "cs-001"}); !errors.Is(err, ErrUserResolverUnavailable) {
		t.Fatalf("missing resolver error = %v", err)
	}

	service.Users = userMap{"cs-001": {AssigneeID: "cs-001", Enabled: false}}
	if _, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{AssigneeID: "cs-001"}); !errors.Is(err, ErrAssigneeUserNotFoundOrDisabled) {
		t.Fatalf("disabled user error = %v", err)
	}
	if _, err := service.AssigneeLogin(context.Background(), AssigneeLoginRequest{AssigneeID: "missing"}); !errors.Is(err, ErrAssigneeUserNotFoundOrDisabled) {
		t.Fatalf("missing user error = %v", err)
	}
}

func TestCSLoginVerifiesPasswordAndUpdatesLastSeen(t *testing.T) {
	service := testService(t)
	lastSeen := &lastSeenRecorder{}
	service.LastSeen = lastSeen
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		Enabled:      true,
		PasswordHash: passwordHash("secret"),
	}}

	response, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: " cs-001 ", Password: "secret"})
	if err != nil {
		t.Fatalf("CSLogin returned error: %v", err)
	}
	if !response.Success || response.Token == "" || response.AssigneeID != "cs-001" || response.AssigneeName != "客服一" || response.Role != "cs" {
		t.Fatalf("unexpected cs login response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-08T00:16:40+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
	if strings.Join(lastSeen.ids, ",") != "cs-001" {
		t.Fatalf("last seen updates = %v", lastSeen.ids)
	}
	verified, err := service.Verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify cs token returned error: %v", err)
	}
	if verified.AssigneeID != "cs-001" || verified.AssigneeName != "客服一" || verified.Role != "cs" {
		t.Fatalf("unexpected cs token claims: %+v", verified)
	}
}

func TestCSLoginWritesLegacyAudit(t *testing.T) {
	service := testService(t)
	service.AuditLogs = &auditRecorder{}
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		Enabled:      true,
		PasswordHash: passwordHash("secret"),
	}}

	_, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: "cs-001", Password: "secret"}, LoginMetadata{ClientIP: "10.0.0.2"})
	if err != nil {
		t.Fatalf("CSLogin returned error: %v", err)
	}
	audit := service.AuditLogs.(*auditRecorder)
	if len(audit.entries) != 1 || audit.entries[0].Operator != "cs-001" || audit.entries[0].Detail != "客服 客服一(cs-001) 密码登录" || audit.entries[0].IP != "10.0.0.2" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestCSLoginMapsLegacyFailures(t *testing.T) {
	service := testService(t)
	if _, err := service.CSLogin(context.Background(), CSLoginRequest{}); !errors.Is(err, ErrCSLoginMissingCredentials) {
		t.Fatalf("missing credentials error = %v", err)
	}
	if _, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: "cs-001", Password: "secret"}); !errors.Is(err, ErrUserResolverUnavailable) {
		t.Fatalf("missing resolver error = %v", err)
	}

	service.Users = userMap{"cs-001": {AssigneeID: "cs-001", Enabled: false, PasswordHash: passwordHash("secret")}}
	if _, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: "cs-001", Password: "secret"}); !errors.Is(err, ErrCSLoginUserNotFoundOrDisabled) {
		t.Fatalf("disabled user error = %v", err)
	}
	if _, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: "missing", Password: "secret"}); !errors.Is(err, ErrCSLoginUserNotFoundOrDisabled) {
		t.Fatalf("missing user error = %v", err)
	}

	service.Users = userMap{"cs-001": {AssigneeID: "cs-001", Enabled: true, PasswordHash: passwordHash("secret")}}
	if _, err := service.CSLogin(context.Background(), CSLoginRequest{AssigneeID: "cs-001", Password: "bad"}); !errors.Is(err, ErrCSLoginInvalidCredentials) {
		t.Fatalf("bad password error = %v", err)
	}
}

func TestGenerateCSTokenIssuesShortLivedWorkspaceToken(t *testing.T) {
	service := testService(t)
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		Enabled:      true,
	}}
	adminToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin",
		"name": "管理员",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	response, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, " cs-001 ")
	if err != nil {
		t.Fatalf("GenerateCSToken returned error: %v", err)
	}
	if !response.Success || response.Token == "" || response.AssigneeID != "cs-001" || response.AssigneeName != "客服一" {
		t.Fatalf("unexpected generate token response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-02T00:16:40+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
	verified, err := service.Verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify generated cs token returned error: %v", err)
	}
	if verified.AssigneeID != "cs-001" || verified.AssigneeName != "客服一" || verified.Role != "cs" {
		t.Fatalf("unexpected generated token claims: %+v", verified)
	}
}

func TestGenerateCSTokenWritesLegacyImpersonationAudit(t *testing.T) {
	service := testService(t)
	service.AuditLogs = &auditRecorder{}
	service.Users = userMap{"cs-001": {
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		Enabled:      true,
	}}
	adminToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-1",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	_, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, "cs-001", LoginMetadata{ClientIP: "10.0.0.3"})
	if err != nil {
		t.Fatalf("GenerateCSToken returned error: %v", err)
	}
	audit := service.AuditLogs.(*auditRecorder)
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "impersonate" || audit.entries[0].Detail != "管理员 admin-1 为客服 cs-001(客服一) 生成工作台 Token" || audit.entries[0].IP != "10.0.0.3" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestGenerateCSTokenMapsLegacyFailures(t *testing.T) {
	service := testService(t)
	adminToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})
	csToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-cs",
	})

	if _, err := service.GenerateCSToken(context.Background(), "", "cs-001"); !errors.Is(err, ErrMissingBearerToken) {
		t.Fatalf("missing bearer error = %v", err)
	}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer invalid", "cs-001"); !errors.Is(err, ErrInvalidOrExpiredSession) {
		t.Fatalf("invalid token error = %v", err)
	}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer "+csToken, "cs-001"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("permission error = %v", err)
	}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, ""); !errors.Is(err, ErrAssigneeIDRequired) {
		t.Fatalf("missing assignee error = %v", err)
	}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, "cs-001"); !errors.Is(err, ErrUserResolverUnavailable) {
		t.Fatalf("missing resolver error = %v", err)
	}

	service.Users = userMap{"cs-001": {AssigneeID: "cs-001", Enabled: false}}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, "cs-001"); !errors.Is(err, ErrCSUserNotFoundOrDisabled) {
		t.Fatalf("disabled user error = %v", err)
	}
	if _, err := service.GenerateCSToken(context.Background(), "Bearer "+adminToken, "missing"); !errors.Is(err, ErrCSUserNotFoundOrDisabled) {
		t.Fatalf("missing user error = %v", err)
	}
}

func TestCurrentUserReturnsLegacyMeResponse(t *testing.T) {
	service := testService(t)
	service.Profiles = profileMap{"cs-001": {AIEnabled: true}}
	service.LastSeen = &lastSeenRecorder{}
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"name": "客服一",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-test",
	})

	response, err := service.CurrentUser(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("CurrentUser returned error: %v", err)
	}
	if response.AssigneeID != "cs-001" || response.AssigneeName != "客服一" || response.Role != "cs" || !response.AIEnabled {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-01T00:33:20+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
}

func TestCurrentUserMapsAuthFailuresToLegacyErrors(t *testing.T) {
	service := testService(t)
	if _, err := service.CurrentUser(context.Background(), ""); !errors.Is(err, ErrMissingBearerToken) {
		t.Fatalf("missing bearer error = %v", err)
	}
	if _, err := service.CurrentUser(context.Background(), "Bearer invalid"); !errors.Is(err, ErrInvalidOrExpiredSession) {
		t.Fatalf("invalid token error = %v", err)
	}
}

func TestCurrentUserPropagatesBlacklistStoreErrors(t *testing.T) {
	service := testService(t)
	service.Verifier.Blacklist = failingBlacklist{err: errors.New("db unavailable")}
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-test",
	})

	_, err := service.CurrentUser(context.Background(), "Bearer "+token)
	if !errors.Is(err, auth.ErrBlacklistUnavailable) {
		t.Fatalf("CurrentUser error = %v, want %v", err, auth.ErrBlacklistUnavailable)
	}
}

func TestCurrentUserThrottlesLastSeenAndSkipsAdmin(t *testing.T) {
	service := testService(t)
	recorder := &lastSeenRecorder{}
	service.LastSeen = recorder
	userToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-user",
	})
	adminToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "admin",
		"exp": int64(2000),
		"jti": "jwt-admin",
	})

	_, _ = service.CurrentUser(context.Background(), "Bearer "+userToken)
	_, _ = service.CurrentUser(context.Background(), "Bearer "+userToken)
	_, _ = service.CurrentUser(context.Background(), "Bearer "+adminToken)

	if got := strings.Join(recorder.ids, ","); got != "cs-001" {
		t.Fatalf("last seen updates = %q, want cs-001", got)
	}
}

func TestRefreshRevokesOldTokenAndReturnsLegacyResponse(t *testing.T) {
	service := testService(t)
	service.Profiles = profileMap{"cs-001": {AIEnabled: true}}
	revoker := &revokerRecorder{}
	service.Revoker = revoker
	oldToken := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"name": "客服一",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-old",
	})

	response, err := service.Refresh(context.Background(), "Bearer "+oldToken)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if !response.Success || response.Token == "" || response.AssigneeID != "cs-001" || response.AssigneeName != "客服一" || !response.AIEnabled {
		t.Fatalf("unexpected refresh response: %+v", response)
	}
	if response.ExpiresAt != "1970-01-08T00:16:40+00:00" {
		t.Fatalf("expires_at = %q", response.ExpiresAt)
	}
	if revoker.jti != "jwt-old" || !revoker.expiresAt.Equal(time.Unix(2000, 0).UTC()) {
		t.Fatalf("revoked token = %q expires=%s", revoker.jti, revoker.expiresAt)
	}
	refreshed, err := service.Verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify refreshed token returned error: %v", err)
	}
	if refreshed.JTI == "jwt-old" || refreshed.AssigneeID != "cs-001" || refreshed.Role != "cs" {
		t.Fatalf("unexpected refreshed claims: %+v", refreshed)
	}
}

func TestRefreshMapsAuthFailuresToLegacyErrors(t *testing.T) {
	service := testService(t)
	service.Revoker = &revokerRecorder{}

	if _, err := service.Refresh(context.Background(), ""); !errors.Is(err, ErrMissingBearerToken) {
		t.Fatalf("missing bearer error = %v", err)
	}
	if _, err := service.Refresh(context.Background(), "Bearer invalid"); !errors.Is(err, ErrInvalidOrExpiredSession) {
		t.Fatalf("invalid token error = %v", err)
	}
}

func TestRefreshRequiresRevokerAndPropagatesStoreErrors(t *testing.T) {
	service := testService(t)
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-old",
	})

	if _, err := service.Refresh(context.Background(), "Bearer "+token); !errors.Is(err, ErrRevokerUnavailable) {
		t.Fatalf("missing revoker error = %v", err)
	}

	service.Revoker = failingRevoker{err: errors.New("db down")}
	if _, err := service.Refresh(context.Background(), "Bearer "+token); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("revoker error = %v, want db down", err)
	}
}

func TestLogoutRevokesSignedTokenAndReturnsLegacyResponse(t *testing.T) {
	service := testService(t)
	revoker := &revokerRecorder{}
	service.Revoker = revoker
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "other-issuer",
		"sub": "cs-001",
		"exp": int64(999),
		"jti": "jwt-logout",
	})

	response, err := service.Logout(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if !response.Success {
		t.Fatalf("success = false, want true")
	}
	if revoker.jti != "jwt-logout" || revoker.expiresAt.Unix() != 999 {
		t.Fatalf("revoked token = %q expires=%s", revoker.jti, revoker.expiresAt)
	}
}

func TestLogoutWritesLegacyAudit(t *testing.T) {
	service := testService(t)
	service.Revoker = &revokerRecorder{}
	service.AuditLogs = &auditRecorder{}
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"name": "客服一",
		"exp":  int64(2000),
		"jti":  "jwt-logout",
	})

	_, err := service.Logout(context.Background(), "Bearer "+token, LoginMetadata{ClientIP: "10.0.0.4"})
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	audit := service.AuditLogs.(*auditRecorder)
	if len(audit.entries) != 1 || audit.entries[0].Operator != "cs-001" || audit.entries[0].ActionType != "logout" || audit.entries[0].Detail != "用户 客服一 登出" || audit.entries[0].IP != "10.0.0.4" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestLogoutReturnsFalseForInvalidSignedPayload(t *testing.T) {
	service := testService(t)
	service.Revoker = &revokerRecorder{}

	response, err := service.Logout(context.Background(), "Bearer invalid")

	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if response.Success {
		t.Fatalf("success = true, want false")
	}
}

func TestLogoutMapsMissingBearerAndRevokerErrors(t *testing.T) {
	service := testService(t)
	if _, err := service.Logout(context.Background(), ""); !errors.Is(err, ErrMissingBearerToken) {
		t.Fatalf("missing bearer error = %v", err)
	}
	token := signSessionToken(t, service.Verifier.Secret, map[string]any{
		"iss": "wework-cloud",
		"sub": "cs-001",
		"exp": int64(2000),
		"jti": "jwt-logout",
	})
	if _, err := service.Logout(context.Background(), "Bearer "+token); !errors.Is(err, ErrRevokerUnavailable) {
		t.Fatalf("missing revoker error = %v", err)
	}
	service.Revoker = failingRevoker{err: errors.New("db down")}
	if _, err := service.Logout(context.Background(), "Bearer "+token); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("revoker error = %v, want db down", err)
	}
}

type profileMap map[string]Profile

func (profiles profileMap) GetProfile(ctx context.Context, assigneeID string) (Profile, bool, error) {
	profile, ok := profiles[assigneeID]
	return profile, ok, nil
}

type userMap map[string]User

func (users userMap) GetUser(ctx context.Context, assigneeID string) (User, bool, error) {
	user, ok := users[assigneeID]
	return user, ok, nil
}

type lastSeenRecorder struct {
	ids []string
}

func (recorder *lastSeenRecorder) UpdateLastSeen(ctx context.Context, assigneeID string) error {
	recorder.ids = append(recorder.ids, assigneeID)
	return nil
}

type auditRecorder struct {
	entries []AuditLogEntry
}

func (recorder *auditRecorder) AddAuditLog(ctx context.Context, entry AuditLogEntry) error {
	recorder.entries = append(recorder.entries, entry)
	return nil
}

type revokerRecorder struct {
	jti       string
	expiresAt time.Time
}

func (recorder *revokerRecorder) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	recorder.jti = jti
	recorder.expiresAt = expiresAt
	return nil
}

type failingRevoker struct {
	err error
}

func (revoker failingRevoker) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	return revoker.err
}

type failingBlacklist struct {
	err error
}

func (blacklist failingBlacklist) Contains(ctx context.Context, jti string) (bool, error) {
	return false, blacklist.err
}

func testService(t *testing.T) *Service {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time {
		return time.Unix(1000, 0).UTC()
	}
	return &Service{
		Verifier:         verifier,
		LastSeenThrottle: 60 * time.Second,
		now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	}
}

func signSessionToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeSessionTokenPart(t, header)
	encodedClaims := encodeSessionTokenPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeSessionTokenPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func passwordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}
