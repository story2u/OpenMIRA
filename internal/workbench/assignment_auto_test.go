package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"im-go/internal/auth"
)

func TestServiceAutoAssignUsesRulePoolClaimsPublishesAndCommitsState(t *testing.T) {
	config := &fakeAssignmentConfigStore{values: map[string]string{
		assignmentRulesKey:                   `[{"rule_id":"rule-a","name":"VIP","priority":1,"enabled":true,"field_name":"sender_name","operator":"contains","match_value":"张","target_type":"pool","target_value":"pool-a","updated_at":"2026-07-01T09:00:00Z"}]`,
		assignmentPoolsKey:                   `[{"pool_id":"pool-a","pool_name":"A组","strategy_type":"round_robin","members":[{"assignee_id":"cs-002","weight":1},{"assignee_id":"cs-001","weight":1}],"enabled":true}]`,
		assignmentPoolStatePrefix + "pool-a": `{"next_index":1}`,
	}}
	assignments := &fakeAssignmentAutoStore{counts: map[string]int{"cs-001": 1, "cs-002": 0}}
	candidates := &fakeAssignmentAutoProjection{rows: []ProjectionRow{{
		"conversation_id": "conv-001",
		"tenant_id":       "tenant-a",
		"sender_name":     "张三",
		"unread_count":    2,
	}}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	runtimeState := &fakeAssignmentRuntimeState{claimErr: errors.New("cache unavailable")}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		CSUsers:                    &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-001", AssigneeName: "消息端一", Role: "cs", Enabled: true, MaxSessions: 3}, {AssigneeID: "cs-002", AssigneeName: "消息端二", Role: "cs", Enabled: true, MaxSessions: 3}}},
		Assignments:                assignments,
		Projection:                 candidates,
		AssignmentCfg:              config,
		AssignmentConfigWriteStore: config,
		AssignmentEvents:           events,
		AssignmentRuntimeState:     runtimeState,
		AuditLogWriter:             audit,
		ReadModelInvalidator:       invalidator,
	}

	payload, err := service.AutoAssignAssignments(context.Background(), NewAssignmentAutoAssignRequest(AssignmentAutoAssignBody{Limit: 1}, auth.Session{Role: "admin", AssigneeID: "admin-1", Claims: map[string]any{"tenant_id": "tenant-a"}}))
	if err != nil {
		t.Fatalf("AutoAssignAssignments returned error: %v", err)
	}
	if payload["success"] != true || payload["assigned_count"] != 1 || payload["skipped_count"] != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if assignments.countTenant != "tenant-a" || candidates.tenantID != "tenant-a" || candidates.limit != 60 {
		t.Fatalf("count/candidate scope = tenant:%q candidateTenant:%q limit:%d", assignments.countTenant, candidates.tenantID, candidates.limit)
	}
	if len(assignments.claims) != 1 || assignments.claims[0].ConversationID != "conv-001" || assignments.claims[0].AssigneeID != "cs-001" || assignments.claims[0].TenantID != "tenant-a" {
		t.Fatalf("claims = %+v", assignments.claims)
	}
	assignmentsPayload := payload["assignments"].([]ProjectionRow)
	decision := assignmentsPayload[0]["decision"].(ProjectionRow)
	if rowText(decision, "rule_id") != "rule-a" || rowText(decision, "pool_id") != "pool-a" || rowText(decision, "strategy_type") != "round_robin" {
		t.Fatalf("decision = %+v", decision)
	}
	var state map[string]int
	if err := json.Unmarshal([]byte(config.writes[assignmentPoolStatePrefix+"pool-a"]), &state); err != nil {
		t.Fatalf("state json error: %v", err)
	}
	if state["next_index"] != 0 {
		t.Fatalf("state = %+v", state)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.assigned" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(runtimeState.claims) != 1 || runtimeState.claims[0].tenantID != "tenant-a" || runtimeState.claims[0].assigneeID != "cs-001" || runtimeState.claims[0].conversationID != "conv-001" {
		t.Fatalf("runtime claims = %+v", runtimeState.claims)
	}
	if len(audit.entries) != 1 || !strings.Contains(audit.entries[0].Detail, "assigned=1, skipped=0") {
		t.Fatalf("audit = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceAutoAssignUsesPoolRuntimeSelector(t *testing.T) {
	config := &fakeAssignmentConfigStore{values: map[string]string{
		assignmentRulesKey: `[{"rule_id":"rule-a","name":"VIP","priority":1,"enabled":true,"field_name":"sender_name","operator":"contains","match_value":"张","target_type":"pool","target_value":"pool-a","updated_at":"2026-07-01T09:00:00Z"}]`,
		assignmentPoolsKey: `[{"pool_id":"pool-a","pool_name":"A组","strategy_type":"round_robin","members":[{"assignee_id":"cs-001","weight":1},{"assignee_id":"cs-002","weight":1}],"enabled":true}]`,
	}}
	assignments := &fakeAssignmentAutoStore{counts: map[string]int{"cs-001": 0, "cs-002": 0}}
	selector := &fakeAssignmentPoolRuntimeSelector{roundRobinSelected: "cs-002", roundRobinOK: true}
	service := Service{
		CSUsers:                       &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-001", AssigneeName: "消息端一", Role: "cs", Enabled: true, MaxSessions: 3}, {AssigneeID: "cs-002", AssigneeName: "消息端二", Role: "cs", Enabled: true, MaxSessions: 3}}},
		Assignments:                   assignments,
		Projection:                    &fakeAssignmentAutoProjection{rows: []ProjectionRow{{"conversation_id": "conv-002", "tenant_id": "tenant-a", "sender_name": "张三", "unread_count": 1}}},
		AssignmentCfg:                 config,
		AssignmentConfigWriteStore:    config,
		AssignmentPoolRuntimeSelector: selector,
	}

	payload, err := service.AutoAssignAssignments(context.Background(), NewAssignmentAutoAssignRequest(AssignmentAutoAssignBody{Limit: 1}, auth.Session{Role: "admin", Claims: map[string]any{"tenant_id": "tenant-a"}}))
	if err != nil {
		t.Fatalf("AutoAssignAssignments returned error: %v", err)
	}

	if payload["assigned_count"] != 1 || len(assignments.claims) != 1 || assignments.claims[0].AssigneeID != "cs-002" {
		t.Fatalf("payload=%#v claims=%+v", payload, assignments.claims)
	}
	if len(selector.roundRobinCalls) != 1 || selector.roundRobinCalls[0].poolID != "pool-a" || strings.Join(selector.roundRobinCalls[0].memberIDs, ",") != "cs-001,cs-002" || strings.Join(selector.roundRobinCalls[0].availableIDs, ",") != "cs-001,cs-002" {
		t.Fatalf("selector calls = %+v", selector.roundRobinCalls)
	}
	if _, ok := config.writes[assignmentPoolStatePrefix+"pool-a"]; ok {
		t.Fatalf("pool state write should be skipped when Redis selector applies: %+v", config.writes)
	}
}

func TestServiceAutoAssignUsesRuntimeLoadCountsAndBackfillsMissing(t *testing.T) {
	config := &fakeAssignmentConfigStore{values: map[string]string{assignmentRulesKey: `[]`, assignmentPoolsKey: `[]`}}
	assignments := &fakeAssignmentAutoStore{counts: map[string]int{"cs-002": 0}}
	runtimeState := &fakeAssignmentRuntimeState{
		loadCounts:  map[string]int{"cs-001": 5},
		loadMissing: []string{"cs-002"},
	}
	service := Service{
		CSUsers:                &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-001", AssigneeName: "消息端一", Role: "cs", Enabled: true, MaxSessions: 10}, {AssigneeID: "cs-002", AssigneeName: "消息端二", Role: "cs", Enabled: true, MaxSessions: 10}}},
		Assignments:            assignments,
		Projection:             &fakeAssignmentAutoProjection{rows: []ProjectionRow{{"conversation_id": "conv-002", "tenant_id": "tenant-a", "unread_count": 1}}},
		AssignmentCfg:          config,
		AssignmentRuntimeState: runtimeState,
	}

	payload, err := service.AutoAssignAssignments(context.Background(), NewAssignmentAutoAssignRequest(AssignmentAutoAssignBody{Limit: 1}, auth.Session{Role: "admin", Claims: map[string]any{"tenant_id": "tenant-a"}}))
	if err != nil {
		t.Fatalf("AutoAssignAssignments returned error: %v", err)
	}

	if payload["assigned_count"] != 1 || len(assignments.claims) != 1 || assignments.claims[0].AssigneeID != "cs-002" {
		t.Fatalf("payload=%#v claims=%+v", payload, assignments.claims)
	}
	if len(runtimeState.loadCalls) != 1 || runtimeState.loadCalls[0].tenantID != "tenant-a" || strings.Join(runtimeState.loadCalls[0].assigneeIDs, ",") != "cs-001,cs-002" {
		t.Fatalf("runtime load calls = %+v", runtimeState.loadCalls)
	}
	if strings.Join(assignments.countIDs, ",") != "cs-002" || assignments.countTenant != "tenant-a" {
		t.Fatalf("db count backfill ids=%+v tenant=%q", assignments.countIDs, assignments.countTenant)
	}
}

func TestServiceAutoAssignSkipsWhenNoCapacity(t *testing.T) {
	config := &fakeAssignmentConfigStore{values: map[string]string{assignmentRulesKey: `[]`, assignmentPoolsKey: `[]`}}
	assignments := &fakeAssignmentAutoStore{counts: map[string]int{"cs-001": 1, "cs-002": 2}}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		CSUsers:              &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-001", AssigneeName: "消息端一", Role: "cs", Enabled: true, MaxSessions: 1}, {AssigneeID: "cs-002", AssigneeName: "消息端二", Role: "cs", Enabled: true, MaxSessions: 2}}},
		Assignments:          assignments,
		Projection:           &fakeAssignmentAutoProjection{rows: []ProjectionRow{{"conversation_id": "conv-001", "tenant_id": "tenant-a"}}},
		AssignmentCfg:        config,
		ReadModelInvalidator: invalidator,
	}

	payload, err := service.AutoAssignAssignments(context.Background(), NewAssignmentAutoAssignRequest(AssignmentAutoAssignBody{Limit: 5}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("AutoAssignAssignments returned error: %v", err)
	}
	if payload["assigned_count"] != 0 || payload["skipped_count"] != 1 || len(assignments.claims) != 0 {
		t.Fatalf("payload=%#v claims=%+v", payload, assignments.claims)
	}
	skipped := payload["skipped"].([]ProjectionRow)
	if rowText(skipped[0], "reason") != "no assignee capacity" {
		t.Fatalf("skipped = %+v", skipped)
	}
	if len(invalidator.namespaces) != 0 {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceAutoAssignRequiresEnabledCSUsers(t *testing.T) {
	service := Service{
		CSUsers:       &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "admin-1", Role: "admin", Enabled: true}}},
		Assignments:   &fakeAssignmentAutoStore{},
		Projection:    &fakeAssignmentAutoProjection{},
		AssignmentCfg: &fakeAssignmentConfigStore{values: map[string]string{}},
	}

	_, err := service.AutoAssignAssignments(context.Background(), NewAssignmentAutoAssignRequest(AssignmentAutoAssignBody{}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrNoEnabledCSUsers) {
		t.Fatalf("err = %v, want %v", err, ErrNoEnabledCSUsers)
	}
}

type fakeAssignmentAutoProjection struct {
	rows     []ProjectionRow
	tenantID string
	limit    int
}

func (store *fakeAssignmentAutoProjection) ListRows(ctx context.Context, query ProjectionQuery) ([]ProjectionRow, error) {
	return nil, nil
}

func (store *fakeAssignmentAutoProjection) CountScoped(ctx context.Context, query ProjectionQuery) (ProjectionStats, error) {
	return ProjectionStats{}, nil
}

func (store *fakeAssignmentAutoProjection) ListAutoAssignCandidates(ctx context.Context, tenantID string, limit int) ([]ProjectionRow, error) {
	store.tenantID = tenantID
	store.limit = limit
	return store.rows, nil
}

type fakeAssignmentAutoStore struct {
	counts      map[string]int
	countIDs    []string
	countTenant string
	claims      []AssignmentClaimCommand
	claimErr    error
}

func (store *fakeAssignmentAutoStore) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	return nil, nil
}

func (store *fakeAssignmentAutoStore) CountByAssigneeIDs(ctx context.Context, assigneeIDs []string, tenantID string) (map[string]int, error) {
	store.countIDs = append([]string{}, assigneeIDs...)
	store.countTenant = tenantID
	return store.counts, nil
}

func (store *fakeAssignmentAutoStore) ClaimAssignment(ctx context.Context, command AssignmentClaimCommand) (AssignmentRecord, error) {
	store.claims = append(store.claims, command)
	if store.claimErr != nil {
		return AssignmentRecord{}, store.claimErr
	}
	return AssignmentRecord{TenantID: command.TenantID, ConversationID: command.ConversationID, AssigneeID: command.AssigneeID, AssigneeName: command.AssigneeName, AssignedAt: "2026-07-01T09:00:00Z", UpdatedAt: "2026-07-01T09:00:00Z"}, nil
}

func (store *fakeAssignmentAutoStore) ReleaseAssignment(ctx context.Context, command AssignmentReleaseCommand) (bool, error) {
	return false, nil
}

type fakeAssignmentPoolRuntimeCall struct {
	poolID       string
	memberIDs    []string
	availableIDs []string
	weights      map[string]int
}

type fakeAssignmentPoolRuntimeSelector struct {
	roundRobinCalls    []fakeAssignmentPoolRuntimeCall
	roundRobinSelected string
	roundRobinOK       bool
	roundRobinErr      error
	ratioCalls         []fakeAssignmentPoolRuntimeCall
	ratioSelected      string
	ratioOK            bool
	ratioErr           error
}

func (selector *fakeAssignmentPoolRuntimeSelector) SelectRoundRobinPoolUser(ctx context.Context, poolID string, memberIDs []string, availableIDs []string) (string, bool, error) {
	selector.roundRobinCalls = append(selector.roundRobinCalls, fakeAssignmentPoolRuntimeCall{poolID: poolID, memberIDs: append([]string{}, memberIDs...), availableIDs: append([]string{}, availableIDs...)})
	if selector.roundRobinErr != nil {
		return "", false, selector.roundRobinErr
	}
	return selector.roundRobinSelected, selector.roundRobinOK, nil
}

func (selector *fakeAssignmentPoolRuntimeSelector) SelectRatioPoolUser(ctx context.Context, poolID string, weights map[string]int, availableIDs []string) (string, bool, error) {
	selector.ratioCalls = append(selector.ratioCalls, fakeAssignmentPoolRuntimeCall{poolID: poolID, weights: copyStringIntMap(weights), availableIDs: append([]string{}, availableIDs...)})
	if selector.ratioErr != nil {
		return "", false, selector.ratioErr
	}
	return selector.ratioSelected, selector.ratioOK, nil
}

func copyStringIntMap(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
