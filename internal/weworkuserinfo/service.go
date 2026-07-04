// Package weworkuserinfo ports read-only WeWork user-info identity helpers.
package weworkuserinfo

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"wework-go/internal/readmodelcache"
	"wework-go/internal/workbench"
)

var (
	// ErrDeviceIDRequired preserves the legacy required device_id body/query field.
	ErrDeviceIDRequired = errors.New("device_id is required")
	// ErrStoreUnavailable means a required read store was not configured.
	ErrStoreUnavailable = errors.New("wework user info store is unavailable")
	// ErrSelectedIdentityMismatch mirrors the legacy manual-selection validation failure.
	ErrSelectedIdentityMismatch = errors.New("selected wework_user_id does not match current account")
	// ErrSDKRouteUnavailable mirrors the legacy SDK route preflight failure.
	ErrSDKRouteUnavailable = errors.New("SDK route is not configured for this device")

	matchTextSeparatorPattern = regexp.MustCompile(`[[:space:]\-_/·.。()（）]+`)
	internalUserTailPattern   = regexp.MustCompile(`([a-zA-Z]{1,8}-?\d{3,8})$`)
	cjkChunkPattern           = regexp.MustCompile(`[\p{Han}]{2,20}`)
	cjkAnyPattern             = regexp.MustCompile(`[\p{Han}]`)
)

// LoginSessionStore reads current device login identities.
type LoginSessionStore interface {
	ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error)
}

// LoginSessionWriter updates the local WeCom login identity overlay.
type LoginSessionWriter interface {
	UpsertLoginSession(ctx context.Context, record workbench.LoginSessionRecord) (workbench.LoginSessionRecord, error)
}

// EnterpriseStore lists enterprise identity fields needed for org matching.
type EnterpriseStore interface {
	ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error)
}

// EnterpriseStoreFunc adapts a function into EnterpriseStore.
type EnterpriseStoreFunc func(ctx context.Context) ([]EnterpriseRecord, error)

// ListEnterprises calls fn.
func (fn EnterpriseStoreFunc) ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error) {
	return fn(ctx)
}

// InternalUserCandidateStore reads local corp-user candidates.
type InternalUserCandidateStore interface {
	ListInternalUserCandidatesByNames(ctx context.Context, enterpriseID string, names []string, limit int) ([]InternalUserCandidate, error)
}

// InternalUserResolver reads one cached internal WeCom user by userid.
type InternalUserResolver interface {
	GetInternalUserByUserID(ctx context.Context, enterpriseID string, userID string) (InternalUserCandidate, bool, error)
}

// AccountStore reads and writes local account bindings for manual repair.
type AccountStore interface {
	ListAccounts(ctx context.Context) ([]workbench.AccountRecord, error)
	UpsertAccount(ctx context.Context, command workbench.AccountUpsertCommand) (workbench.AccountRecord, error)
}

// EventPublisher publishes Python-compatible workbench/device events.
type EventPublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// AuditLogWriter records the operator-visible repair action.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// SDKDeviceChecker checks whether the sidecar currently owns one SDK device.
type SDKDeviceChecker interface {
	HasDevice(ctx context.Context, deviceID string) (bool, error)
}

// ReadModelInvalidator invalidates cached workbench read-model namespaces.
type ReadModelInvalidator interface {
	InvalidateNamespaces(ctx context.Context, namespaces ...string) error
}

// EnterpriseRecord is the matching subset of the legacy enterprises row.
type EnterpriseRecord struct {
	EnterpriseID string
	CorpID       string
	Name         string
}

// InternalUserCandidate is one local wework_corp_users match.
type InternalUserCandidate struct {
	EnterpriseID   string
	UserID         string
	Name           string
	DepartmentJSON []any
	Position       string
	Avatar         string
	SyncedAt       string
	UpdatedAt      string
}

// CandidatePayload is the legacy JSON shape for one candidate row.
type CandidatePayload struct {
	EnterpriseID   string `json:"enterprise_id"`
	UserID         string `json:"userid"`
	Name           string `json:"name"`
	DepartmentJSON []any  `json:"department_json"`
	Position       string `json:"position"`
	Avatar         string `json:"avatar"`
	SyncedAt       string `json:"synced_at"`
	UpdatedAt      string `json:"updated_at"`
}

// CandidatesResult is the legacy JSON response for GET /wework/user-info/candidates.
type CandidatesResult struct {
	Success           bool               `json:"success"`
	RequiresSelection bool               `json:"requires_selection"`
	DeviceID          string             `json:"device_id"`
	AccountName       string             `json:"account_name"`
	OrganizationName  string             `json:"organization_name"`
	EnterpriseID      string             `json:"enterprise_id"`
	Candidates        []CandidatePayload `json:"candidates"`
}

// Service resolves local user-info candidate choices without remote WeWork calls.
type Service struct {
	LoginSessions              LoginSessionStore
	LoginWriter                LoginSessionWriter
	Enterprises                EnterpriseStore
	UserCandidates             InternalUserCandidateStore
	InternalUsers              InternalUserResolver
	Accounts                   AccountStore
	TaskCreator                TaskCreator
	Events                     EventPublisher
	AuditLogs                  AuditLogWriter
	Invalidator                ReadModelInvalidator
	SDKDevices                 SDKDeviceChecker
	RequireSDKDeviceConfigured bool
	Now                        func() time.Time
	NewID                      func(prefix string) string
}

// Candidates returns possible internal userid matches for the device login name.
func (service Service) Candidates(ctx context.Context, deviceID string, limit int) (CandidatesResult, error) {
	normalizedDeviceID := strings.TrimSpace(deviceID)
	result := CandidatesResult{
		Success:    true,
		DeviceID:   normalizedDeviceID,
		Candidates: []CandidatePayload{},
	}
	if normalizedDeviceID == "" {
		return result, nil
	}
	if service.LoginSessions == nil || service.Enterprises == nil {
		return result, ErrStoreUnavailable
	}
	session, err := service.currentLoginSession(ctx, normalizedDeviceID)
	if err != nil {
		return result, err
	}
	result.AccountName = strings.TrimSpace(session.AccountName)
	result.OrganizationName = strings.TrimSpace(session.OrganizationName)
	if result.AccountName == "" || result.OrganizationName == "" {
		return result, nil
	}
	enterpriseID, err := service.matchEnterpriseID(ctx, result.OrganizationName)
	if err != nil {
		return result, err
	}
	result.EnterpriseID = enterpriseID
	if enterpriseID == "" || service.UserCandidates == nil {
		return result, nil
	}
	candidates, err := service.UserCandidates.ListInternalUserCandidatesByNames(ctx, enterpriseID, []string{result.AccountName}, clampLimit(limit))
	if err != nil {
		return result, err
	}
	if len(candidates) == 0 {
		candidates, err = service.UserCandidates.ListInternalUserCandidatesByNames(ctx, enterpriseID, buildInternalUserNameCandidates(result.AccountName), clampLimit(limit))
		if err != nil {
			return result, err
		}
	}
	result.Candidates = candidatePayloads(enterpriseID, candidates)
	result.RequiresSelection = len(result.Candidates) > 1
	return result, nil
}

func (service Service) currentLoginSession(ctx context.Context, deviceID string) (workbench.LoginSessionRecord, error) {
	sessions, err := service.LoginSessions.ListLoginSessions(ctx, []string{deviceID})
	if err != nil {
		return workbench.LoginSessionRecord{}, err
	}
	for _, session := range sessions {
		if strings.TrimSpace(session.DeviceID) == deviceID {
			return session, nil
		}
	}
	return workbench.LoginSessionRecord{DeviceID: deviceID}, nil
}

func (service Service) matchEnterpriseID(ctx context.Context, organizationName string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(organizationName))
	targetNorm := normalizeMatchText(organizationName)
	if target == "" {
		return "", nil
	}
	records, err := service.Enterprises.ListEnterprises(ctx)
	if err != nil {
		return "", err
	}
	for _, item := range records {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		nameNorm := normalizeMatchText(item.Name)
		corpID := strings.ToLower(strings.TrimSpace(item.CorpID))
		enterpriseID := strings.TrimSpace(item.EnterpriseID)
		if enterpriseID == "" {
			continue
		}
		if target == name ||
			target == corpID ||
			(name != "" && (strings.Contains(target, name) || strings.Contains(name, target))) ||
			(targetNorm != "" && nameNorm != "" && (targetNorm == nameNorm || strings.Contains(targetNorm, nameNorm) || strings.Contains(nameNorm, targetNorm))) {
			return enterpriseID, nil
		}
	}
	return "", nil
}

func candidatePayloads(enterpriseID string, candidates []InternalUserCandidate) []CandidatePayload {
	payloads := make([]CandidatePayload, 0, len(candidates))
	for _, candidate := range candidates {
		userID := strings.TrimSpace(candidate.UserID)
		if userID == "" {
			continue
		}
		departments := candidate.DepartmentJSON
		if departments == nil {
			departments = []any{}
		}
		payloads = append(payloads, CandidatePayload{
			EnterpriseID:   defaultString(strings.TrimSpace(candidate.EnterpriseID), enterpriseID),
			UserID:         userID,
			Name:           strings.TrimSpace(candidate.Name),
			DepartmentJSON: departments,
			Position:       strings.TrimSpace(candidate.Position),
			Avatar:         strings.TrimSpace(candidate.Avatar),
			SyncedAt:       strings.TrimSpace(candidate.SyncedAt),
			UpdatedAt:      strings.TrimSpace(candidate.UpdatedAt),
		})
	}
	return payloads
}

func buildInternalUserNameCandidates(userName string) []string {
	raw := strings.TrimSpace(userName)
	if raw == "" {
		return []string{}
	}
	candidates := make([]string, 0, 4)
	push := func(value string) {
		item := strings.TrimSpace(value)
		if item == "" {
			return
		}
		for _, existing := range candidates {
			if existing == item {
				return
			}
		}
		candidates = append(candidates, item)
	}
	push(raw)
	trimmedTail := strings.Trim(internalUserTailPattern.ReplaceAllString(raw, ""), " -_/·.。()（）")
	push(trimmedTail)
	source := defaultString(trimmedTail, raw)
	for _, chunk := range cjkChunkPattern.FindAllString(source, -1) {
		push(chunk)
	}
	for _, part := range matchTextSeparatorPattern.Split(source, -1) {
		if cjkAnyPattern.MatchString(part) {
			push(part)
		}
	}
	return candidates
}

func normalizeMatchText(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return ""
	}
	return matchTextSeparatorPattern.ReplaceAllString(text, "")
}

func internalIdentityMatchesAccountName(candidate InternalUserCandidate, accountName string) bool {
	targetName := strings.TrimSpace(accountName)
	if targetName == "" {
		return true
	}
	resolvedName := strings.TrimSpace(candidate.Name)
	resolvedUserID := strings.TrimSpace(candidate.UserID)
	normalizedUserID := workbench.NormalizeIDHint(resolvedUserID)
	normalizedTarget := workbench.NormalizeIDHint(targetName)
	if normalizedUserID != "" && normalizedTarget == normalizedUserID {
		return true
	}
	if resolvedName == "" {
		return false
	}
	resolvedNorm := normalizeMatchText(resolvedName)
	targetNorm := normalizeMatchText(targetName)
	if resolvedNorm != "" && targetNorm != "" && resolvedNorm == targetNorm {
		return true
	}
	for _, name := range buildInternalUserNameCandidates(targetName) {
		if normalizeMatchText(name) != "" && normalizeMatchText(name) == resolvedNorm {
			return true
		}
	}
	for _, name := range buildInternalUserNameCandidates(resolvedName) {
		if normalizeMatchText(name) != "" && normalizeMatchText(name) == targetNorm {
			return true
		}
	}
	return false
}

func findAccountForManualRepair(accounts []workbench.AccountRecord, deviceID string, weworkUserID string, accountName string, enterpriseID string) (workbench.AccountRecord, bool) {
	normalizedDeviceID := strings.TrimSpace(deviceID)
	normalizedUserID := workbench.NormalizeIDHint(weworkUserID)
	for _, account := range accounts {
		if normalizedUserID != "" && workbench.NormalizeIDHint(account.WeWorkUserID) == normalizedUserID {
			return account, true
		}
	}
	targetName := normalizeMatchText(accountName)
	targetEnterpriseID := strings.TrimSpace(enterpriseID)
	for _, account := range accounts {
		if targetEnterpriseID != "" && strings.TrimSpace(account.EnterpriseID) != "" && strings.TrimSpace(account.EnterpriseID) != targetEnterpriseID {
			continue
		}
		if targetName != "" && normalizeMatchText(account.AccountName) == targetName {
			return account, true
		}
	}
	for _, account := range accounts {
		if normalizedDeviceID != "" && strings.TrimSpace(account.DeviceID) == normalizedDeviceID {
			return account, true
		}
	}
	return workbench.AccountRecord{}, false
}

func accountPayload(account workbench.AccountRecord, organizationName string, accountAvatar string) map[string]any {
	payload := map[string]any{
		"account_id":             strings.TrimSpace(account.AccountID),
		"account_name":           strings.TrimSpace(account.AccountName),
		"agent_id":               nilIfBlank(strings.TrimSpace(account.AgentID)),
		"device_id":              nilIfBlank(strings.TrimSpace(account.DeviceID)),
		"wework_user_id":         nilIfBlank(strings.TrimSpace(account.WeWorkUserID)),
		"enterprise_id":          nilIfBlank(strings.TrimSpace(account.EnterpriseID)),
		"assignee_id":            nilIfBlank(strings.TrimSpace(account.AssigneeID)),
		"assignee_name":          nilIfBlank(strings.TrimSpace(account.AssigneeName)),
		"organization_name":      nilIfBlank(strings.TrimSpace(organizationName)),
		"account_avatar":         nilIfBlank(strings.TrimSpace(accountAvatar)),
		"sop_flow_id":            nilIfBlank(strings.TrimSpace(account.SOPFlowID)),
		"sop_enabled":            nil,
		"sop_reply_window_start": nilIfBlank(strings.TrimSpace(account.SOPReplyWindowStart)),
		"sop_reply_window_end":   nilIfBlank(strings.TrimSpace(account.SOPReplyWindowEnd)),
		"ai_enabled":             account.AIEnabled,
		"ai_model":               nilIfBlank(strings.TrimSpace(account.AIModel)),
		"knowledge_tag":          nilIfBlank(strings.TrimSpace(account.KnowledgeTag)),
		"created_at":             nilIfBlank(strings.TrimSpace(account.CreatedAt)),
		"updated_at":             nilIfBlank(strings.TrimSpace(account.UpdatedAt)),
	}
	if account.SOPEnabled != nil {
		payload["sop_enabled"] = *account.SOPEnabled
	}
	return payload
}

func (service Service) invalidateReadModels(ctx context.Context) {
	if service.Invalidator == nil {
		return
	}
	_ = service.Invalidator.InvalidateNamespaces(ctx, readmodelcache.AllNamespaces()...)
}

func nilIfBlank(value string) any {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	return normalized
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return strings.TrimSpace(fallback)
	}
	return strings.TrimSpace(value)
}
