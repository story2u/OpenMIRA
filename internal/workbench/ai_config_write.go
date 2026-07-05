package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"im-go/internal/auth"
)

var (
	ErrAIConfigBaseURLRequired = errors.New("base_url is required")
	ErrAIConfigModelRequired   = errors.New("model is required")
	ErrAIConfigTimeoutInvalid  = errors.New("timeout_sec must be > 0")
	ErrAIConfigTemperature     = errors.New("temperature must be in [0, 2]")
)

// AIConfigValidationError maps Python ValueError details to HTTP 422.
type AIConfigValidationError struct {
	Detail string
}

func (err AIConfigValidationError) Error() string {
	return err.Detail
}

// AIConfigUpdateBody is the JSON input for POST /admin/ai-config.
type AIConfigUpdateBody struct {
	Enabled                *bool                      `json:"enabled"`
	BaseURL                *string                    `json:"base_url"`
	Model                  *string                    `json:"model"`
	TimeoutSec             *float64                   `json:"timeout_sec"`
	Temperature            *float64                   `json:"temperature"`
	SystemPrompt           string                     `json:"system_prompt"`
	InterceptKeywords      string                     `json:"intercept_keywords"`
	DefaultHandoffReply    string                     `json:"default_handoff_reply"`
	LocalTargetAudience    string                     `json:"local_target_audience"`
	LocalTargetScope       string                     `json:"local_target_scope"`
	LocalTargetAccountIDs  []string                   `json:"local_target_account_ids"`
	LocalDefaultAIEnabled  bool                       `json:"local_default_ai_enabled"`
	APIKey                 *string                    `json:"api_key"`
	ActiveCozeProfileID    string                     `json:"active_coze_profile_id"`
	CozeProfiles           []CozeProfileConfigBody    `json:"coze_profiles"`
	ActiveXiaobeiProfileID string                     `json:"active_xiaobei_profile_id"`
	XiaobeiProfiles        []XiaobeiProfileConfigBody `json:"xiaobei_profiles"`
}

// CozeProfileConfigBody is one submitted Coze profile.
type CozeProfileConfigBody struct {
	ProfileID        string   `json:"profile_id"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	BaseURL          string   `json:"base_url"`
	WorkflowID       string   `json:"workflow_id"`
	Token            *string  `json:"token"`
	Enabled          *bool    `json:"enabled"`
	TargetAudience   string   `json:"target_audience"`
	TargetScope      string   `json:"target_scope"`
	TargetAccountIDs []string `json:"target_account_ids"`
	DefaultAIEnabled bool     `json:"default_ai_enabled"`
	WorkflowSchema   string   `json:"workflow_schema"`
	WorkflowVersion  string   `json:"workflow_version"`
	SpaceID          string   `json:"space_id"`
	SpaceIDAlias     string   `json:"spaceId"`
}

// XiaobeiProfileConfigBody is one submitted Xiaobei profile.
type XiaobeiProfileConfigBody struct {
	ProfileID        string   `json:"profile_id"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	BaseURL          string   `json:"base_url"`
	HealthURL        string   `json:"health_url"`
	Token            *string  `json:"token"`
	Enabled          *bool    `json:"enabled"`
	TargetAudience   string   `json:"target_audience"`
	TargetScope      string   `json:"target_scope"`
	TargetAccountIDs []string `json:"target_account_ids"`
	DefaultAIEnabled bool     `json:"default_ai_enabled"`
}

// AIConfigUpdateRequest carries the legacy POST request body.
type AIConfigUpdateRequest struct {
	Session auth.Session
	Body    AIConfigUpdateBody
}

// NewAIConfigUpdateRequest normalizes the AI config write boundary.
func NewAIConfigUpdateRequest(body AIConfigUpdateBody, session auth.Session) AIConfigUpdateRequest {
	return AIConfigUpdateRequest{Session: session, Body: body}
}

// UpdateAIConfig handles POST /api/v1/admin/ai-config.
func (service Service) UpdateAIConfig(ctx context.Context, request AIConfigUpdateRequest) (Payload, error) {
	if service.AIConfigStore == nil || service.AIConfigWriteStore == nil {
		return nil, ErrAIConfigStoreUnavailable
	}
	if service.Accounts == nil || service.accountAIWriteStore() == nil {
		return nil, ErrAccountStoreUnavailable
	}
	body := request.Body
	if body.BaseURL == nil || strings.TrimSpace(*body.BaseURL) == "" {
		return nil, ErrAIConfigBaseURLRequired
	}
	if body.Model == nil || strings.TrimSpace(*body.Model) == "" {
		return nil, ErrAIConfigModelRequired
	}
	timeoutSec := float64(20)
	if body.TimeoutSec != nil {
		timeoutSec = *body.TimeoutSec
	}
	if timeoutSec <= 0 {
		return nil, ErrAIConfigTimeoutInvalid
	}
	temperature := 0.7
	if body.Temperature != nil {
		temperature = *body.Temperature
	}
	if temperature < 0 || temperature > 2 {
		return nil, ErrAIConfigTemperature
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	reader := aiConfigReader{ctx: ctx, store: service.AIConfigStore}
	previousConfig, err := reader.config()
	if err != nil {
		return nil, err
	}
	previousCozePrivate, err := reader.privateCozeProfiles()
	if err != nil {
		return nil, err
	}
	previousXiaobeiPrivate, err := reader.privateXiaobeiProfiles()
	if err != nil {
		return nil, err
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	cozeProfilesForValidation := cozeBodyRows(body.CozeProfiles)
	if len(cozeProfilesForValidation) == 0 {
		cozeProfilesForValidation = rowsFromConfig(previousConfig["coze_profiles"])
	}
	xiaobeiProfilesForValidation := xiaobeiBodyRows(body.XiaobeiProfiles)
	if len(xiaobeiProfilesForValidation) == 0 {
		xiaobeiProfilesForValidation = rowsFromConfig(previousConfig["xiaobei_profiles"])
	}
	if err := validateAIConfigTargetConflicts(enabled, body.LocalTargetAudience, body.LocalTargetScope, body.LocalTargetAccountIDs, cozeProfilesForValidation, xiaobeiProfilesForValidation, accounts); err != nil {
		return nil, err
	}
	mergedCozeProfiles := previousCozePrivate
	if len(body.CozeProfiles) > 0 {
		mergedCozeProfiles = mergePrivateCozeProfiles(body.CozeProfiles, previousCozePrivate)
	}
	mergedXiaobeiProfiles := previousXiaobeiPrivate
	if len(body.XiaobeiProfiles) > 0 {
		mergedXiaobeiProfiles = mergePrivateXiaobeiProfiles(body.XiaobeiProfiles, previousXiaobeiPrivate)
	}
	enabled, mergedCozeProfiles, normalizedActiveCozeID := normalizeMutualAIActivation(enabled, mergedCozeProfiles, body.ActiveCozeProfileID)
	if err := validateEnabledCozeProfiles(mergedCozeProfiles); err != nil {
		return nil, err
	}
	activeXiaobeiID := strings.TrimSpace(body.ActiveXiaobeiProfileID)
	if activeXiaobeiID == "" {
		activeXiaobeiID = firstProfileID(mergedXiaobeiProfiles, defaultXiaobeiProfileID)
	}
	if err := service.writeAIConfigSettings(ctx, aiConfigWriteSettings{
		Enabled:                enabled,
		BaseURL:                strings.TrimSpace(*body.BaseURL),
		Model:                  strings.TrimSpace(*body.Model),
		TimeoutSec:             timeoutSec,
		Temperature:            temperature,
		SystemPrompt:           strings.TrimSpace(body.SystemPrompt),
		InterceptKeywords:      strings.TrimSpace(body.InterceptKeywords),
		DefaultHandoffReply:    strings.TrimSpace(body.DefaultHandoffReply),
		LocalTargetAudience:    aiNormalizeTargetAudience(body.LocalTargetAudience),
		LocalTargetScope:       normalizeTargetScope(ProjectionRow{"target_scope": body.LocalTargetScope, "target_audience": body.LocalTargetAudience, "target_account_ids": body.LocalTargetAccountIDs}),
		LocalTargetAccountIDs:  normalizeTargetAccountIDs(body.LocalTargetAccountIDs),
		LocalDefaultAIEnabled:  body.LocalDefaultAIEnabled,
		APIKey:                 body.APIKey,
		CozeProfiles:           mergedCozeProfiles,
		ActiveCozeProfileID:    normalizedActiveCozeID,
		XiaobeiProfiles:        mergedXiaobeiProfiles,
		ActiveXiaobeiProfileID: activeXiaobeiID,
	}); err != nil {
		return nil, err
	}
	config, err := reader.config()
	if err != nil {
		return nil, err
	}
	defaultSync, err := service.syncAIConfigDefaultAccountAI(ctx, config, previousConfig, accounts)
	if err != nil {
		return nil, err
	}
	if service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: fmt.Sprintf("更新AI配置: model=%s, enabled=%t", strings.TrimSpace(*body.Model), enabled)}); err != nil {
			return nil, err
		}
		if len(defaultSync.ChangedAccounts) > 0 {
			if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: defaultSync.auditDetail()}); err != nil {
				return nil, err
			}
		}
	}
	if len(defaultSync.ChangedAccounts) > 0 {
		service.invalidateAllReadModelNamespaces(ctx)
	}
	if service.AIConfigEvents != nil {
		if err := service.AIConfigEvents.Publish(ctx, "devices", "ai.config.updated", "ai.config", map[string]any(config)); err != nil {
			return nil, err
		}
	}
	return Payload{"success": true, "config": config}, nil
}

type aiConfigWriteSettings struct {
	Enabled                bool
	BaseURL                string
	Model                  string
	TimeoutSec             float64
	Temperature            float64
	SystemPrompt           string
	InterceptKeywords      string
	DefaultHandoffReply    string
	LocalTargetAudience    string
	LocalTargetScope       string
	LocalTargetAccountIDs  []string
	LocalDefaultAIEnabled  bool
	APIKey                 *string
	CozeProfiles           []ProjectionRow
	ActiveCozeProfileID    string
	XiaobeiProfiles        []ProjectionRow
	ActiveXiaobeiProfileID string
}

type aiConfigDefaultSyncSummary struct {
	ChangedAccounts   []ProjectionRow
	ConversationCount int
}

func (summary aiConfigDefaultSyncSummary) auditDetail() string {
	enabledCount := 0
	disabledCount := 0
	resetCount := 0
	accountIDs := make([]string, 0, len(summary.ChangedAccounts))
	for _, account := range summary.ChangedAccounts {
		if rowBool(account, "ai_enabled") {
			enabledCount++
		} else {
			disabledCount++
		}
		if rowBool(account, "reset_override_to_inherit") {
			resetCount++
		}
		if accountID := rowText(account, "account_id"); accountID != "" {
			accountIDs = append(accountIDs, accountID)
		}
	}
	return fmt.Sprintf(
		"AI配置默认托管同步: 开启账号 %d 个，关闭账号 %d 个，重置会话覆盖账号 %d 个，同步会话 %d 条；账号=%s",
		enabledCount,
		disabledCount,
		resetCount,
		summary.ConversationCount,
		strings.Join(accountIDs, ","),
	)
}

func (service Service) syncAIConfigDefaultAccountAI(ctx context.Context, config ProjectionRow, previousConfig ProjectionRow, accounts []AccountRecord) (aiConfigDefaultSyncSummary, error) {
	desiredByAccountID := aiConfigDefaultTargets(config, accounts)
	resetAccountIDs := aiConfigDefaultResetAccountIDs(config, previousConfig, accounts)
	for accountID := range aiConfigDefaultTargets(previousConfig, accounts) {
		if _, ok := desiredByAccountID[accountID]; !ok {
			desiredByAccountID[accountID] = false
		}
	}
	if len(desiredByAccountID) == 0 {
		return aiConfigDefaultSyncSummary{}, nil
	}
	accountByID := map[string]AccountRecord{}
	for _, account := range accounts {
		accountID := strings.TrimSpace(account.AccountID)
		if accountID != "" {
			accountByID[accountID] = account
		}
	}
	store := service.accountAIWriteStore()
	summary := aiConfigDefaultSyncSummary{}
	for accountID, desiredEnabled := range desiredByAccountID {
		current, ok := accountByID[accountID]
		if !ok {
			continue
		}
		resetOverride := resetAccountIDs[accountID]
		accountAIChanged := current.AIEnabled != desiredEnabled
		if !accountAIChanged && !resetOverride {
			continue
		}
		account := current
		if accountAIChanged {
			updated, changed, err := store.SetAccountAIEnabled(ctx, accountID, desiredEnabled)
			if err != nil {
				return aiConfigDefaultSyncSummary{}, err
			}
			if changed {
				account = updated
			}
		}
		account.AIEnabled = desiredEnabled
		syncResult, err := store.SyncAccountAIEnabled(ctx, account, desiredEnabled, resetOverride)
		if err != nil {
			return aiConfigDefaultSyncSummary{}, err
		}
		updatedCount := len(syncResult.Conversations) + len(syncResult.ProjectionOnlyConversationIDs)
		summary.ConversationCount += updatedCount
		if service.AIConfigEvents != nil {
			for _, conversation := range syncResult.Conversations {
				if strings.TrimSpace(conversation.ConversationID) == "" {
					continue
				}
				if err := service.AIConfigEvents.Publish(ctx, "conversations", "conversation.ai_auto_reply", "conversation.ai_auto_reply", accountConversationAIEventPayload(conversation, desiredEnabled)); err != nil {
					return aiConfigDefaultSyncSummary{}, err
				}
			}
			for _, conversationID := range syncResult.ProjectionOnlyConversationIDs {
				if err := service.AIConfigEvents.Publish(ctx, "conversations", "conversation.ai_auto_reply", "conversation.ai_auto_reply", map[string]any{
					"conversation_id":    conversationID,
					"tenant_id":          nil,
					"account_id":         strings.TrimSpace(account.AccountID),
					"ai_auto_reply":      desiredEnabled,
					"ai_mode_override":   "inherit",
					"account_ai_enabled": desiredEnabled,
					"enabled":            desiredEnabled,
				}); err != nil {
					return aiConfigDefaultSyncSummary{}, err
				}
			}
			if err := service.AIConfigEvents.Publish(ctx, "devices", "account.updated", "account.changed", map[string]any(accountRecordFullPayload(account))); err != nil {
				return aiConfigDefaultSyncSummary{}, err
			}
		}
		summary.ChangedAccounts = append(summary.ChangedAccounts, ProjectionRow{
			"account_id":                       strings.TrimSpace(account.AccountID),
			"account_name":                     strings.TrimSpace(account.AccountName),
			"ai_enabled":                       desiredEnabled,
			"reset_override_to_inherit":        resetOverride,
			"updated_conversations":            updatedCount,
			"projection_alias_conversations":   len(syncResult.ProjectionAliasConversationIDs),
			"projection_only_conversations":    len(syncResult.ProjectionOnlyConversationIDs),
			"main_conversation_sync_completed": len(syncResult.Conversations),
		})
	}
	return summary, nil
}

func aiConfigDefaultTargets(config ProjectionRow, accounts []AccountRecord) map[string]bool {
	desired := map[string]bool{}
	localProfile := ProjectionRow{
		"enabled":            rowBool(config, "enabled"),
		"target_scope":       config["local_target_scope"],
		"target_account_ids": config["local_target_account_ids"],
		"target_audience":    config["local_target_audience"],
		"default_ai_enabled": rowBool(config, "local_default_ai_enabled"),
	}
	if rowBool(localProfile, "enabled") {
		for _, accountID := range accountIDsForAIProfile(localProfile, accounts) {
			desired[accountID] = rowBool(localProfile, "default_ai_enabled")
		}
	}
	for _, profile := range rowsFromConfig(config["coze_profiles"]) {
		if !rowBool(profile, "enabled") {
			continue
		}
		for _, accountID := range accountIDsForAIProfile(profile, accounts) {
			desired[accountID] = rowBool(profile, "default_ai_enabled")
		}
	}
	for _, profile := range rowsFromConfig(config["xiaobei_profiles"]) {
		if !rowBool(profile, "enabled") {
			continue
		}
		for _, accountID := range accountIDsForAIProfile(profile, accounts) {
			desired[accountID] = rowBool(profile, "default_ai_enabled")
		}
	}
	return desired
}

func aiConfigDefaultResetAccountIDs(config ProjectionRow, previousConfig ProjectionRow, accounts []AccountRecord) map[string]bool {
	currentStates := aiConfigDefaultStates(config, accounts)
	previousStates := aiConfigDefaultStates(previousConfig, accounts)
	out := map[string]bool{}
	for accountID, current := range currentStates {
		if previousStates[accountID] != current {
			out[accountID] = true
		}
	}
	for accountID, previous := range previousStates {
		if currentStates[accountID] != previous {
			out[accountID] = true
		}
	}
	return out
}

func aiConfigDefaultStates(config ProjectionRow, accounts []AccountRecord) map[string]string {
	states := map[string]string{}
	localProfile := ProjectionRow{
		"enabled":            rowBool(config, "enabled"),
		"target_scope":       config["local_target_scope"],
		"target_account_ids": config["local_target_account_ids"],
		"target_audience":    config["local_target_audience"],
		"default_ai_enabled": rowBool(config, "local_default_ai_enabled"),
	}
	if rowBool(localProfile, "enabled") {
		for _, accountID := range accountIDsForAIProfile(localProfile, accounts) {
			states[accountID] = fmt.Sprintf("local:%t", rowBool(localProfile, "default_ai_enabled"))
		}
	}
	for index, profile := range rowsFromConfig(config["coze_profiles"]) {
		if !rowBool(profile, "enabled") {
			continue
		}
		profileID := firstNonBlank(rowText(profile, "profile_id"), rowText(profile, "id"), fmt.Sprint(index))
		for _, accountID := range accountIDsForAIProfile(profile, accounts) {
			states[accountID] = fmt.Sprintf("coze_profiles:%s:%t", profileID, rowBool(profile, "default_ai_enabled"))
		}
	}
	for index, profile := range rowsFromConfig(config["xiaobei_profiles"]) {
		if !rowBool(profile, "enabled") {
			continue
		}
		profileID := firstNonBlank(rowText(profile, "profile_id"), rowText(profile, "id"), fmt.Sprint(index))
		for _, accountID := range accountIDsForAIProfile(profile, accounts) {
			states[accountID] = fmt.Sprintf("xiaobei_profiles:%s:%t", profileID, rowBool(profile, "default_ai_enabled"))
		}
	}
	return states
}

func accountIDsForAIProfile(profile ProjectionRow, accounts []AccountRecord) []string {
	scope := normalizeTargetScope(profile)
	if scope == aiTargetScopeAll {
		ids := make([]string, 0, len(accounts))
		for _, account := range accounts {
			if accountID := strings.TrimSpace(account.AccountID); accountID != "" {
				ids = append(ids, accountID)
			}
		}
		return ids
	}
	if scope == aiTargetScopeAccount {
		return normalizeTargetAccountIDs(profile["target_account_ids"])
	}
	return []string{}
}

func (service Service) writeAIConfigSettings(ctx context.Context, settings aiConfigWriteSettings) error {
	values := []struct {
		key   string
		value string
	}{
		{"ai.base_url", settings.BaseURL},
		{"ai.model", settings.Model},
		{"ai.timeout_sec", formatFloat(settings.TimeoutSec)},
		{"ai.temperature", formatFloat(settings.Temperature)},
		{"ai.system_prompt", settings.SystemPrompt},
		{"ai.intercept_keywords", settings.InterceptKeywords},
		{"ai.default_handoff_reply", settings.DefaultHandoffReply},
		{"ai.local_target_audience", settings.LocalTargetAudience},
		{"ai.local_target_scope", settings.LocalTargetScope},
		{"ai.local_target_account_ids", mustJSON(settings.LocalTargetAccountIDs)},
		{"ai.local_default_ai_enabled", boolSetting(settings.LocalDefaultAIEnabled)},
		{"ai.enabled", boolSetting(settings.Enabled)},
		{"ai.coze_profiles", mustJSON(settings.CozeProfiles)},
		{"ai.active_coze_profile_id", strings.TrimSpace(settings.ActiveCozeProfileID)},
		{"ai.xiaobei_profiles", mustJSON(settings.XiaobeiProfiles)},
		{"ai.active_xiaobei_profile_id", strings.TrimSpace(settings.ActiveXiaobeiProfileID)},
	}
	if settings.APIKey != nil {
		values = append(values[:11], append([]struct {
			key   string
			value string
		}{{"ai.api_key", strings.TrimSpace(*settings.APIKey)}}, values[11:]...)...)
	}
	for _, item := range values {
		if err := service.AIConfigWriteStore.SetAIConfigValue(ctx, item.key, item.value); err != nil {
			return err
		}
	}
	return nil
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func boolSetting(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mergePrivateCozeProfiles(input []CozeProfileConfigBody, previous []ProjectionRow) []ProjectionRow {
	previousByID := previousProfilesByID(previous)
	merged := make([]ProjectionRow, 0, len(input))
	seen := map[string]bool{}
	for index, item := range input {
		profileID := cozeProfileID(cozeBodyRow(item), index)
		if profileID == "" || seen[profileID] {
			continue
		}
		seen[profileID] = true
		previousItem := previousByID[profileID]
		token := strings.TrimSpace(stringPtrValue(item.Token))
		if token == "" {
			token = strings.TrimSpace(stringFromAny(previousItem["token"]))
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		merged = append(merged, ProjectionRow{
			"profile_id":         profileID,
			"name":               defaultText(strings.TrimSpace(item.Name), "Coze 配置 "+fmt.Sprint(index+1)),
			"base_url":           defaultText(strings.TrimSpace(item.BaseURL), defaultCozeBaseURL),
			"workflow_id":        strings.TrimSpace(item.WorkflowID),
			"token":              token,
			"enabled":            enabled,
			"target_audience":    aiNormalizeTargetAudience(item.TargetAudience),
			"target_scope":       normalizeTargetScope(cozeBodyRow(item)),
			"target_account_ids": normalizeTargetAccountIDs(item.TargetAccountIDs),
			"default_ai_enabled": item.DefaultAIEnabled,
			"workflow_schema":    normalizeWorkflowSchema(firstNonBlank(item.WorkflowSchema, item.WorkflowVersion, stringFromAny(previousItem["workflow_schema"]), stringFromAny(previousItem["workflow_version"]))),
			"space_id":           firstNonBlank(strings.TrimSpace(item.SpaceID), strings.TrimSpace(item.SpaceIDAlias), stringFromAny(previousItem["space_id"])),
		})
	}
	if len(merged) > 0 {
		return merged
	}
	return previous
}

func mergePrivateXiaobeiProfiles(input []XiaobeiProfileConfigBody, previous []ProjectionRow) []ProjectionRow {
	previousByID := previousProfilesByID(previous)
	merged := make([]ProjectionRow, 0, len(input))
	seen := map[string]bool{}
	for index, item := range input {
		profileID := xiaobeiProfileID(xiaobeiBodyRow(item), index)
		if profileID == "" || seen[profileID] {
			continue
		}
		seen[profileID] = true
		previousItem := previousByID[profileID]
		token := strings.TrimSpace(stringPtrValue(item.Token))
		if token == "" {
			token = strings.TrimSpace(stringFromAny(previousItem["token"]))
		}
		enabled := false
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		merged = append(merged, ProjectionRow{
			"profile_id":         profileID,
			"name":               defaultText(strings.TrimSpace(item.Name), "小贝配置 "+fmt.Sprint(index+1)),
			"base_url":           defaultText(strings.TrimSpace(item.BaseURL), defaultXiaobeiBaseURL),
			"health_url":         defaultText(strings.TrimSpace(item.HealthURL), defaultXiaobeiHealthURL),
			"token":              token,
			"enabled":            enabled,
			"target_audience":    aiNormalizeTargetAudience(item.TargetAudience),
			"target_scope":       normalizeTargetScope(xiaobeiBodyRow(item)),
			"target_account_ids": normalizeTargetAccountIDs(item.TargetAccountIDs),
			"default_ai_enabled": item.DefaultAIEnabled,
		})
	}
	if len(merged) > 0 {
		return merged
	}
	return previous
}

func previousProfilesByID(profiles []ProjectionRow) map[string]ProjectionRow {
	out := map[string]ProjectionRow{}
	for _, profile := range profiles {
		profileID := firstNonBlank(stringFromAny(profile["profile_id"]), stringFromAny(profile["id"]))
		if profileID != "" {
			out[profileID] = profile
		}
	}
	return out
}

func validateEnabledCozeProfiles(profiles []ProjectionRow) error {
	for index, profile := range profiles {
		if !boolValue(profile, "enabled", true) {
			continue
		}
		missing := make([]string, 0)
		if strings.TrimSpace(stringFromAny(profile["base_url"])) == "" {
			missing = append(missing, "接口地址")
		}
		if strings.TrimSpace(stringFromAny(profile["workflow_id"])) == "" {
			missing = append(missing, "工作流ID")
		}
		if !cozeRuntimeTokenSet(profile) {
			missing = append(missing, "Token")
		}
		if len(missing) > 0 {
			profileName := defaultText(stringFromAny(profile["name"]), cozeProfileID(profile, index))
			return AIConfigValidationError{Detail: fmt.Sprintf("%s 配置不完整：请先配置%s", profileName, strings.Join(missing, ", "))}
		}
	}
	return nil
}

func cozeBodyRows(items []CozeProfileConfigBody) []ProjectionRow {
	rows := make([]ProjectionRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, cozeBodyRow(item))
	}
	return rows
}

func cozeBodyRow(item CozeProfileConfigBody) ProjectionRow {
	enabled := true
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	return ProjectionRow{
		"profile_id":         strings.TrimSpace(item.ProfileID),
		"id":                 strings.TrimSpace(item.ID),
		"name":               strings.TrimSpace(item.Name),
		"base_url":           strings.TrimSpace(item.BaseURL),
		"workflow_id":        strings.TrimSpace(item.WorkflowID),
		"token":              stringPtrValue(item.Token),
		"enabled":            enabled,
		"target_audience":    strings.TrimSpace(item.TargetAudience),
		"target_scope":       strings.TrimSpace(item.TargetScope),
		"target_account_ids": item.TargetAccountIDs,
		"default_ai_enabled": item.DefaultAIEnabled,
		"workflow_schema":    strings.TrimSpace(item.WorkflowSchema),
		"workflow_version":   strings.TrimSpace(item.WorkflowVersion),
		"space_id":           strings.TrimSpace(firstNonBlank(item.SpaceID, item.SpaceIDAlias)),
	}
}

func xiaobeiBodyRows(items []XiaobeiProfileConfigBody) []ProjectionRow {
	rows := make([]ProjectionRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, xiaobeiBodyRow(item))
	}
	return rows
}

func xiaobeiBodyRow(item XiaobeiProfileConfigBody) ProjectionRow {
	enabled := false
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	return ProjectionRow{
		"profile_id":         strings.TrimSpace(item.ProfileID),
		"id":                 strings.TrimSpace(item.ID),
		"name":               strings.TrimSpace(item.Name),
		"base_url":           strings.TrimSpace(item.BaseURL),
		"health_url":         strings.TrimSpace(item.HealthURL),
		"token":              stringPtrValue(item.Token),
		"enabled":            enabled,
		"target_audience":    strings.TrimSpace(item.TargetAudience),
		"target_scope":       strings.TrimSpace(item.TargetScope),
		"target_account_ids": item.TargetAccountIDs,
		"default_ai_enabled": item.DefaultAIEnabled,
	}
}

func rowsFromConfig(value any) []ProjectionRow {
	switch typed := value.(type) {
	case []ProjectionRow:
		return typed
	default:
		return []ProjectionRow{}
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func aiNormalizeTargetAudience(value string) string {
	normalized := strings.NewReplacer("\n", ",", "，", ",", "；", ",").Replace(strings.TrimSpace(value))
	if normalized == defaultTargetAudienceAll || normalized == defaultTargetAudienceNone {
		return normalized
	}
	items := strings.Split(normalized, ",")
	values := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		candidate := strings.TrimSpace(item)
		if candidate == "" || candidate == defaultTargetAudienceAll || candidate == defaultTargetAudienceNone || seen[candidate] {
			continue
		}
		seen[candidate] = true
		values = append(values, candidate)
	}
	if len(values) == 0 {
		return defaultTargetAudienceNone
	}
	return strings.Join(values, ",")
}

func validateAIConfigTargetConflicts(enabled bool, localAudience string, localScope string, localAccountIDs []string, cozeProfiles []ProjectionRow, xiaobeiProfiles []ProjectionRow, accounts []AccountRecord) error {
	entries := make([]namedAudienceSelection, 0)
	accountEntries := make([]namedAccountSelection, 0)
	normalizedLocalScope := normalizeTargetScope(ProjectionRow{"target_scope": localScope, "target_audience": localAudience, "target_account_ids": localAccountIDs})
	if enabled && normalizedLocalScope == aiTargetScopeAssignee {
		entries = append(entries, namedAudienceSelection{name: "本地提示词配置", selection: parseTargetAudienceSelection(localAudience)})
	}
	if enabled && (normalizedLocalScope == aiTargetScopeAccount || normalizedLocalScope == aiTargetScopeAll) {
		accountEntries = append(accountEntries, namedAccountSelection{name: "本地提示词配置", selection: parseAccountTargetSelection(ProjectionRow{"target_scope": normalizedLocalScope, "target_account_ids": localAccountIDs, "target_audience": localAudience}, accounts)})
	}
	for index, profile := range cozeProfiles {
		if !boolValue(profile, "enabled", true) {
			continue
		}
		appendAIConfigConflictEntry(profileDisplayName(profile, "Coze配置 "+fmt.Sprint(index+1)), profile, accounts, &entries, &accountEntries)
	}
	for index, profile := range xiaobeiProfiles {
		if !boolValue(profile, "enabled", false) {
			continue
		}
		appendAIConfigConflictEntry(profileDisplayName(profile, "小贝配置 "+fmt.Sprint(index+1)), profile, accounts, &entries, &accountEntries)
	}
	for index, current := range entries {
		for _, previous := range entries[:index] {
			if conflict := targetAudienceConflictLabel(current.selection, previous.selection); conflict != "" {
				return AIConfigValidationError{Detail: fmt.Sprintf("一个消息端只能分配一个 AI 自动回复逻辑：%s 已配置在 %s，不能再配置到 %s", conflict, previous.name, current.name)}
			}
		}
	}
	for index, current := range accountEntries {
		for _, previous := range accountEntries[:index] {
			if conflict := accountTargetConflictLabel(current.selection, previous.selection); conflict != "" {
				return AIConfigValidationError{Detail: fmt.Sprintf("一个企微账号只能分配一个 AI 自动回复逻辑：%s 已配置在 %s，不能再配置到 %s", conflict, previous.name, current.name)}
			}
		}
	}
	return nil
}

type namedAudienceSelection struct {
	name      string
	selection audienceSelection
}

type audienceSelection struct {
	mode string
	ids  map[string]bool
}

type namedAccountSelection struct {
	name      string
	selection accountSelection
}

type accountSelection struct {
	mode       string
	keys       map[string]bool
	visibleIDs []string
}

func appendAIConfigConflictEntry(name string, profile ProjectionRow, accounts []AccountRecord, entries *[]namedAudienceSelection, accountEntries *[]namedAccountSelection) {
	scope := normalizeTargetScope(profile)
	switch scope {
	case aiTargetScopeAssignee:
		*entries = append(*entries, namedAudienceSelection{name: name, selection: parseTargetAudienceSelection(stringFromAny(profile["target_audience"]))})
	case aiTargetScopeAccount, aiTargetScopeAll:
		*accountEntries = append(*accountEntries, namedAccountSelection{name: name, selection: parseAccountTargetSelection(profile, accounts)})
	}
}

func profileDisplayName(profile ProjectionRow, fallback string) string {
	return defaultText(firstNonBlank(stringFromAny(profile["name"]), stringFromAny(profile["profile_id"]), stringFromAny(profile["id"])), fallback)
}

func parseTargetAudienceSelection(raw string) audienceSelection {
	normalized := aiNormalizeTargetAudience(raw)
	if normalized == defaultTargetAudienceAll {
		return audienceSelection{mode: "all", ids: map[string]bool{}}
	}
	if normalized == defaultTargetAudienceNone {
		return audienceSelection{mode: "none", ids: map[string]bool{}}
	}
	ids := map[string]bool{}
	for _, item := range strings.Split(normalized, ",") {
		if candidate := strings.TrimSpace(item); candidate != "" {
			ids[candidate] = true
		}
	}
	return audienceSelection{mode: "specific", ids: ids}
}

func parseAccountTargetSelection(profile ProjectionRow, accounts []AccountRecord) accountSelection {
	accountIDs := normalizeTargetAccountIDs(profile["target_account_ids"])
	scope := normalizeTargetScope(ProjectionRow{"target_scope": profile["target_scope"], "target_audience": profile["target_audience"], "target_account_ids": accountIDs})
	allKeys, aliasMap := accountTargetAliasMap(accounts)
	if scope == aiTargetScopeAll {
		return accountSelection{mode: "all", keys: allKeys}
	}
	if scope != aiTargetScopeAccount {
		return accountSelection{mode: "none", keys: map[string]bool{}}
	}
	keys := map[string]bool{}
	visibleIDs := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		visibleIDs = append(visibleIDs, accountID)
		if aliases, ok := aliasMap[accountID]; ok {
			for alias := range aliases {
				keys[alias] = true
			}
		} else {
			keys["account:"+accountID] = true
		}
	}
	if len(keys) == 0 {
		return accountSelection{mode: "none", keys: map[string]bool{}}
	}
	return accountSelection{mode: "specific", keys: keys, visibleIDs: visibleIDs}
}

func accountTargetAliasMap(accounts []AccountRecord) (map[string]bool, map[string]map[string]bool) {
	allKeys := map[string]bool{}
	byAccountID := map[string]map[string]bool{}
	for _, account := range accounts {
		accountID := strings.TrimSpace(account.AccountID)
		if accountID == "" {
			continue
		}
		keys := map[string]bool{"account:" + accountID: true}
		weworkUserID := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(account.WeWorkUserID), "-", ""))
		if weworkUserID != "" {
			keys["wework:"+weworkUserID] = true
		}
		byAccountID[accountID] = keys
		for key := range keys {
			allKeys[key] = true
		}
	}
	return allKeys, byAccountID
}

func targetAudienceConflictLabel(left audienceSelection, right audienceSelection) string {
	if left.mode == "none" || right.mode == "none" {
		return ""
	}
	if left.mode == "all" || right.mode == "all" {
		return "全部消息端范围"
	}
	overlap := sortedIntersection(left.ids, right.ids)
	if len(overlap) == 0 {
		return ""
	}
	return "消息端 " + strings.Join(overlap, "、")
}

func accountTargetConflictLabel(left accountSelection, right accountSelection) string {
	if left.mode == "none" || right.mode == "none" {
		return ""
	}
	if left.mode == "all" || right.mode == "all" {
		return "全部企微账号范围"
	}
	overlap := sortedIntersection(left.keys, right.keys)
	if len(overlap) == 0 {
		return ""
	}
	visibleOverlap := sortedStringSliceIntersection(left.visibleIDs, right.visibleIDs)
	if len(visibleOverlap) > 0 {
		return "企微账号 " + strings.Join(visibleOverlap, "、")
	}
	return "相同企微账号"
}

func sortedIntersection(left map[string]bool, right map[string]bool) []string {
	out := make([]string, 0)
	for value := range left {
		if right[value] {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedStringSliceIntersection(left []string, right []string) []string {
	rightSet := map[string]bool{}
	for _, item := range right {
		if normalized := strings.TrimSpace(item); normalized != "" {
			rightSet[normalized] = true
		}
	}
	outSet := map[string]bool{}
	for _, item := range left {
		normalized := strings.TrimSpace(item)
		if normalized != "" && rightSet[normalized] {
			outSet[normalized] = true
		}
	}
	out := make([]string, 0, len(outSet))
	for item := range outSet {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
