package workbench

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceToggleConversationAIRequiresExplicitProfileAndClearsRuntime(t *testing.T) {
	enabled := true
	store := &fakeConversationAIStore{records: map[string]ConversationAIRecord{
		"conv-1": {
			ConversationID:  "conv-1",
			TenantID:        "tenant-a",
			AccountID:       "acc-001",
			AIModeOverride:  "manual",
			SOPRuntimeState: map[string]any{"ai_reply_status": "failed", "sensitive_handoff_pending": true},
		},
	}}
	events := &fakeScriptEventPublisher{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Accounts:             &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001", AssigneeID: "cs-001", AIEnabled: false}}},
		ConversationAIStore:  store,
		ConversationAIEvents: events,
		ReadModelInvalidator: invalidator,
		AIConfigStore: fakeAIConfigStore{values: map[string]string{
			"ai.enabled":                  "true",
			"ai.local_target_scope":       "account",
			"ai.local_target_account_ids": `["acc-001"]`,
			"ai.local_target_audience":    "__NONE__",
			"ai.coze_profiles":            `[]`,
			"ai.xiaobei_profiles":         `[]`,
		}},
	}

	payload, err := service.ToggleConversationAI(context.Background(), NewConversationAIRequest(" conv-1 ", ConversationAIBody{Enabled: &enabled}, auth.Session{Role: "cs", AssigneeID: "cs-001"}))
	if err != nil {
		t.Fatalf("ToggleConversationAI returned error: %v", err)
	}
	if store.setConversationID != "conv-1" || store.setOverride != "auto" || store.setAccountAIEnabled {
		t.Fatalf("set call = conversation:%q override:%q accountAI:%t", store.setConversationID, store.setOverride, store.setAccountAIEnabled)
	}
	if payload["success"] != true || payload["ai_auto_reply"] != true || payload["account_ai_enabled"] != false || payload["ai_mode_override"] != "auto" {
		t.Fatalf("payload = %#v", payload)
	}
	if store.updatedRuntime["ai_reply_status"] != nil || store.updatedRuntime["handoff_status"] != "auto_active" || store.updatedRuntime["sensitive_handoff_pending"] != false {
		t.Fatalf("runtime = %#v", store.updatedRuntime)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.ai_auto_reply" || events.events[0].topic != "conversation.message" {
		t.Fatalf("events = %+v", events.events)
	}
	if events.events[0].payload["conversation_id"] != "conv-1" || events.events[0].payload["ai_auto_reply"] != true || events.events[0].payload["account_ai_enabled"] != false {
		t.Fatalf("event payload = %#v", events.events[0].payload)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceToggleConversationAIRejectsMissingExplicitProfile(t *testing.T) {
	enabled := true
	service := Service{
		Accounts: &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001", AssigneeID: "cs-001"}}},
		ConversationAIStore: &fakeConversationAIStore{records: map[string]ConversationAIRecord{
			"conv-1": {ConversationID: "conv-1", AccountID: "acc-001"},
		}},
		AIConfigStore: fakeAIConfigStore{values: map[string]string{
			"ai.enabled":                  "true",
			"ai.local_target_scope":       "none",
			"ai.local_target_account_ids": `[]`,
			"ai.local_target_audience":    "__NONE__",
			"ai.coze_profiles":            `[]`,
			"ai.xiaobei_profiles":         `[]`,
		}},
	}

	_, err := service.ToggleConversationAI(context.Background(), NewConversationAIRequest("conv-1", ConversationAIBody{Enabled: &enabled}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrConversationAIProfileRequired) {
		t.Fatalf("err = %v, want profile required", err)
	}
}

func TestServiceToggleConversationAIBulkUsesCSScopeAndPublishesBulkEvent(t *testing.T) {
	enabled := false
	store := &fakeConversationAIStore{
		scopedIDs: []string{"conv-1", "conv-2"},
		records: map[string]ConversationAIRecord{
			"conv-1": {ConversationID: "conv-1", TenantID: "tenant-a", AccountID: "acc-1", AIAutoReply: true, AIModeOverride: "auto"},
			"conv-2": {ConversationID: "conv-2", TenantID: "tenant-a", AccountID: "acc-2", AIAutoReply: true, AIModeOverride: "auto"},
		},
	}
	events := &fakeScriptEventPublisher{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{ConversationAIStore: store, ConversationAIEvents: events, ReadModelInvalidator: invalidator}

	payload, err := service.ToggleConversationAIBulk(context.Background(), NewConversationAIBulkRequest(ConversationAIBulkBody{Enabled: &enabled}, auth.Session{Role: "cs", AssigneeID: "cs-001", Claims: map[string]any{"tenant_id": "tenant-a"}}))
	if err != nil {
		t.Fatalf("ToggleConversationAIBulk returned error: %v", err)
	}
	if store.scopedAssigneeID != "cs-001" || store.scopedTenantID != "tenant-a" || store.bulkOverride != "manual" || store.bulkAccountAIEnabled {
		t.Fatalf("bulk store = %+v", store)
	}
	if payload["success"] != true || payload["enabled"] != false || payload["updated_count"] != 2 || payload["assignee_id"] != "cs-001" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.ai_auto_reply.bulk" || events.events[0].payload["updated_count"] != 2 {
		t.Fatalf("events = %+v", events.events)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceToggleConversationAIBulkEnforcesCrossAssigneePermission(t *testing.T) {
	enabled := true
	service := Service{ConversationAIStore: &fakeConversationAIStore{}}

	_, err := service.ToggleConversationAIBulk(context.Background(), NewConversationAIBulkRequest(ConversationAIBulkBody{Enabled: &enabled, AssigneeID: "cs-002"}, auth.Session{Role: "cs", AssigneeID: "cs-001"}))
	if !errors.Is(err, auth.ErrPermissionDenied) {
		t.Fatalf("err = %v, want permission denied", err)
	}
}

func TestAITargetAudienceMatchesLegacySeparators(t *testing.T) {
	if !aiTargetAudienceMatches("__ALL__", "cs-001") {
		t.Fatal("__ALL__ should match non-empty assignee")
	}
	if !aiTargetAudienceMatches("cs-000, cs-001\ncs-002；cs-003", "cs-002") {
		t.Fatal("mixed separators should match assignee")
	}
	if aiTargetAudienceMatches("__NONE__", "cs-001") || aiTargetAudienceMatches("cs-002", "cs-001") {
		t.Fatal("unexpected target audience match")
	}
}

type fakeConversationAIStore struct {
	records              map[string]ConversationAIRecord
	allIDs               []string
	scopedIDs            []string
	setConversationID    string
	setOverride          string
	setAccountAIEnabled  bool
	bulkIDs              []string
	bulkOverride         string
	bulkAccountAIEnabled bool
	scopedAssigneeID     string
	scopedTenantID       string
	updatedRuntime       map[string]any
}

func (store *fakeConversationAIStore) GetConversationAI(ctx context.Context, conversationID string) (ConversationAIRecord, bool, error) {
	record, ok := store.records[conversationID]
	return record, ok, nil
}

func (store *fakeConversationAIStore) SetConversationAIModeOverride(ctx context.Context, conversationID string, overrideMode string, accountAIEnabled bool) (ConversationAIRecord, bool, error) {
	store.setConversationID = conversationID
	store.setOverride = overrideMode
	store.setAccountAIEnabled = accountAIEnabled
	record, ok := store.records[conversationID]
	if !ok {
		return ConversationAIRecord{}, false, nil
	}
	record.AIModeOverride = overrideMode
	record.AIAutoReply = ComputeEffectiveConversationAI(accountAIEnabled, overrideMode)
	store.records[conversationID] = record
	return record, true, nil
}

func (store *fakeConversationAIStore) SetConversationAIModeOverrideBulk(ctx context.Context, conversationIDs []string, overrideMode string, accountAIEnabled bool) ([]ConversationAIRecord, error) {
	store.bulkIDs = append([]string{}, conversationIDs...)
	store.bulkOverride = overrideMode
	store.bulkAccountAIEnabled = accountAIEnabled
	out := make([]ConversationAIRecord, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		record, ok := store.records[id]
		if !ok {
			continue
		}
		record.AIModeOverride = overrideMode
		record.AIAutoReply = ComputeEffectiveConversationAI(accountAIEnabled, overrideMode)
		store.records[id] = record
		out = append(out, record)
	}
	return out, nil
}

func (store *fakeConversationAIStore) ListAllConversationAIIDs(ctx context.Context) ([]string, error) {
	return append([]string{}, store.allIDs...), nil
}

func (store *fakeConversationAIStore) ListAssigneeScopedConversationAIIDs(ctx context.Context, assigneeID string, tenantID string) ([]string, error) {
	store.scopedAssigneeID = assigneeID
	store.scopedTenantID = tenantID
	return append([]string{}, store.scopedIDs...), nil
}

func (store *fakeConversationAIStore) UpdateConversationRuntimeState(ctx context.Context, conversationID string, runtimeState map[string]any) error {
	store.updatedRuntime = runtimeState
	return nil
}
