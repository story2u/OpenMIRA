// Package session prepares the Go implementation of legacy session endpoints.
// It keeps HTTP routing and database repositories out of the package so phase
// two can verify JWT/session behavior before any route takes traffic.
package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"im-go/internal/auth"
)

var (
	// ErrMissingBearerToken matches the legacy /me 401 detail.
	ErrMissingBearerToken = errors.New("missing bearer token")
	// ErrInvalidOrExpiredSession matches the legacy /me 401 detail.
	ErrInvalidOrExpiredSession = errors.New("session invalid or expired")
	// ErrRevokerUnavailable means refresh/logout cannot fail closed.
	ErrRevokerUnavailable = errors.New("session revoker is not configured")
	// ErrAdminLoginNotConfigured matches legacy admin-login without env creds.
	ErrAdminLoginNotConfigured = errors.New("admin login is not configured")
	// ErrAdminLoginMissingCredentials matches the legacy validation detail.
	ErrAdminLoginMissingCredentials = errors.New("admin login username and password are required")
	// ErrAdminLoginInvalidCredentials matches the legacy auth failure detail.
	ErrAdminLoginInvalidCredentials = errors.New("admin login credentials are invalid")
	// ErrAdminPasswordChangeMissingCredentials means the change-password body is incomplete.
	ErrAdminPasswordChangeMissingCredentials = errors.New("admin password change current and new password are required")
	// ErrAdminPasswordChangeInvalidNewPassword means the replacement password is unsafe.
	ErrAdminPasswordChangeInvalidNewPassword = errors.New("admin password change new password is invalid")
	// ErrAdminPasswordChangeInvalidCurrent means the current password did not verify.
	ErrAdminPasswordChangeInvalidCurrent = errors.New("admin password change current password is invalid")
	// ErrPasswordlessLoginDisabled matches legacy /session/login when disabled.
	ErrPasswordlessLoginDisabled = errors.New("passwordless login disabled")
	// ErrAssigneeIDRequired matches legacy /session/login validation.
	ErrAssigneeIDRequired = errors.New("assignee_id is required")
	// ErrAssigneeUserNotFoundOrDisabled matches legacy /session/login auth.
	ErrAssigneeUserNotFoundOrDisabled = errors.New("assignee user not found or disabled")
	// ErrUserResolverUnavailable means cs_users lookup cannot be performed.
	ErrUserResolverUnavailable = errors.New("session user resolver is not configured")
	// ErrCSLoginMissingCredentials matches legacy /session/cs-login validation.
	ErrCSLoginMissingCredentials = errors.New("cs login assignee and password are required")
	// ErrCSLoginUserNotFoundOrDisabled matches legacy /session/cs-login auth.
	ErrCSLoginUserNotFoundOrDisabled = errors.New("cs login user not found or disabled")
	// ErrCSLoginInvalidCredentials matches legacy /session/cs-login password failure.
	ErrCSLoginInvalidCredentials = errors.New("cs login credentials are invalid")
	// ErrPermissionDenied matches legacy require_roles failures.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrCSUserNotFoundOrDisabled matches admin CS token generation.
	ErrCSUserNotFoundOrDisabled = errors.New("cs user not found or disabled")
	// ErrLoginRateLimited matches legacy auth source-IP throttling.
	ErrLoginRateLimited = errors.New("login rate limited")
)

const (
	// AdminPasswordChangeRole can only complete the mandatory password change flow.
	AdminPasswordChangeRole = "admin_password_change"
	adminDisplayName        = "管理员"
)

// Profile is the cs_users-backed subset needed by /api/v1/session/me.
type Profile struct {
	AIEnabled bool
}

// User is the cs_users-backed subset needed by login endpoints.
type User struct {
	AssigneeID   string
	AssigneeName string
	Role         string
	Enabled      bool
	AIEnabled    bool
	PasswordHash string
}

// ProfileResolver loads optional user details for a verified session.
type ProfileResolver interface {
	GetProfile(ctx context.Context, assigneeID string) (Profile, bool, error)
}

// UserResolver loads CS user rows for login endpoints.
type UserResolver interface {
	GetUser(ctx context.Context, assigneeID string) (User, bool, error)
}

// LastSeenUpdater records low-frequency last_seen changes for CS users.
type LastSeenUpdater interface {
	UpdateLastSeen(ctx context.Context, assigneeID string) error
}

// LoginAttemptLimiter limits login attempts by normalized source key.
type LoginAttemptLimiter interface {
	Check(key string) (bool, string)
	Record(key string)
}

// AuditLogWriter appends legacy audit_logs rows for session actions.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry AuditLogEntry) error
}

// AuditLogEntry is the audit_logs subset written by session endpoints.
type AuditLogEntry struct {
	Operator   string
	ActionType string
	Detail     string
	IP         string
}

// LoginMetadata carries transport-derived facts into login services.
type LoginMetadata struct {
	ClientIP string
}

// LoginRateLimitError preserves the legacy 429 detail text.
type LoginRateLimitError struct {
	Reason string
}

func (err LoginRateLimitError) Error() string {
	if strings.TrimSpace(err.Reason) == "" {
		return ErrLoginRateLimited.Error()
	}
	return err.Reason
}

func (err LoginRateLimitError) Is(target error) bool {
	return target == ErrLoginRateLimited
}

// AdminUser is the stored management credential used by /admin-login.
type AdminUser struct {
	Username               string
	PasswordHash           string
	PasswordChangeRequired bool
}

// AdminUserStore loads and mutates stored management credentials.
type AdminUserStore interface {
	GetAdminUser(ctx context.Context, username string) (AdminUser, bool, error)
	UpdateAdminPassword(ctx context.Context, username string, passwordHash string, passwordChangeRequired bool) error
	RecordAdminLogin(ctx context.Context, username string) error
}

// Service builds session endpoint responses from verified JWT identities.
type Service struct {
	Verifier          auth.Verifier
	Revoker           auth.Revoker
	Profiles          ProfileResolver
	Users             UserResolver
	LastSeen          LastSeenUpdater
	LastSeenThrottle  time.Duration
	RefreshTTL        time.Duration
	AdminUsers        AdminUserStore
	AdminLoginTTL     time.Duration
	PasswordlessLogin bool
	LoginLimiter      LoginAttemptLimiter
	AuditLogs         AuditLogWriter

	mu                sync.Mutex
	now               func() time.Time
	lastSeenDeadlines map[string]time.Time
}

// LoginResponse is the JSON shape returned by legacy login endpoints.
type LoginResponse struct {
	Success                bool   `json:"success"`
	Token                  string `json:"token"`
	AssigneeID             string `json:"assignee_id"`
	AssigneeName           string `json:"assignee_name"`
	Role                   string `json:"role"`
	ExpiresAt              string `json:"expires_at"`
	PasswordChangeRequired bool   `json:"password_change_required"`
}

// GenerateCSTokenResponse is returned by legacy admin/generate-cs-token.
type GenerateCSTokenResponse struct {
	Success      bool   `json:"success"`
	Token        string `json:"token"`
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
	ExpiresAt    string `json:"expires_at"`
}

// MeResponse is the JSON shape returned by the legacy /api/v1/session/me.
type MeResponse struct {
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
	Role         string `json:"role"`
	AIEnabled    bool   `json:"ai_enabled"`
	ExpiresAt    string `json:"expires_at"`
}

// RefreshResponse is the JSON shape returned by legacy /api/v1/session/refresh.
type RefreshResponse struct {
	Success      bool   `json:"success"`
	Token        string `json:"token"`
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
	Role         string `json:"role"`
	AIEnabled    bool   `json:"ai_enabled"`
	ExpiresAt    string `json:"expires_at"`
}

// LogoutResponse is the JSON shape returned by legacy /api/v1/session/logout.
type LogoutResponse struct {
	Success bool `json:"success"`
}

// AssigneeLoginRequest describes legacy passwordless /api/v1/session/login.
type AssigneeLoginRequest struct {
	AssigneeID   string
	AssigneeName string
	TTLHours     int
}

// CSLoginRequest describes legacy /api/v1/session/cs-login.
type CSLoginRequest struct {
	AssigneeID string
	Password   string
}

// AdminPasswordChangeRequest describes the mandatory admin password reset body.
type AdminPasswordChangeRequest struct {
	CurrentPassword string
	NewPassword     string
}

// AdminLogin verifies stored admin credentials and issues a session token.
func (service *Service) AdminLogin(ctx context.Context, username string, password string, metadata ...LoginMetadata) (LoginResponse, error) {
	meta := firstLoginMetadata(metadata)
	if err := service.checkLoginRateLimit(meta.ClientIP); err != nil {
		return LoginResponse{}, err
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrAdminLoginMissingCredentials
	}
	if service.AdminUsers != nil {
		adminUser, ok, err := service.AdminUsers.GetAdminUser(ctx, username)
		if err != nil {
			return LoginResponse{}, err
		}
		if !ok || !auth.VerifyPasswordHash(adminUser.PasswordHash, password) {
			service.recordLoginAttempt(meta.ClientIP)
			return LoginResponse{}, ErrAdminLoginInvalidCredentials
		}
		if strings.TrimSpace(adminUser.Username) == "" {
			adminUser.Username = username
		}
		if err := service.AdminUsers.RecordAdminLogin(ctx, adminUser.Username); err != nil {
			return LoginResponse{}, err
		}
		response, err := service.issueAdminToken(ctx, adminUser.Username, adminUser.PasswordChangeRequired, meta, "login", "管理员账密登录")
		service.recordLoginAttempt(meta.ClientIP)
		return response, err
	}

	return LoginResponse{}, ErrAdminLoginNotConfigured
}

// ChangeAdminPassword completes the forced first-login password change.
func (service *Service) ChangeAdminPassword(ctx context.Context, authorization string, request AdminPasswordChangeRequest, metadata ...LoginMetadata) (LoginResponse, error) {
	meta := firstLoginMetadata(metadata)
	if service.AdminUsers == nil {
		return LoginResponse{}, ErrAdminLoginNotConfigured
	}
	token := auth.ParseBearerToken(authorization)
	if token == "" {
		return LoginResponse{}, ErrMissingBearerToken
	}
	verified, err := service.Verifier.VerifyContext(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrBlacklistUnavailable) {
			return LoginResponse{}, err
		}
		return LoginResponse{}, ErrInvalidOrExpiredSession
	}
	if !verified.HasRole("admin", AdminPasswordChangeRole) {
		return LoginResponse{}, ErrPermissionDenied
	}
	username := strings.TrimSpace(verified.AssigneeID)
	currentPassword := strings.TrimSpace(request.CurrentPassword)
	newPassword := strings.TrimSpace(request.NewPassword)
	if username == "" || currentPassword == "" || newPassword == "" {
		return LoginResponse{}, ErrAdminPasswordChangeMissingCredentials
	}
	if len([]rune(newPassword)) < 10 || newPassword == currentPassword {
		return LoginResponse{}, ErrAdminPasswordChangeInvalidNewPassword
	}
	adminUser, ok, err := service.AdminUsers.GetAdminUser(ctx, username)
	if err != nil {
		return LoginResponse{}, err
	}
	if !ok || !auth.VerifyPasswordHash(adminUser.PasswordHash, currentPassword) {
		return LoginResponse{}, ErrAdminPasswordChangeInvalidCurrent
	}
	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return LoginResponse{}, err
	}
	if err := service.AdminUsers.UpdateAdminPassword(ctx, username, passwordHash, false); err != nil {
		return LoginResponse{}, err
	}
	return service.issueAdminToken(ctx, username, false, meta, "password_change", "管理员首次登录修改密码")
}

func (service *Service) issueAdminToken(ctx context.Context, username string, passwordChangeRequired bool, meta LoginMetadata, auditAction string, auditDetail string) (LoginResponse, error) {
	ttl := service.AdminLoginTTL
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	role := "admin"
	if passwordChangeRequired {
		role = AdminPasswordChangeRole
	}
	issued, err := service.Verifier.Issue(auth.IssueOptions{
		AssigneeID:   strings.TrimSpace(username),
		AssigneeName: adminDisplayName,
		Role:         role,
		TTL:          ttl,
	})
	if err != nil {
		return LoginResponse{}, err
	}
	_ = service.addAuditLog(ctx, AuditLogEntry{
		Operator:   issued.AssigneeID,
		ActionType: auditAction,
		Detail:     auditDetail,
		IP:         meta.ClientIP,
	})
	return LoginResponse{
		Success:                true,
		Token:                  issued.Token,
		AssigneeID:             issued.AssigneeID,
		AssigneeName:           issued.AssigneeName,
		Role:                   issued.Role,
		ExpiresAt:              formatLegacyISOTime(issued.ExpiresAt),
		PasswordChangeRequired: passwordChangeRequired,
	}, nil
}

// AssigneeLogin verifies an enabled cs_users row and issues a session token.
func (service *Service) AssigneeLogin(ctx context.Context, request AssigneeLoginRequest, metadata ...LoginMetadata) (LoginResponse, error) {
	meta := firstLoginMetadata(metadata)
	if err := service.checkLoginRateLimit(meta.ClientIP); err != nil {
		return LoginResponse{}, err
	}
	if !service.PasswordlessLogin {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrPasswordlessLoginDisabled
	}
	assigneeID := strings.TrimSpace(request.AssigneeID)
	if assigneeID == "" {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrAssigneeIDRequired
	}
	if service.Users == nil {
		return LoginResponse{}, ErrUserResolverUnavailable
	}
	user, ok, err := service.Users.GetUser(ctx, assigneeID)
	if err != nil {
		return LoginResponse{}, err
	}
	if !ok || !user.Enabled {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrAssigneeUserNotFoundOrDisabled
	}
	assigneeName := strings.TrimSpace(user.AssigneeName)
	if override := strings.TrimSpace(request.AssigneeName); override != "" {
		assigneeName = override
	}
	ttlHours := request.TTLHours
	if ttlHours == 0 {
		ttlHours = 168
	}
	issued, err := service.Verifier.Issue(auth.IssueOptions{
		AssigneeID:   assigneeID,
		AssigneeName: assigneeName,
		Role:         user.Role,
		TTL:          time.Duration(ttlHours) * time.Hour,
	})
	if err != nil {
		return LoginResponse{}, err
	}
	if err := service.addAuditLog(ctx, AuditLogEntry{
		Operator:   assigneeID,
		ActionType: "login",
		Detail:     fmt.Sprintf("用户 %s(%s) 登录", assigneeName, assigneeID),
		IP:         meta.ClientIP,
	}); err != nil {
		return LoginResponse{}, err
	}
	service.recordLoginAttempt(meta.ClientIP)
	return LoginResponse{
		Success:      true,
		Token:        issued.Token,
		AssigneeID:   issued.AssigneeID,
		AssigneeName: issued.AssigneeName,
		Role:         issued.Role,
		ExpiresAt:    formatLegacyISOTime(issued.ExpiresAt),
	}, nil
}

// CSLogin verifies a configured CS password hash and issues a 7-day token.
func (service *Service) CSLogin(ctx context.Context, request CSLoginRequest, metadata ...LoginMetadata) (LoginResponse, error) {
	meta := firstLoginMetadata(metadata)
	if err := service.checkLoginRateLimit(meta.ClientIP); err != nil {
		return LoginResponse{}, err
	}
	assigneeID := strings.TrimSpace(request.AssigneeID)
	password := strings.TrimSpace(request.Password)
	if assigneeID == "" || password == "" {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrCSLoginMissingCredentials
	}
	if service.Users == nil {
		return LoginResponse{}, ErrUserResolverUnavailable
	}
	user, ok, err := service.Users.GetUser(ctx, assigneeID)
	if err != nil {
		return LoginResponse{}, err
	}
	if !ok || !user.Enabled {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrCSLoginUserNotFoundOrDisabled
	}
	if !verifySHA256Password(password, user.PasswordHash) {
		service.recordLoginAttempt(meta.ClientIP)
		return LoginResponse{}, ErrCSLoginInvalidCredentials
	}
	issued, err := service.Verifier.Issue(auth.IssueOptions{
		AssigneeID:   assigneeID,
		AssigneeName: user.AssigneeName,
		Role:         user.Role,
		TTL:          168 * time.Hour,
	})
	if err != nil {
		return LoginResponse{}, err
	}
	if service.LastSeen != nil {
		if err := service.LastSeen.UpdateLastSeen(ctx, assigneeID); err != nil {
			return LoginResponse{}, err
		}
	}
	if err := service.addAuditLog(ctx, AuditLogEntry{
		Operator:   assigneeID,
		ActionType: "login",
		Detail:     fmt.Sprintf("消息端 %s(%s) 密码登录", user.AssigneeName, assigneeID),
		IP:         meta.ClientIP,
	}); err != nil {
		return LoginResponse{}, err
	}
	service.recordLoginAttempt(meta.ClientIP)
	return LoginResponse{
		Success:      true,
		Token:        issued.Token,
		AssigneeID:   issued.AssigneeID,
		AssigneeName: issued.AssigneeName,
		Role:         issued.Role,
		ExpiresAt:    formatLegacyISOTime(issued.ExpiresAt),
	}, nil
}

// GenerateCSToken lets admins mint a short-lived workspace token for a CS user.
func (service *Service) GenerateCSToken(ctx context.Context, authorization string, assigneeID string, metadata ...LoginMetadata) (GenerateCSTokenResponse, error) {
	meta := firstLoginMetadata(metadata)
	token := auth.ParseBearerToken(authorization)
	if token == "" {
		return GenerateCSTokenResponse{}, ErrMissingBearerToken
	}
	adminSession, err := service.Verifier.VerifyContext(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrBlacklistUnavailable) {
			return GenerateCSTokenResponse{}, err
		}
		return GenerateCSTokenResponse{}, ErrInvalidOrExpiredSession
	}
	if !adminSession.HasRole("admin", "supervisor") {
		return GenerateCSTokenResponse{}, ErrPermissionDenied
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return GenerateCSTokenResponse{}, ErrAssigneeIDRequired
	}
	if service.Users == nil {
		return GenerateCSTokenResponse{}, ErrUserResolverUnavailable
	}
	user, ok, err := service.Users.GetUser(ctx, assigneeID)
	if err != nil {
		return GenerateCSTokenResponse{}, err
	}
	if !ok || !user.Enabled {
		return GenerateCSTokenResponse{}, ErrCSUserNotFoundOrDisabled
	}
	issued, err := service.Verifier.Issue(auth.IssueOptions{
		AssigneeID:   assigneeID,
		AssigneeName: user.AssigneeName,
		Role:         user.Role,
		TTL:          24 * time.Hour,
	})
	if err != nil {
		return GenerateCSTokenResponse{}, err
	}
	operator := strings.TrimSpace(adminSession.AssigneeID)
	if operator == "" {
		operator = "admin"
	}
	if err := service.addAuditLog(ctx, AuditLogEntry{
		Operator:   operator,
		ActionType: "impersonate",
		Detail:     fmt.Sprintf("管理员 %s 为消息端 %s(%s) 生成工作台 Token", operator, assigneeID, user.AssigneeName),
		IP:         meta.ClientIP,
	}); err != nil {
		return GenerateCSTokenResponse{}, err
	}
	return GenerateCSTokenResponse{
		Success:      true,
		Token:        issued.Token,
		AssigneeID:   issued.AssigneeID,
		AssigneeName: issued.AssigneeName,
		ExpiresAt:    formatLegacyISOTime(issued.ExpiresAt),
	}, nil
}

// CurrentUser verifies Authorization and returns the legacy /me response body.
func (service *Service) CurrentUser(ctx context.Context, authorization string) (MeResponse, error) {
	token := auth.ParseBearerToken(authorization)
	if token == "" {
		return MeResponse{}, ErrMissingBearerToken
	}
	verified, err := service.Verifier.VerifyContext(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrBlacklistUnavailable) {
			return MeResponse{}, err
		}
		return MeResponse{}, ErrInvalidOrExpiredSession
	}
	if service.shouldUpdateLastSeen(verified.AssigneeID) && service.LastSeen != nil {
		_ = service.LastSeen.UpdateLastSeen(ctx, verified.AssigneeID)
	}
	aiEnabled, err := service.aiEnabled(ctx, verified.AssigneeID)
	if err != nil {
		return MeResponse{}, err
	}
	return MeResponse{
		AssigneeID:   verified.AssigneeID,
		AssigneeName: verified.AssigneeName,
		Role:         verified.Role,
		AIEnabled:    aiEnabled,
		ExpiresAt:    formatLegacyISOTime(verified.ExpiresAt),
	}, nil
}

// Refresh revokes the old JWT and issues a new legacy-compatible session token.
func (service *Service) Refresh(ctx context.Context, authorization string) (RefreshResponse, error) {
	token := auth.ParseBearerToken(authorization)
	if token == "" {
		return RefreshResponse{}, ErrMissingBearerToken
	}
	verified, err := service.Verifier.VerifyContext(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrBlacklistUnavailable) {
			return RefreshResponse{}, err
		}
		return RefreshResponse{}, ErrInvalidOrExpiredSession
	}
	if service.Revoker == nil {
		return RefreshResponse{}, ErrRevokerUnavailable
	}
	if err := service.Revoker.Add(ctx, verified.JTI, verified.ExpiresAt); err != nil {
		return RefreshResponse{}, fmt.Errorf("revoke old session: %w", err)
	}
	ttl := service.RefreshTTL
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	issued, err := service.Verifier.Issue(auth.IssueOptions{
		AssigneeID:   verified.AssigneeID,
		AssigneeName: verified.AssigneeName,
		Role:         verified.Role,
		TTL:          ttl,
	})
	if err != nil {
		return RefreshResponse{}, err
	}
	aiEnabled, err := service.aiEnabled(ctx, verified.AssigneeID)
	if err != nil {
		return RefreshResponse{}, err
	}
	return RefreshResponse{
		Success:      true,
		Token:        issued.Token,
		AssigneeID:   issued.AssigneeID,
		AssigneeName: issued.AssigneeName,
		Role:         issued.Role,
		AIEnabled:    aiEnabled,
		ExpiresAt:    formatLegacyISOTime(issued.ExpiresAt),
	}, nil
}

// Logout revokes the signed JWT when it contains a revocable jti/exp pair.
func (service *Service) Logout(ctx context.Context, authorization string, metadata ...LoginMetadata) (LogoutResponse, error) {
	meta := firstLoginMetadata(metadata)
	token := auth.ParseBearerToken(authorization)
	if token == "" {
		return LogoutResponse{}, ErrMissingBearerToken
	}
	if service.Revoker == nil {
		return LogoutResponse{}, ErrRevokerUnavailable
	}
	revocable, ok := service.Verifier.DecodeRevocableToken(token)
	if !ok {
		return LogoutResponse{Success: false}, nil
	}
	if err := service.Revoker.Add(ctx, revocable.JTI, revocable.ExpiresAt); err != nil {
		return LogoutResponse{}, fmt.Errorf("revoke session: %w", err)
	}
	assigneeID := strings.TrimSpace(textClaim(revocable.Claims, "sub"))
	assigneeName := strings.TrimSpace(textClaim(revocable.Claims, "name"))
	if assigneeID != "" {
		if err := service.addAuditLog(ctx, AuditLogEntry{
			Operator:   assigneeID,
			ActionType: "logout",
			Detail:     fmt.Sprintf("用户 %s 登出", assigneeName),
			IP:         meta.ClientIP,
		}); err != nil {
			return LogoutResponse{}, err
		}
	}
	return LogoutResponse{Success: true}, nil
}

func (service *Service) aiEnabled(ctx context.Context, assigneeID string) (bool, error) {
	if service.Profiles == nil {
		return false, nil
	}
	profile, ok, err := service.Profiles.GetProfile(ctx, assigneeID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return profile.AIEnabled, nil
}

func firstLoginMetadata(metadata []LoginMetadata) LoginMetadata {
	if len(metadata) == 0 {
		return LoginMetadata{}
	}
	return LoginMetadata{ClientIP: strings.TrimSpace(metadata[0].ClientIP)}
}

func authRateLimitKey(clientIP string) string {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return ""
	}
	return "auth:" + clientIP
}

func (service *Service) checkLoginRateLimit(clientIP string) error {
	key := authRateLimitKey(clientIP)
	if key == "" || service.LoginLimiter == nil {
		return nil
	}
	allowed, reason := service.LoginLimiter.Check(key)
	if allowed {
		return nil
	}
	return LoginRateLimitError{Reason: reason}
}

func (service *Service) recordLoginAttempt(clientIP string) {
	key := authRateLimitKey(clientIP)
	if key == "" || service.LoginLimiter == nil {
		return
	}
	service.LoginLimiter.Record(key)
}

func (service *Service) addAuditLog(ctx context.Context, entry AuditLogEntry) error {
	if service.AuditLogs == nil {
		return nil
	}
	entry.Operator = strings.TrimSpace(entry.Operator)
	entry.ActionType = strings.TrimSpace(entry.ActionType)
	entry.Detail = strings.TrimSpace(entry.Detail)
	entry.IP = strings.TrimSpace(entry.IP)
	return service.AuditLogs.AddAuditLog(ctx, entry)
}

func textClaim(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (service *Service) shouldUpdateLastSeen(assigneeID string) bool {
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" || assigneeID == "admin" {
		return false
	}
	now := service.clock()
	throttle := service.LastSeenThrottle
	if throttle <= 0 {
		throttle = 60 * time.Second
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.lastSeenDeadlines == nil {
		service.lastSeenDeadlines = map[string]time.Time{}
	}
	if deadline := service.lastSeenDeadlines[assigneeID]; deadline.After(now) {
		return false
	}
	service.lastSeenDeadlines[assigneeID] = now.Add(throttle)
	return true
}

func (service *Service) clock() time.Time {
	if service.now == nil {
		return time.Now()
	}
	return service.now()
}

func formatLegacyISOTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05+00:00")
}

func verifySHA256Password(password string, expectedHash string) bool {
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))
	if expectedHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:]) == expectedHash
}
