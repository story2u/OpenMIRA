package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceAssignmentConfigBuildsSortedPayload(t *testing.T) {
	store := fakeAssignmentConfigStore{values: map[string]string{
		assignmentRulesKey: `[
			{"rule_id":"rule-b","name":"B","priority":20,"field_name":"sender","target_type":"pool","target_value":"pool-a","updated_at":"2026-06-29T10:00:00Z"},
			{"rule_id":"rule-a","name":"A","priority":10,"field_name":"sender","target_type":"pool","target_value":"pool-b","updated_at":"2026-06-29T11:00:00Z"}
		]`,
		assignmentPoolsKey: `[
			{"pool_id":"pool-b","pool_name":"B组","strategy_type":"ratio","members":[{"assignee_id":"cs-002","weight":2}],"enabled":true},
			{"pool_id":"pool-a","pool_name":"A组","strategy_type":"round_robin","members":[{"assignee_id":"cs-001","weight":1}],"enabled":true}
		]`,
	}}
	service := Service{AssignmentCfg: store}

	payload, err := service.AssignmentConfig(context.Background(), AssignmentConfigRequest{Session: auth.Session{Role: "admin"}})
	if err != nil {
		t.Fatalf("AssignmentConfig returned error: %v", err)
	}
	rules := payload["rules"].([]ProjectionRow)
	pools := payload["pools"].([]ProjectionRow)
	if len(rules) != 2 || rowText(rules[0], "rule_id") != "rule-a" || rowText(rules[1], "rule_id") != "rule-b" {
		t.Fatalf("rules not sorted: %+v", rules)
	}
	if len(pools) != 2 || rowText(pools[0], "pool_id") != "pool-a" || rowText(pools[1], "pool_id") != "pool-b" {
		t.Fatalf("pools not sorted: %+v", pools)
	}
}

func TestServiceAssignmentConfigTreatsInvalidJSONAsEmpty(t *testing.T) {
	service := Service{AssignmentCfg: fakeAssignmentConfigStore{values: map[string]string{
		assignmentRulesKey: "not-json",
		assignmentPoolsKey: "",
	}}}

	payload, err := service.AssignmentConfig(context.Background(), AssignmentConfigRequest{})
	if err != nil {
		t.Fatalf("AssignmentConfig returned error: %v", err)
	}
	if len(payload["rules"].([]ProjectionRow)) != 0 || len(payload["pools"].([]ProjectionRow)) != 0 {
		t.Fatalf("payload = %+v, want empty rules and pools", payload)
	}
}

func TestServiceAssignmentConfigFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).AssignmentConfig(context.Background(), AssignmentConfigRequest{})
	if err != ErrAssignmentConfigStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrAssignmentConfigStoreUnavailable)
	}
}

func TestServiceUpdateAssignmentConfigNormalizesWritesResetsPublishesAndAudits(t *testing.T) {
	store := &fakeAssignmentConfigStore{values: map[string]string{
		assignmentPoolsKey: `[{"pool_id":"pool-old","pool_name":"旧池","strategy_type":"round_robin","members":[{"assignee_id":"cs-old","weight":1}],"enabled":true}]`,
	}}
	audit := &fakeAuditWriter{}
	events := &fakeScriptEventPublisher{}
	runtime := &fakeAssignmentPoolRuntime{}
	service := Service{
		AssignmentCfg:              store,
		AssignmentConfigWriteStore: store,
		AssignmentConfigEvents:     events,
		AssignmentPoolRuntime:      runtime,
		AuditLogWriter:             audit,
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 8, 0, 0, 123456789, time.UTC)
		},
	}

	payload, err := service.UpdateAssignmentConfig(context.Background(), NewAssignmentConfigUpdateRequest(AssignmentConfigUpdateBody{
		Rules: []map[string]any{{
			"rule_id":      " rule-001 ",
			"name":         " VIP ",
			"priority":     float64(5),
			"enabled":      false,
			"field_name":   " sender_name ",
			"match_value":  " 张 ",
			"target_value": " pool-new ",
			"created_at":   "2026-06-30T01:02:03+00:00",
		}},
		Pools: []map[string]any{{
			"pool_id":       " pool-new ",
			"pool_name":     " A组 ",
			"strategy_type": " ratio ",
			"members": []any{
				map[string]any{"assignee_id": " cs-001 ", "weight": float64(2)},
				map[string]any{"assignee_id": "cs-001", "weight": float64(9)},
				map[string]any{"assignee_id": " ", "weight": float64(1)},
			},
			"created_at": "bad-time",
		}},
	}, auth.Session{AssigneeID: "admin-001", Role: "admin"}))
	if err != nil {
		t.Fatalf("UpdateAssignmentConfig returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	rules := payload["rules"].([]ProjectionRow)
	pools := payload["pools"].([]ProjectionRow)
	if len(rules) != 1 || len(pools) != 1 {
		t.Fatalf("payload rules/pools = %+v", payload)
	}
	rule := rules[0]
	if rowText(rule, "rule_id") != "rule-001" || rowText(rule, "operator") != "equals" || rowText(rule, "target_type") != "pool" || rule["enabled"] != false {
		t.Fatalf("normalized rule = %+v", rule)
	}
	if rowText(rule, "created_at") != "2026-06-30T01:02:03Z" || rowText(rule, "updated_at") != "2026-07-01T08:00:00.123456Z" {
		t.Fatalf("rule times = created:%v updated:%v", rule["created_at"], rule["updated_at"])
	}
	pool := pools[0]
	members := pool["members"].([]any)
	if rowText(pool, "pool_id") != "pool-new" || rowText(pool, "strategy_type") != "ratio" || len(members) != 1 {
		t.Fatalf("normalized pool = %+v", pool)
	}
	if member := members[0].(map[string]any); member["assignee_id"] != "cs-001" || member["weight"] != float64(2) {
		t.Fatalf("member = %+v", member)
	}
	if store.writes[assignmentPoolStatePrefix+"pool-old"] != "{}" || store.writes[assignmentPoolStatePrefix+"pool-new"] != "{}" {
		t.Fatalf("pool state writes = %+v", store.writes)
	}
	if !reflect.DeepEqual(runtime.poolIDs, []string{"pool-new", "pool-old"}) {
		t.Fatalf("runtime pool IDs = %+v", runtime.poolIDs)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "config" || !strings.Contains(audit.entries[0].Detail, "rules=1, pools=1") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if len(events.events) != 1 || events.events[0].event != "assignment.config.updated" || events.events[0].topic != "assignment.config" || events.events[0].payload["rules_count"] != 1 {
		t.Fatalf("events = %+v", events.events)
	}
	var savedRules []map[string]any
	if err := json.Unmarshal([]byte(store.writes[assignmentRulesKey]), &savedRules); err != nil {
		t.Fatalf("saved rules json error: %v", err)
	}
	if savedRules[0]["target_value"] != "pool-new" {
		t.Fatalf("saved rules = %+v", savedRules)
	}
}

func TestServiceUpdateAssignmentConfigRejectsMissingPoolReference(t *testing.T) {
	store := &fakeAssignmentConfigStore{values: map[string]string{assignmentPoolsKey: `[]`}}
	service := Service{AssignmentCfg: store, AssignmentConfigWriteStore: store}

	_, err := service.UpdateAssignmentConfig(context.Background(), NewAssignmentConfigUpdateRequest(AssignmentConfigUpdateBody{
		Rules: []map[string]any{{
			"rule_id":      "rule-001",
			"name":         "VIP",
			"field_name":   "sender_name",
			"target_type":  "pool",
			"target_value": "pool-missing",
		}},
		Pools: []map[string]any{},
	}, auth.Session{}))

	var validation AssignmentConfigValidationError
	if !errors.As(err, &validation) || validation.Error() != "rule.target_value references missing pool: pool-missing" {
		t.Fatalf("error = %v", err)
	}
	if len(store.writes) != 0 {
		t.Fatalf("writes should be empty on validation failure: %+v", store.writes)
	}
}

func TestServiceUpdateAssignmentConfigFailsClosedWithoutWriteStore(t *testing.T) {
	_, err := (Service{AssignmentCfg: fakeAssignmentConfigStore{values: map[string]string{}}}).UpdateAssignmentConfig(context.Background(), AssignmentConfigUpdateRequest{})
	if err != ErrAssignmentConfigStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrAssignmentConfigStoreUnavailable)
	}
}

type fakeAssignmentConfigStore struct {
	values map[string]string
	writes map[string]string
}

func (store fakeAssignmentConfigStore) GetAssignmentConfigValue(ctx context.Context, key string) (string, error) {
	return store.values[key], nil
}

func (store *fakeAssignmentConfigStore) SetAssignmentConfigValue(ctx context.Context, key string, value string) error {
	if store.writes == nil {
		store.writes = map[string]string{}
	}
	if store.values == nil {
		store.values = map[string]string{}
	}
	store.writes[key] = value
	store.values[key] = value
	return nil
}

type fakeAssignmentPoolRuntime struct {
	poolIDs []string
}

func (runtime *fakeAssignmentPoolRuntime) ResetAssignmentPoolRuntime(ctx context.Context, poolIDs []string) error {
	runtime.poolIDs = append([]string{}, poolIDs...)
	return nil
}
