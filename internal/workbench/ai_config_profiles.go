// AI config profile helpers mirror Python's public profile serialization while
// keeping secret tokens masked as booleans. They are shared by the read-only
// admin AI config candidate and its focused unit tests.
package workbench

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func defaultPrivateCozeProfile() ProjectionRow {
	return ProjectionRow{
		"profile_id":         defaultCozeProfileID,
		"name":               defaultCozeProfileName,
		"base_url":           envString("COZE_WORKFLOW_BASE_URL", defaultCozeBaseURL),
		"workflow_id":        envString("COZE_WORKFLOW_ID", defaultCozeWorkflowID),
		"token":              firstEnv("COZE_WORKFLOW_API_KEY", "COZE_API_KEY"),
		"enabled":            true,
		"target_audience":    defaultTargetAudienceNone,
		"target_scope":       aiTargetScopeNone,
		"target_account_ids": []string{},
		"default_ai_enabled": false,
		"workflow_schema":    cozeWorkflowSchemaLegacyV1,
		"space_id":           "",
	}
}

func defaultPrivateCozeV2Profile() ProjectionRow {
	return ProjectionRow{
		"profile_id":         defaultCozeV2ProfileID,
		"name":               defaultCozeV2ProfileName,
		"base_url":           envString("COZE_V2_WORKFLOW_BASE_URL", defaultCozeBaseURL),
		"workflow_id":        envString("COZE_V2_WORKFLOW_ID", defaultCozeV2WorkflowID),
		"token":              strings.TrimSpace(os.Getenv("COZE_V2_WORKFLOW_API_KEY")),
		"enabled":            false,
		"target_audience":    defaultTargetAudienceNone,
		"target_scope":       aiTargetScopeNone,
		"target_account_ids": []string{},
		"default_ai_enabled": false,
		"workflow_schema":    cozeWorkflowSchemaMainV2,
		"space_id":           defaultCozeV2SpaceID,
	}
}

func defaultPrivateXiaobeiProfile() ProjectionRow {
	return ProjectionRow{
		"profile_id":         defaultXiaobeiProfileID,
		"name":               defaultXiaobeiProfileName,
		"base_url":           envString("XIAOBEI_AI_CHAT_URL", defaultXiaobeiBaseURL),
		"health_url":         envString("XIAOBEI_AI_HEALTH_URL", defaultXiaobeiHealthURL),
		"token":              firstEnv("XIAOBEI_AI_API_KEY", "AI_EXTERNAL_API_KEY"),
		"enabled":            false,
		"target_audience":    defaultTargetAudienceNone,
		"target_scope":       aiTargetScopeNone,
		"target_account_ids": []string{},
		"default_ai_enabled": false,
	}
}

func publicCozeProfiles(profiles []ProjectionRow) []ProjectionRow {
	if len(profiles) == 0 {
		profiles = []ProjectionRow{defaultPrivateCozeProfile(), defaultPrivateCozeV2Profile()}
	}
	public := make([]ProjectionRow, 0, len(profiles))
	for index, item := range profiles {
		public = append(public, ProjectionRow{
			"profile_id":         cozeProfileID(item, index),
			"name":               stringValue(item, "name", "Coze 配置 "+strconv.Itoa(index+1)),
			"base_url":           stringValue(item, "base_url", defaultCozeBaseURL),
			"workflow_id":        stringValue(item, "workflow_id", ""),
			"token_set":          cozeRuntimeTokenSet(item),
			"enabled":            boolValue(item, "enabled", true),
			"target_audience":    normalizedTargetAudience(stringValue(item, "target_audience", defaultTargetAudienceNone)),
			"target_scope":       normalizeTargetScope(item),
			"target_account_ids": normalizeTargetAccountIDs(item["target_account_ids"]),
			"default_ai_enabled": boolValue(item, "default_ai_enabled", false),
			"workflow_schema":    normalizeWorkflowSchema(firstNonBlank(stringFromAny(item["workflow_schema"]), stringFromAny(item["workflow_version"]))),
			"space_id":           firstNonBlank(stringFromAny(item["space_id"]), stringFromAny(item["spaceId"])),
		})
	}
	return public
}

func publicXiaobeiProfiles(profiles []ProjectionRow) []ProjectionRow {
	if len(profiles) == 0 {
		profiles = []ProjectionRow{defaultPrivateXiaobeiProfile()}
	}
	public := make([]ProjectionRow, 0, len(profiles))
	for index, item := range profiles {
		public = append(public, ProjectionRow{
			"profile_id":         xiaobeiProfileID(item, index),
			"name":               stringValue(item, "name", "小贝配置 "+strconv.Itoa(index+1)),
			"base_url":           stringValue(item, "base_url", defaultXiaobeiBaseURL),
			"health_url":         stringValue(item, "health_url", defaultXiaobeiHealthURL),
			"token_set":          xiaobeiRuntimeTokenSet(item),
			"enabled":            boolValue(item, "enabled", false),
			"target_audience":    normalizedTargetAudience(stringValue(item, "target_audience", defaultTargetAudienceNone)),
			"target_scope":       normalizeTargetScope(item),
			"target_account_ids": normalizeTargetAccountIDs(item["target_account_ids"]),
			"default_ai_enabled": boolValue(item, "default_ai_enabled", false),
		})
	}
	return public
}

func normalizeMutualAIActivation(enabled bool, profiles []ProjectionRow, activeID string) (bool, []ProjectionRow, string) {
	profileIDs := make([]string, 0, len(profiles))
	profileSet := map[string]bool{}
	for index, item := range profiles {
		profileID := cozeProfileID(item, index)
		item["profile_id"] = profileID
		if _, ok := item["enabled"]; !ok {
			item["enabled"] = true
		}
		profileIDs = append(profileIDs, profileID)
		profileSet[profileID] = true
	}
	normalizedActiveID := strings.TrimSpace(activeID)
	if profileSet[normalizedActiveID] {
		return enabled, profiles, normalizedActiveID
	}
	for _, item := range profiles {
		if boolValue(item, "enabled", true) {
			return enabled, profiles, stringFromAny(item["profile_id"])
		}
	}
	if len(profileIDs) > 0 {
		return enabled, profiles, profileIDs[0]
	}
	return enabled, profiles, defaultCozeProfileID
}

func normalizeTargetScope(item ProjectionRow) string {
	rawScope := strings.ToLower(strings.TrimSpace(stringFromAny(item["target_scope"])))
	switch rawScope {
	case aiTargetScopeAccount, "accounts", "account_ids":
		return aiTargetScopeAccount
	case aiTargetScopeAll, "all_accounts":
		return aiTargetScopeAll
	case aiTargetScopeNone, defaultTargetAudienceNone:
		return aiTargetScopeNone
	case aiTargetScopeAssignee, "assignees", "cs":
		return aiTargetScopeAssignee
	}
	targetAudience := strings.TrimSpace(stringFromAny(item["target_audience"]))
	if targetAudience != "" && targetAudience != defaultTargetAudienceNone {
		return aiTargetScopeAssignee
	}
	if len(normalizeTargetAccountIDs(item["target_account_ids"])) > 0 {
		return aiTargetScopeAccount
	}
	return aiTargetScopeNone
}

func normalizeTargetAccountIDs(raw any) []string {
	var values []any
	switch typed := raw.(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, item := range values {
		candidate := strings.TrimSpace(stringFromAny(item))
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func normalizedTargetAudience(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return defaultTargetAudienceNone
	}
	return normalized
}

func hasCozeV2Profile(profiles []ProjectionRow) bool {
	for _, item := range profiles {
		if normalizeWorkflowSchema(firstNonBlank(stringFromAny(item["workflow_schema"]), stringFromAny(item["workflow_version"]))) == cozeWorkflowSchemaMainV2 {
			return true
		}
		if strings.TrimSpace(stringFromAny(item["workflow_id"])) == defaultCozeV2WorkflowID {
			return true
		}
	}
	return false
}

func normalizeWorkflowSchema(value string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	switch normalized {
	case "main_v2", "coze_main_v2", "v2", "2.0", "coze_2.0":
		return cozeWorkflowSchemaMainV2
	default:
		return cozeWorkflowSchemaLegacyV1
	}
}

func cozeRuntimeTokenSet(item ProjectionRow) bool {
	if strings.TrimSpace(stringFromAny(item["token"])) != "" {
		return true
	}
	if normalizeWorkflowSchema(firstNonBlank(stringFromAny(item["workflow_schema"]), stringFromAny(item["workflow_version"]))) == cozeWorkflowSchemaMainV2 {
		return firstEnv("COZE_V2_WORKFLOW_API_KEY", "COZE_MAIN_V2_API_KEY") != ""
	}
	return firstEnv("COZE_WORKFLOW_API_KEY", "COZE_API_KEY") != ""
}

func xiaobeiRuntimeTokenSet(item ProjectionRow) bool {
	if strings.TrimSpace(stringFromAny(item["token"])) != "" {
		return true
	}
	return firstEnv("XIAOBEI_AI_API_KEY", "AI_EXTERNAL_API_KEY") != ""
}

func cozeProfileID(item ProjectionRow, index int) string {
	value := firstNonBlank(stringFromAny(item["profile_id"]), stringFromAny(item["id"]))
	if value != "" {
		return value
	}
	if index == 0 {
		return "default"
	}
	return "coze-" + strconv.Itoa(index+1)
}

func xiaobeiProfileID(item ProjectionRow, index int) string {
	value := firstNonBlank(stringFromAny(item["profile_id"]), stringFromAny(item["id"]))
	if value != "" {
		return value
	}
	if index == 0 {
		return defaultXiaobeiProfileID
	}
	return "xiaobei-" + strconv.Itoa(index+1)
}

func firstProfileID(profiles []ProjectionRow, fallback string) string {
	if len(profiles) == 0 {
		return fallback
	}
	return firstNonBlank(stringFromAny(profiles[0]["profile_id"]), fallback)
}

func stringValue(row ProjectionRow, key string, fallback string) string {
	value := strings.TrimSpace(stringFromAny(row[key]))
	if value == "" {
		return fallback
	}
	return value
}

func boolValue(row ProjectionRow, key string, fallback bool) bool {
	value, ok := row[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return stringBool(typed)
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return fallback
	}
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}
