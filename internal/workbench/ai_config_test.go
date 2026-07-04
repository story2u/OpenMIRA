package workbench

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceAIConfigBuildsDefaults keeps the legacy default response shape.
func TestServiceAIConfigBuildsDefaults(t *testing.T) {
	t.Setenv("COZE_API_KEY", "coze-token")
	service := Service{AIConfigStore: fakeAIConfigStore{}}

	payload, err := service.AIConfig(context.Background(), AIConfigRequest{})
	if err != nil {
		t.Fatalf("AIConfig returned error: %v", err)
	}
	config := payload["config"].(ProjectionRow)
	if config["enabled"] != true || config["model"] != "deepseek-chat" || config["timeout_sec"] != float64(20) {
		t.Fatalf("config basics = %+v", config)
	}
	if config["api_key_set"] != false || config["provider_hint"] != "openai-compatible" {
		t.Fatalf("secret/provider fields = %+v", config)
	}
	profiles := config["coze_profiles"].([]ProjectionRow)
	if len(profiles) != 2 {
		t.Fatalf("coze profiles = %+v", profiles)
	}
	if rowText(profiles[0], "profile_id") != defaultCozeProfileID || profiles[0]["token_set"] != true {
		t.Fatalf("default coze profile = %+v", profiles[0])
	}
	if rowText(profiles[1], "workflow_schema") != cozeWorkflowSchemaMainV2 {
		t.Fatalf("default coze v2 profile = %+v", profiles[1])
	}
}

// TestServiceAIConfigParsesStoredProfiles keeps target and token masking stable.
func TestServiceAIConfigParsesStoredProfiles(t *testing.T) {
	store := fakeAIConfigStore{values: map[string]string{
		"ai.enabled":                   "false",
		"ai.api_key":                   "db-api-key",
		"ai.local_target_scope":        "account",
		"ai.local_target_account_ids":  `["acc-1","acc-1","acc-2"]`,
		"ai.local_default_ai_enabled":  "true",
		"ai.coze_v2_profile_seeded":    "true",
		"ai.active_coze_profile_id":    "coze-main",
		"ai.active_xiaobei_profile_id": "xb-1",
		"ai.coze_profiles":             `[{"profile_id":"coze-main","name":"主工作流","base_url":"https://coze.example","workflow_id":"wf-1","token":"secret","enabled":true,"target_scope":"account","target_account_ids":["acc-1"],"default_ai_enabled":true,"workflow_schema":"main-v2","spaceId":"space-1"}]`,
		"ai.xiaobei_profiles":          `[{"profile_id":"xb-1","name":"小贝一","base_url":"https://xb.example/chat","health_url":"https://xb.example/health","enabled":true,"target_audience":"cs-001"}]`,
	}}
	service := Service{AIConfigStore: store}

	payload, err := service.AIConfig(context.Background(), AIConfigRequest{})
	if err != nil {
		t.Fatalf("AIConfig returned error: %v", err)
	}
	config := payload["config"].(ProjectionRow)
	if config["enabled"] != false || config["api_key_set"] != true || config["local_default_ai_enabled"] != true {
		t.Fatalf("config flags = %+v", config)
	}
	accountIDs := config["local_target_account_ids"].([]string)
	if len(accountIDs) != 2 || accountIDs[0] != "acc-1" || accountIDs[1] != "acc-2" {
		t.Fatalf("local target ids = %+v", accountIDs)
	}
	cozeProfiles := config["coze_profiles"].([]ProjectionRow)
	if len(cozeProfiles) != 1 || rowText(cozeProfiles[0], "workflow_schema") != cozeWorkflowSchemaMainV2 || cozeProfiles[0]["token_set"] != true {
		t.Fatalf("coze profiles = %+v", cozeProfiles)
	}
	if rowText(cozeProfiles[0], "space_id") != "space-1" || rowText(cozeProfiles[0], "target_scope") != aiTargetScopeAccount {
		t.Fatalf("coze profile fields = %+v", cozeProfiles[0])
	}
	xiaobeiProfiles := config["xiaobei_profiles"].([]ProjectionRow)
	if len(xiaobeiProfiles) != 1 || rowText(xiaobeiProfiles[0], "target_scope") != aiTargetScopeAssignee {
		t.Fatalf("xiaobei profiles = %+v", xiaobeiProfiles)
	}
}

// TestServiceAIConfigFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceAIConfigFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).AIConfig(context.Background(), AIConfigRequest{})
	if err != ErrAIConfigStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrAIConfigStoreUnavailable)
	}
}

func TestServiceUpdateAIConfigWritesSettingsSyncsDefaultsAndAudits(t *testing.T) {
	store := fakeAIConfigStore{values: map[string]string{
		"ai.coze_v2_profile_seeded": "true",
		"ai.coze_profiles":          `[{"profile_id":"coze-a","name":"旧Coze","base_url":"https://old.example","workflow_id":"wf-old","token":"old-token","enabled":false,"target_scope":"none","target_account_ids":[],"default_ai_enabled":false}]`,
	}}
	enabled := true
	timeout := 30.0
	temperature := 0.3
	baseURL := " https://ai.example/v1 "
	model := " deepseek-chat "
	cozeEnabled := false
	writeStore := &fakeAccountAIWriteStore{
		account: AccountRecord{AccountID: "acc-001", AccountName: "账号一", AIEnabled: true},
		conversations: []AccountConversationAIRecord{
			{ConversationID: "conv-1", TenantID: "tenant-a", AccountID: "acc-001", AIAutoReply: true, AIModeOverride: "inherit"},
		},
	}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		AIConfigStore:        store,
		AIConfigWriteStore:   store,
		Accounts:             &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001", AccountName: "账号一"}}},
		AccountAIWriteStore:  writeStore,
		AIConfigEvents:       events,
		AuditLogWriter:       audit,
		ReadModelInvalidator: invalidator,
	}

	payload, err := service.UpdateAIConfig(context.Background(), NewAIConfigUpdateRequest(AIConfigUpdateBody{
		Enabled:               &enabled,
		BaseURL:               &baseURL,
		Model:                 &model,
		TimeoutSec:            &timeout,
		Temperature:           &temperature,
		LocalTargetScope:      "account",
		LocalTargetAccountIDs: []string{"acc-001"},
		LocalDefaultAIEnabled: true,
		CozeProfiles: []CozeProfileConfigBody{{
			ProfileID:  "coze-a",
			Name:       "新Coze",
			BaseURL:    "https://coze.example",
			WorkflowID: "wf-new",
			Enabled:    &cozeEnabled,
		}},
		ActiveCozeProfileID: "coze-a",
	}, auth.Session{Role: "admin", AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("UpdateAIConfig returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if store.values["ai.base_url"] != "https://ai.example/v1" || store.values["ai.model"] != "deepseek-chat" || store.values["ai.local_default_ai_enabled"] != "true" {
		t.Fatalf("stored values = %#v", store.values)
	}
	if !strings.Contains(store.values["ai.coze_profiles"], `"token":"old-token"`) || !strings.Contains(store.values["ai.coze_profiles"], `"name":"新Coze"`) {
		t.Fatalf("coze profile token/name not preserved: %s", store.values["ai.coze_profiles"])
	}
	if writeStore.accountID != "acc-001" || !writeStore.enabled || writeStore.syncAccountID != "acc-001" || !writeStore.syncEnabled || !writeStore.syncReset {
		t.Fatalf("account write store = %+v", writeStore)
	}
	if len(events.events) != 3 || events.events[0].event != "conversation.ai_auto_reply" || events.events[1].event != "account.updated" || events.events[2].event != "ai.config.updated" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 2 || !strings.Contains(audit.entries[0].Detail, "更新AI配置") || !strings.Contains(audit.entries[1].Detail, "AI配置默认托管同步") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

type fakeAIConfigStore struct {
	values map[string]string
}

func (store fakeAIConfigStore) GetAIConfigValue(ctx context.Context, key string) (string, error) {
	return store.values[key], nil
}

func (store fakeAIConfigStore) SetAIConfigValue(ctx context.Context, key string, value string) error {
	if store.values == nil {
		store.values = map[string]string{}
	}
	store.values[key] = value
	return nil
}
