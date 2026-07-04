// AI config builds the admin read-only view of system_settings based AI
// provider settings. It masks secrets and does not persist Python's default
// profile seed values while serving the candidate route.
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"wework-go/internal/auth"
)

const (
	defaultCozeProfileID       = "smart-customer-service-coze"
	defaultCozeProfileName     = "智能客服Coze"
	defaultCozeBaseURL         = "https://api.coze.cn/v1/workflow/run"
	defaultCozeWorkflowID      = "7602085515101618212"
	defaultCozeV2ProfileID     = "smart-customer-service-coze-main-v2"
	defaultCozeV2ProfileName   = "智能客服Coze 2.0 main"
	defaultCozeV2WorkflowID    = "7640110586689732618"
	defaultCozeV2SpaceID       = "7594357340615589915"
	cozeWorkflowSchemaLegacyV1 = "legacy_v1"
	cozeWorkflowSchemaMainV2   = "main_v2"
	defaultTargetAudienceNone  = "__NONE__"
	cozeV2SeededKey            = "ai.coze_v2_profile_seeded"
	defaultXiaobeiProfileID    = "xiaobei-default"
	defaultXiaobeiProfileName  = "小贝客服"
	defaultXiaobeiBaseURL      = "http://47.252.81.104/api/ai/chat"
	defaultXiaobeiHealthURL    = "http://47.252.81.104/api/ai/health"
	aiTargetScopeAssignee      = "assignee"
	aiTargetScopeAccount       = "account"
	aiTargetScopeAll           = "all"
	aiTargetScopeNone          = "none"
)

var (
	// ErrAIConfigStoreUnavailable means AI settings cannot be loaded.
	ErrAIConfigStoreUnavailable = errors.New("workbench ai config store is unavailable")
)

// AIConfigRequest carries the authenticated management session.
type AIConfigRequest struct {
	Session auth.Session
}

// NewAIConfigRequest normalizes the AI config request boundary.
func NewAIConfigRequest(session auth.Session) AIConfigRequest {
	return AIConfigRequest{Session: session}
}

// AIConfig builds the read-only /api/v1/admin/ai-config payload.
func (service Service) AIConfig(ctx context.Context, request AIConfigRequest) (Payload, error) {
	if service.AIConfigStore == nil {
		return nil, ErrAIConfigStoreUnavailable
	}
	reader := aiConfigReader{ctx: ctx, store: service.AIConfigStore}
	config, err := reader.config()
	if err != nil {
		return nil, err
	}
	return Payload{"config": config}, nil
}

type aiConfigReader struct {
	ctx   context.Context
	store AIConfigStore
}

func (reader aiConfigReader) config() (ProjectionRow, error) {
	enabled, err := reader.boolSetting("ai.enabled", "true")
	if err != nil {
		return nil, err
	}
	baseURL, err := reader.setting("ai.base_url", envString("AI_BASE_URL", "https://api.deepseek.com/v1"))
	if err != nil {
		return nil, err
	}
	model, err := reader.setting("ai.model", envString("AI_MODEL", "deepseek-chat"))
	if err != nil {
		return nil, err
	}
	timeoutSec, err := reader.floatSetting("ai.timeout_sec", envString("AI_TIMEOUT_SEC", "20"), 20)
	if err != nil {
		return nil, err
	}
	temperature, err := reader.floatSetting("ai.temperature", "0.7", 0.7)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := reader.setting("ai.system_prompt", "你是企微客服助手，请使用专业、友好、清晰的中文回复客户。先准确理解问题，再给出可执行建议；不夸大、不承诺无法保证的结果，必要时引导转人工跟进。")
	if err != nil {
		return nil, err
	}
	interceptKeywords, err := reader.setting("ai.intercept_keywords", "")
	if err != nil {
		return nil, err
	}
	defaultHandoffReply, err := reader.setting("ai.default_handoff_reply", "我先帮您把问题记录下来，这类情况需要由顾问/医生助理进一步确认，稍后会尽快与您跟进。")
	if err != nil {
		return nil, err
	}
	localTargetAudience, err := reader.setting("ai.local_target_audience", defaultTargetAudienceNone)
	if err != nil {
		return nil, err
	}
	localTargetScope, err := reader.setting("ai.local_target_scope", aiTargetScopeAssignee)
	if err != nil {
		return nil, err
	}
	localTargetAccountIDs, err := reader.jsonListSetting("ai.local_target_account_ids")
	if err != nil {
		return nil, err
	}
	localDefaultAIEnabled, err := reader.boolSetting("ai.local_default_ai_enabled", "false")
	if err != nil {
		return nil, err
	}
	apiKeyFromDB, err := reader.setting("ai.api_key", "")
	if err != nil {
		return nil, err
	}
	privateCozeProfiles, err := reader.privateCozeProfiles()
	if err != nil {
		return nil, err
	}
	cozeProfiles := publicCozeProfiles(privateCozeProfiles)
	activeCozeProfileID, err := reader.setting("ai.active_coze_profile_id", firstProfileID(cozeProfiles, defaultCozeProfileID))
	if err != nil {
		return nil, err
	}
	enabled, cozeProfiles, activeCozeProfileID = normalizeMutualAIActivation(enabled, cozeProfiles, activeCozeProfileID)
	privateXiaobeiProfiles, err := reader.privateXiaobeiProfiles()
	if err != nil {
		return nil, err
	}
	xiaobeiProfiles := publicXiaobeiProfiles(privateXiaobeiProfiles)
	activeXiaobeiProfileID, err := reader.setting("ai.active_xiaobei_profile_id", firstProfileID(xiaobeiProfiles, defaultXiaobeiProfileID))
	if err != nil {
		return nil, err
	}
	return ProjectionRow{
		"enabled":                   enabled,
		"base_url":                  baseURL,
		"model":                     model,
		"timeout_sec":               timeoutSec,
		"temperature":               temperature,
		"system_prompt":             systemPrompt,
		"intercept_keywords":        interceptKeywords,
		"default_handoff_reply":     defaultHandoffReply,
		"local_target_audience":     normalizedTargetAudience(localTargetAudience),
		"local_target_scope":        normalizeTargetScope(ProjectionRow{"target_scope": localTargetScope, "target_audience": localTargetAudience, "target_account_ids": localTargetAccountIDs}),
		"local_target_account_ids":  localTargetAccountIDs,
		"local_default_ai_enabled":  localDefaultAIEnabled,
		"api_key_set":               strings.TrimSpace(apiKeyFromDB) != "" || strings.TrimSpace(os.Getenv("AI_API_KEY")) != "",
		"provider_hint":             envString("AI_PROVIDER", "openai-compatible"),
		"active_coze_profile_id":    activeCozeProfileID,
		"coze_profiles":             cozeProfiles,
		"active_xiaobei_profile_id": activeXiaobeiProfileID,
		"xiaobei_profiles":          xiaobeiProfiles,
	}, nil
}

func (reader aiConfigReader) setting(key string, fallback string) (string, error) {
	value, err := reader.store.GetAIConfigValue(reader.ctx, key)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	return trimmed, nil
}

func (reader aiConfigReader) boolSetting(key string, fallback string) (bool, error) {
	value, err := reader.setting(key, fallback)
	if err != nil {
		return false, err
	}
	return stringBool(value), nil
}

func (reader aiConfigReader) floatSetting(key string, fallback string, parsedFallback float64) (float64, error) {
	value, err := reader.setting(key, fallback)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return parsedFallback, nil
	}
	return parsed, nil
}

func (reader aiConfigReader) jsonListSetting(key string) ([]string, error) {
	raw, err := reader.setting(key, "[]")
	if err != nil {
		return nil, err
	}
	var items []any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}, nil
	}
	return normalizeTargetAccountIDs(items), nil
}

func (reader aiConfigReader) privateCozeProfiles() ([]ProjectionRow, error) {
	raw, err := reader.setting("ai.coze_profiles", "[]")
	if err != nil {
		return nil, err
	}
	profiles := parseJSONRows(raw)
	if len(profiles) == 0 {
		profiles = []ProjectionRow{defaultPrivateCozeProfile()}
	}
	seeded, err := reader.boolSetting(cozeV2SeededKey, "false")
	if err != nil {
		return nil, err
	}
	if !seeded && !hasCozeV2Profile(profiles) {
		profiles = append(profiles, defaultPrivateCozeV2Profile())
	}
	return profiles, nil
}

func (reader aiConfigReader) privateXiaobeiProfiles() ([]ProjectionRow, error) {
	raw, err := reader.setting("ai.xiaobei_profiles", "[]")
	if err != nil {
		return nil, err
	}
	profiles := parseJSONRows(raw)
	if len(profiles) == 0 {
		return []ProjectionRow{defaultPrivateXiaobeiProfile()}, nil
	}
	return profiles, nil
}

func parseJSONRows(raw string) []ProjectionRow {
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	rows := make([]ProjectionRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		row := ProjectionRow{}
		for key, value := range item {
			row[key] = value
		}
		rows = append(rows, row)
	}
	return rows
}
