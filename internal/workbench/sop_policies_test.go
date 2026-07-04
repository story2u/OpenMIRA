package workbench

import (
	"context"
	"strings"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceSOPPoliciesBuildsGroupedPayload keeps Python list_policy_groups shape.
func TestServiceSOPPoliciesBuildsGroupedPayload(t *testing.T) {
	policies := []SOPPolicyRecord{
		{PolicyID: "policy-1", FlowID: "default", Name: "DAY1", DayStage: "1", TriggerEvent: "incoming_message", Enabled: true, Priority: 10, ReplyMode: "sop_only", ReplyText: "hello", ImageURLs: "local://image.png", CreatedAt: "2026-06-28T09:00:00Z", UpdatedAt: "2026-06-29T09:00:00Z"},
		{PolicyID: "policy-2", FlowID: "flow-b", Name: "DAY2", DayStage: "2", TriggerEvent: "friend_added", Enabled: true, Priority: 20, MessageSequence: `[{"type":"file","content":"/objects/a/file.pdf"}]`, CreatedAt: "2026-06-28T10:00:00Z", UpdatedAt: "2026-06-29T10:00:00Z"},
	}
	flows := []SOPFlowRecord{
		{FlowID: "default", FlowName: "默认流程", Enabled: true},
		{FlowID: "flow-b", FlowName: "流程B", Enabled: true},
	}
	service := Service{
		SOPPolicyStore:  fakeSOPPolicyStore{policies: policies},
		SOPFlowStore:    fakeSOPFlowStore{flows: flows},
		MediaURLBuilder: fakeMediaURLBuilder{},
	}

	payload, err := service.SOPPolicies(context.Background(), SOPPoliciesRequest{FlowID: "flow-b", DayStage: "2"})
	if err != nil {
		t.Fatalf("SOPPolicies returned error: %v", err)
	}
	topPolicies := payload["policies"].([]ProjectionRow)
	if len(topPolicies) != 1 || rowText(topPolicies[0], "policy_id") != "policy-2" {
		t.Fatalf("top policies = %+v", topPolicies)
	}
	messages := topPolicies[0]["messages"].([]ProjectionRow)
	if len(messages) != 1 || rowText(messages[0], "preview_url") != "signed:policy-2-0:/objects/a/file.pdf" {
		t.Fatalf("messages = %+v", messages)
	}
	groups := payload["flows"].([]ProjectionRow)
	if len(groups) != 2 || rowText(groups[0], "flow_id") != "default" || rowText(groups[1], "flow_id") != "flow-b" {
		t.Fatalf("groups = %+v", groups)
	}
	flowPolicies := groups[1]["policies"].([]ProjectionRow)
	if len(flowPolicies) != 1 || rowText(flowPolicies[0], "policy_id") != "policy-2" {
		t.Fatalf("flow policies = %+v", flowPolicies)
	}

	unfiltered, err := service.SOPPolicies(context.Background(), SOPPoliciesRequest{})
	if err != nil {
		t.Fatalf("SOPPolicies unfiltered returned error: %v", err)
	}
	firstMessages := unfiltered["policies"].([]ProjectionRow)[0]["messages"].([]ProjectionRow)
	if len(firstMessages) != 2 || rowText(firstMessages[1], "preview_url") != "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fimage.png" {
		t.Fatalf("local preview messages = %+v", firstMessages)
	}
}

// TestServiceSOPPoliciesFailsClosedWithoutStores keeps missing stores explicit.
func TestServiceSOPPoliciesFailsClosedWithoutStores(t *testing.T) {
	_, err := (Service{}).SOPPolicies(context.Background(), SOPPoliciesRequest{})
	if err != ErrSOPPolicyStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrSOPPolicyStoreUnavailable)
	}
	_, err = (Service{SOPPolicyStore: fakeSOPPolicyStore{}}).SOPPolicies(context.Background(), SOPPoliciesRequest{})
	if err != ErrSOPFlowStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrSOPFlowStoreUnavailable)
	}
}

// TestServiceUpsertSOPPolicyNormalizesPlatformPullTrigger keeps Python write semantics.
func TestServiceUpsertSOPPolicyNormalizesPlatformPullTrigger(t *testing.T) {
	enabled := false
	priority := 0
	store := &fakeSOPPolicyWriteStore{policy: SOPPolicyRecord{PolicyID: "policy-1", FlowID: "flow-b", Name: "DAY1", DayStage: "1", TriggerEvent: "friend_added", Enabled: false, Priority: 0, ReplyMode: "sop_only", ReplyText: "hello"}}
	publisher := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	service := Service{
		SOPPolicyWriteStore: store,
		SOPFlowStore:        fakeSOPFlowStore{flows: []SOPFlowRecord{{FlowID: "flow-b", ExecutionMode: "platform_pull"}}},
		SOPEvents:           publisher,
		AuditLogWriter:      audit,
	}

	payload, err := service.UpsertSOPPolicy(context.Background(), NewSOPPolicyUpsertRequest(SOPPolicyUpsertBody{
		PolicyID:       " policy-1 ",
		FlowID:         " flow-b ",
		Name:           " DAY1 ",
		DayStage:       " 1 ",
		TriggerEvent:   "incoming_message",
		Enabled:        &enabled,
		Priority:       &priority,
		ReplyText:      " hello ",
		MediaStrategy:  "",
		CustomerState:  "",
		DispatchQueue:  "",
		ReplyMode:      "",
		NeedRAG:        true,
		NeedAIRewrite:  true,
		RiskKeywords:   " 风险 ",
		PromptTemplate: " ",
	}, auth.Session{AssigneeID: "admin-1"}))
	if err != nil {
		t.Fatalf("UpsertSOPPolicy returned error: %v", err)
	}
	if store.command.PolicyID != "policy-1" || store.command.FlowID != "flow-b" || store.command.TriggerEvent != "friend_added" || store.command.Priority != 0 || store.command.CustomerState != "undecided" || store.command.DispatchQueue != "slow" || store.command.ReplyMode != "sop_only" || store.command.MediaStrategy != "fixed" || !store.command.NeedRAG || !store.command.NeedAIRewrite {
		t.Fatalf("command = %+v", store.command)
	}
	policy := payload["policy"].(ProjectionRow)
	if payload["success"] != true || rowText(policy, "policy_id") != "policy-1" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "sop.policy.updated" || publisher.events[0].payload["policy_id"] != "policy-1" {
		t.Fatalf("events = %+v", publisher.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "config" || !strings.Contains(audit.entries[0].Detail, "DAY1") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

// TestServiceUpsertSOPPolicyRejectsRequiredFields preserves FastAPI 422 causes.
func TestServiceUpsertSOPPolicyRejectsRequiredFields(t *testing.T) {
	service := Service{SOPPolicyWriteStore: &fakeSOPPolicyWriteStore{}}
	cases := []struct {
		name string
		body SOPPolicyUpsertBody
		want string
	}{
		{name: "name", body: SOPPolicyUpsertBody{DayStage: "1", TriggerEvent: "incoming_message", ReplyText: "hello"}, want: "name is required"},
		{name: "day", body: SOPPolicyUpsertBody{Name: "DAY1", TriggerEvent: "incoming_message", ReplyText: "hello"}, want: "day_stage is required"},
		{name: "trigger", body: SOPPolicyUpsertBody{Name: "DAY1", DayStage: "1", ReplyText: "hello"}, want: "trigger_event is required"},
		{name: "reply", body: SOPPolicyUpsertBody{Name: "DAY1", DayStage: "1", TriggerEvent: "incoming_message"}, want: "reply_text or prompt_template is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpsertSOPPolicy(context.Background(), NewSOPPolicyUpsertRequest(tc.body, auth.Session{}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestServiceDeleteSOPPolicyPublishesOnlyWhenDeleted keeps delete semantics.
func TestServiceDeleteSOPPolicyPublishesOnlyWhenDeleted(t *testing.T) {
	store := &fakeSOPPolicyWriteStore{deleted: true}
	publisher := &fakeScriptEventPublisher{}
	service := Service{SOPPolicyWriteStore: store, SOPEvents: publisher, AuditLogWriter: &fakeAuditWriter{}}

	payload, err := service.DeleteSOPPolicy(context.Background(), NewSOPPolicyDeleteRequest(" policy-1 ", auth.Session{AssigneeID: "admin-1"}))
	if err != nil {
		t.Fatalf("DeleteSOPPolicy returned error: %v", err)
	}
	if payload["success"] != true || store.policyID != "policy-1" {
		t.Fatalf("payload=%+v policyID=%q", payload, store.policyID)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "sop.policy.deleted" || publisher.events[0].payload["policy_id"] != "policy-1" {
		t.Fatalf("events = %+v", publisher.events)
	}

	store.deleted = false
	publisher.events = nil
	payload, err = service.DeleteSOPPolicy(context.Background(), NewSOPPolicyDeleteRequest("missing", auth.Session{}))
	if err != nil {
		t.Fatalf("DeleteSOPPolicy missing returned error: %v", err)
	}
	if payload["success"] != false || len(publisher.events) != 0 {
		t.Fatalf("missing delete payload=%+v events=%+v", payload, publisher.events)
	}
}

type fakeSOPPolicyStore struct {
	policies []SOPPolicyRecord
}

func (store fakeSOPPolicyStore) ListSOPPolicies(ctx context.Context) ([]SOPPolicyRecord, error) {
	return store.policies, nil
}

type fakeMediaURLBuilder struct{}

func (builder fakeMediaURLBuilder) BuildAccessURL(taskID string, objectURL string) string {
	return "signed:" + taskID + ":" + objectURL
}

type fakeSOPPolicyWriteStore struct {
	command  SOPPolicyCommand
	policy   SOPPolicyRecord
	policyID string
	deleted  bool
}

func (store *fakeSOPPolicyWriteStore) UpsertSOPPolicy(ctx context.Context, command SOPPolicyCommand) (SOPPolicyRecord, error) {
	store.command = command
	return store.policy, nil
}

func (store *fakeSOPPolicyWriteStore) DeleteSOPPolicy(ctx context.Context, policyID string) (bool, error) {
	store.policyID = policyID
	return store.deleted, nil
}
