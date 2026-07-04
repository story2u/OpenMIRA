package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"wework-go/internal/auth"
)

const aiAssigneeProfileRequiredDetail = "请先在 AI 设置中为该客服或企微账号选择 AI 逻辑，再开启单会话托管"

var (
	ErrConversationAIEnabledRequired    = errors.New("enabled is required")
	ErrConversationAIStoreUnavailable   = errors.New("workbench conversation ai store is unavailable")
	ErrConversationNotFound             = errors.New("conversation not found")
	ErrConversationAIProfileRequired    = errors.New(aiAssigneeProfileRequiredDetail)
	ErrConversationRuntimeStateRequired = errors.New("conversation runtime state store is unavailable")
)

// ConversationAIBody is the JSON input for POST /conversations/{conversation_id}/ai-auto-reply.
type ConversationAIBody struct {
	Enabled *bool `json:"enabled"`
}

// ConversationAIRequest carries a single-conversation AI switch request.
type ConversationAIRequest struct {
	Session        auth.Session
	ConversationID string
	Enabled        *bool
}

// NewConversationAIRequest normalizes the single-conversation switch boundary.
func NewConversationAIRequest(conversationID string, body ConversationAIBody, session auth.Session) ConversationAIRequest {
	return ConversationAIRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
		Enabled:        body.Enabled,
	}
}

// ConversationAIBulkBody is the JSON input for POST /conversations/ai-auto-reply/bulk.
type ConversationAIBulkBody struct {
	Enabled    *bool  `json:"enabled"`
	AssigneeID string `json:"assignee_id"`
	SyncCSUser bool   `json:"sync_cs_user"`
}

// ConversationAIBulkRequest carries a bulk AI switch request.
type ConversationAIBulkRequest struct {
	Session    auth.Session
	Enabled    *bool
	AssigneeID string
	SyncCSUser bool
}

// NewConversationAIBulkRequest normalizes the bulk switch boundary.
func NewConversationAIBulkRequest(body ConversationAIBulkBody, session auth.Session) ConversationAIBulkRequest {
	return ConversationAIBulkRequest{
		Session:    session,
		Enabled:    body.Enabled,
		AssigneeID: strings.TrimSpace(body.AssigneeID),
		SyncCSUser: body.SyncCSUser,
	}
}

// ConversationAIRecord is the narrow conversation shape needed by AI switch writes.
type ConversationAIRecord struct {
	ConversationID  string
	TenantID        string
	AccountID       string
	AssigneeID      string
	AIAutoReply     bool
	AIModeOverride  string
	SOPRuntimeState map[string]any
}

// ConversationAIStore mutates conversation-level AI state and projection rows.
type ConversationAIStore interface {
	GetConversationAI(ctx context.Context, conversationID string) (ConversationAIRecord, bool, error)
	SetConversationAIModeOverride(ctx context.Context, conversationID string, overrideMode string, accountAIEnabled bool) (ConversationAIRecord, bool, error)
	SetConversationAIModeOverrideBulk(ctx context.Context, conversationIDs []string, overrideMode string, accountAIEnabled bool) ([]ConversationAIRecord, error)
	ListAllConversationAIIDs(ctx context.Context) ([]string, error)
	ListAssigneeScopedConversationAIIDs(ctx context.Context, assigneeID string, tenantID string) ([]string, error)
	UpdateConversationRuntimeState(ctx context.Context, conversationID string, runtimeState map[string]any) error
}

// ToggleConversationAI switches one conversation to explicit auto/manual AI mode.
func (service Service) ToggleConversationAI(ctx context.Context, request ConversationAIRequest) (Payload, error) {
	store := service.conversationAIStore()
	if store == nil {
		return nil, ErrConversationAIStoreUnavailable
	}
	if request.Enabled == nil {
		return nil, ErrConversationAIEnabledRequired
	}
	conversationID := strings.TrimSpace(request.ConversationID)
	current, ok, err := store.GetConversationAI(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConversationNotFound
	}
	account, accountAIEnabled, err := service.conversationAIAccount(ctx, current.AccountID)
	if err != nil {
		return nil, err
	}
	enabled := bool(*request.Enabled)
	if enabled {
		if err := service.ensureExplicitAIProfile(ctx, account); err != nil {
			return nil, err
		}
	}
	overrideMode := aiOverrideMode(enabled)
	conversation, ok, err := store.SetConversationAIModeOverride(ctx, conversationID, overrideMode, accountAIEnabled)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConversationNotFound
	}
	effectiveEnabled := conversation.AIAutoReply
	if effectiveEnabled {
		if err := service.clearAIRuntimeAfterManualEnable(ctx, store, conversation); err != nil {
			return nil, err
		}
	}
	if service.ConversationAIEvents != nil {
		if err := service.ConversationAIEvents.Publish(ctx, "conversations", "conversation.ai_auto_reply", "conversation.message", conversationAIEventPayload(conversation, accountAIEnabled)); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	return Payload{
		"success":            true,
		"conversation":       conversationAIPayload(conversation),
		"ai_mode_override":   overrideMode,
		"account_ai_enabled": accountAIEnabled,
		"ai_auto_reply":      effectiveEnabled,
	}, nil
}

// ToggleConversationAIBulk switches AI mode across all or assignee-scoped conversations.
func (service Service) ToggleConversationAIBulk(ctx context.Context, request ConversationAIBulkRequest) (Payload, error) {
	store := service.conversationAIStore()
	if store == nil {
		return nil, ErrConversationAIStoreUnavailable
	}
	if request.Enabled == nil {
		return nil, ErrConversationAIEnabledRequired
	}
	enabled := bool(*request.Enabled)
	role := strings.TrimSpace(request.Session.Role)
	operatorAssignee := strings.TrimSpace(request.Session.AssigneeID)
	targetAssignee := strings.TrimSpace(request.AssigneeID)
	if targetAssignee != "" && !roleAllowsBulkAssignee(role) && targetAssignee != operatorAssignee {
		return nil, auth.ErrPermissionDenied
	}
	if targetAssignee == "" && strings.EqualFold(role, "cs") {
		targetAssignee = operatorAssignee
	}
	tenantID := sessionTenantID(request.Session)
	var conversationIDs []string
	var err error
	if targetAssignee != "" {
		conversationIDs, err = store.ListAssigneeScopedConversationAIIDs(ctx, targetAssignee, tenantID)
	} else {
		conversationIDs, err = store.ListAllConversationAIIDs(ctx)
	}
	if err != nil {
		return nil, err
	}
	overrideMode := aiOverrideMode(enabled)
	updated, err := store.SetConversationAIModeOverrideBulk(ctx, conversationIDs, overrideMode, enabled)
	if err != nil {
		return nil, err
	}
	if request.SyncCSUser && targetAssignee != "" {
		if err := service.syncCSUserAIEnabled(ctx, targetAssignee, enabled); err != nil {
			return nil, err
		}
	}
	if service.ConversationAIEvents != nil {
		if err := service.ConversationAIEvents.Publish(ctx, "conversations", "conversation.ai_auto_reply.bulk", "conversation.message", map[string]any{
			"enabled":       enabled,
			"updated_count": len(updated),
			"assignee_id":   nilIfBlank(targetAssignee),
		}); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	return Payload{
		"success":       true,
		"enabled":       enabled,
		"updated_count": len(updated),
		"assignee_id":   nilIfBlank(targetAssignee),
	}, nil
}

func (service Service) conversationAIStore() ConversationAIStore {
	if service.ConversationAIStore != nil {
		return service.ConversationAIStore
	}
	if store, ok := service.AccountAIWriteStore.(ConversationAIStore); ok {
		return store
	}
	if store, ok := service.Accounts.(ConversationAIStore); ok {
		return store
	}
	return nil
}

func (service Service) conversationAIAccount(ctx context.Context, accountID string) (AccountRecord, bool, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || service.Accounts == nil {
		return AccountRecord{}, false, nil
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return AccountRecord{}, false, err
	}
	account, ok := findAccountByID(accounts, accountID)
	if !ok {
		return AccountRecord{}, false, nil
	}
	return account, account.AIEnabled, nil
}

func (service Service) ensureExplicitAIProfile(ctx context.Context, account AccountRecord) error {
	accountID := strings.TrimSpace(account.AccountID)
	assigneeID := strings.TrimSpace(account.AssigneeID)
	if accountID == "" && assigneeID == "" {
		return ErrConversationAIProfileRequired
	}
	if service.AIConfigStore == nil {
		return ErrConversationAIProfileRequired
	}
	reader := aiConfigReader{ctx: ctx, store: service.AIConfigStore}
	config, err := reader.config()
	if err != nil {
		return err
	}
	if rowBool(config, "enabled") {
		local := ProjectionRow{
			"target_scope":       config["local_target_scope"],
			"target_account_ids": config["local_target_account_ids"],
			"target_audience":    config["local_target_audience"],
		}
		if aiAccountTargetMatches(local, accountID, assigneeID) {
			return nil
		}
	}
	for _, profile := range rowsFromConfig(config["coze_profiles"]) {
		if rowBool(profile, "enabled") && aiAccountTargetMatches(profile, accountID, assigneeID) {
			return nil
		}
	}
	for _, profile := range rowsFromConfig(config["xiaobei_profiles"]) {
		if rowBool(profile, "enabled") && aiAccountTargetMatches(profile, accountID, assigneeID) {
			return nil
		}
	}
	return ErrConversationAIProfileRequired
}

func aiAccountTargetMatches(profile ProjectionRow, accountID string, assigneeID string) bool {
	scope := normalizeTargetScope(profile)
	accountID = strings.TrimSpace(accountID)
	assigneeID = strings.TrimSpace(assigneeID)
	switch scope {
	case aiTargetScopeAll:
		return accountID != ""
	case aiTargetScopeAccount:
		if accountID == "" {
			return false
		}
		for _, target := range normalizeTargetAccountIDs(profile["target_account_ids"]) {
			if target == accountID {
				return true
			}
		}
		return false
	case aiTargetScopeAssignee:
		return aiTargetAudienceMatches(profile["target_audience"], assigneeID)
	default:
		return false
	}
}

func aiTargetAudienceMatches(raw any, assigneeID string) bool {
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return false
	}
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if strings.TrimSpace(stringFromAny(item)) == assigneeID {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) == assigneeID {
				return true
			}
		}
		return false
	default:
		value := strings.TrimSpace(stringFromAny(raw))
		if value == "" || value == defaultTargetAudienceNone {
			return false
		}
		if value == "__ALL__" {
			return assigneeID != ""
		}
		for _, item := range regexp.MustCompile(`[,\n，；]`).Split(value, -1) {
			if strings.TrimSpace(item) == assigneeID {
				return true
			}
		}
		return false
	}
}

func (service Service) clearAIRuntimeAfterManualEnable(ctx context.Context, store ConversationAIStore, conversation ConversationAIRecord) error {
	conversationID := strings.TrimSpace(conversation.ConversationID)
	if conversationID == "" {
		return nil
	}
	next := map[string]any{}
	for key, value := range conversation.SOPRuntimeState {
		next[key] = value
	}
	for _, key := range []string{
		"ai_reply_job_id",
		"ai_reply_status",
		"ai_reply_phase",
		"ai_reply_error",
		"ai_reply_force_manual",
		"ai_reply_processing_started_at",
		"ai_reply_preview",
		"ai_reply_message_trace_id",
		"ai_reply_task_id",
	} {
		delete(next, key)
	}
	next["handoff_status"] = "auto_active"
	next["ai_mode_override"] = "auto"
	next["ai_auto_reply"] = true
	next["sensitive_handoff_pending"] = false
	next["sensitive_handoff_reason"] = ""
	next["sensitive_handoff_at"] = ""
	next["sensitive_handoff_message_trace_id"] = ""
	return store.UpdateConversationRuntimeState(ctx, conversationID, next)
}

func (service Service) syncCSUserAIEnabled(ctx context.Context, assigneeID string, enabled bool) error {
	if service.CSUsers == nil || service.CSUserWriteStore == nil {
		return nil
	}
	user, ok, err := service.CSUserWriteStore.GetCSUser(ctx, assigneeID)
	if err != nil || !ok || user.AIEnabled == enabled {
		return err
	}
	command := CSUserCommand{
		AssigneeID:   user.AssigneeID,
		AssigneeName: user.AssigneeName,
		Role:         user.Role,
		Enabled:      user.Enabled,
		AIEnabled:    enabled,
		MaxSessions:  user.MaxSessions,
	}
	_, err = service.CSUserWriteStore.UpsertCSUser(ctx, command)
	return err
}

func aiOverrideMode(enabled bool) string {
	if enabled {
		return "auto"
	}
	return "manual"
}

func computeEffectiveConversationAI(accountAIEnabled bool, overrideMode string) bool {
	return ComputeEffectiveConversationAI(accountAIEnabled, overrideMode)
}

// ComputeEffectiveConversationAI applies the legacy override precedence.
func ComputeEffectiveConversationAI(accountAIEnabled bool, overrideMode string) bool {
	switch strings.ToLower(strings.TrimSpace(overrideMode)) {
	case "auto":
		return true
	case "manual":
		return false
	default:
		return accountAIEnabled
	}
}

func roleAllowsBulkAssignee(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "supervisor":
		return true
	default:
		return false
	}
}

func sessionTenantID(session auth.Session) string {
	if session.Claims == nil {
		return ""
	}
	for _, key := range []string{"tenant_id", "enterprise_id", "organization_name"} {
		if value := strings.TrimSpace(fmt.Sprint(session.Claims[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func conversationAIPayload(conversation ConversationAIRecord) ProjectionRow {
	return ProjectionRow{
		"conversation_id":   strings.TrimSpace(conversation.ConversationID),
		"tenant_id":         nilIfBlank(strings.TrimSpace(conversation.TenantID)),
		"account_id":        nilIfBlank(strings.TrimSpace(conversation.AccountID)),
		"ai_auto_reply":     conversation.AIAutoReply,
		"ai_mode_override":  defaultText(strings.TrimSpace(conversation.AIModeOverride), "inherit"),
		"sop_runtime_state": conversation.SOPRuntimeState,
	}
}

func conversationAIEventPayload(conversation ConversationAIRecord, accountAIEnabled bool) map[string]any {
	return map[string]any{
		"conversation_id":    strings.TrimSpace(conversation.ConversationID),
		"tenant_id":          nilIfBlank(strings.TrimSpace(conversation.TenantID)),
		"account_id":         nilIfBlank(strings.TrimSpace(conversation.AccountID)),
		"ai_auto_reply":      conversation.AIAutoReply,
		"enabled":            conversation.AIAutoReply,
		"ai_mode_override":   defaultText(strings.TrimSpace(conversation.AIModeOverride), "inherit"),
		"account_ai_enabled": accountAIEnabled,
	}
}

func runtimeStateFromJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func runtimeStateToJSON(value map[string]any) string {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
