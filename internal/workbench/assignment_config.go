// Assignment config exposes the management allocation rules and guarded config
// writes. The write candidate mirrors Python SessionAllocationService
// normalization before later assignment execution routes move to Go.
package workbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
)

const (
	assignmentRulesKey        = "assignment.config.rules"
	assignmentPoolsKey        = "assignment.config.pools"
	assignmentPoolStatePrefix = "assignment.pool_state."
)

var supportedAssignmentOperators = map[string]bool{
	"equals":      true,
	"contains":    true,
	"starts_with": true,
	"in":          true,
	"empty":       true,
}

var supportedAssignmentStrategies = map[string]bool{
	"round_robin": true,
	"ratio":       true,
}

var supportedAssignmentTargetTypes = map[string]bool{
	"assignee": true,
	"pool":     true,
}

var (
	// ErrAssignmentConfigStoreUnavailable means allocation settings cannot be loaded.
	ErrAssignmentConfigStoreUnavailable = errors.New("workbench assignment config store is unavailable")
)

// AssignmentConfigValidationError maps Python ValueError details to HTTP 422.
type AssignmentConfigValidationError struct {
	Detail string
}

func (err AssignmentConfigValidationError) Error() string {
	return err.Detail
}

// AssignmentConfigRequest carries the authenticated management session.
type AssignmentConfigRequest struct {
	Session auth.Session
}

// AssignmentConfigUpdateBody is the JSON input for POST /admin/assignment-config.
type AssignmentConfigUpdateBody struct {
	Rules []map[string]any `json:"rules"`
	Pools []map[string]any `json:"pools"`
}

// AssignmentConfigUpdateRequest carries an assignment config replacement.
type AssignmentConfigUpdateRequest struct {
	Session auth.Session
	Rules   []map[string]any
	Pools   []map[string]any
}

// NewAssignmentConfigRequest normalizes the assignment config request boundary.
func NewAssignmentConfigRequest(session auth.Session) AssignmentConfigRequest {
	return AssignmentConfigRequest{Session: session}
}

// NewAssignmentConfigUpdateRequest normalizes the update request boundary.
func NewAssignmentConfigUpdateRequest(body AssignmentConfigUpdateBody, session auth.Session) AssignmentConfigUpdateRequest {
	return AssignmentConfigUpdateRequest{
		Session: session,
		Rules:   append([]map[string]any{}, body.Rules...),
		Pools:   append([]map[string]any{}, body.Pools...),
	}
}

// AssignmentConfig builds GET /api/v1/admin/assignment-config from system settings.
func (service Service) AssignmentConfig(ctx context.Context, request AssignmentConfigRequest) (Payload, error) {
	if service.AssignmentCfg == nil {
		return nil, ErrAssignmentConfigStoreUnavailable
	}
	rulesRaw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentRulesKey)
	if err != nil {
		return nil, err
	}
	poolsRaw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentPoolsKey)
	if err != nil {
		return nil, err
	}
	rules := decodeAssignmentConfigRows(rulesRaw)
	pools := decodeAssignmentConfigRows(poolsRaw)
	sort.SliceStable(rules, func(left int, right int) bool {
		leftPriority := rowIntDefault(rules[left], "priority", 100)
		rightPriority := rowIntDefault(rules[right], "priority", 100)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftUpdated := rowText(rules[left], "updated_at")
		rightUpdated := rowText(rules[right], "updated_at")
		if leftUpdated != rightUpdated {
			return leftUpdated < rightUpdated
		}
		return rowText(rules[left], "rule_id") < rowText(rules[right], "rule_id")
	})
	sort.SliceStable(pools, func(left int, right int) bool {
		leftName := strings.ToLower(rowText(pools[left], "pool_name"))
		rightName := strings.ToLower(rowText(pools[right], "pool_name"))
		if leftName != rightName {
			return leftName < rightName
		}
		return rowText(pools[left], "pool_id") < rowText(pools[right], "pool_id")
	})
	return Payload{"rules": rules, "pools": pools}, nil
}

// UpdateAssignmentConfig handles POST /api/v1/admin/assignment-config.
func (service Service) UpdateAssignmentConfig(ctx context.Context, request AssignmentConfigUpdateRequest) (Payload, error) {
	writeStore := service.assignmentConfigWriteStore()
	if service.AssignmentCfg == nil || writeStore == nil {
		return nil, ErrAssignmentConfigStoreUnavailable
	}
	existingPoolsRaw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentPoolsKey)
	if err != nil {
		return nil, err
	}
	existingPoolIDs := assignmentPoolIDs(decodeAssignmentConfigRows(existingPoolsRaw))
	now := service.now().UTC().Truncate(time.Microsecond)
	normalizedRules, err := normalizeAssignmentConfigRules(request.Rules, now)
	if err != nil {
		return nil, err
	}
	normalizedPools, err := normalizeAssignmentConfigPools(request.Pools, now)
	if err != nil {
		return nil, err
	}
	normalizedPoolIDs := assignmentPoolIDs(normalizedPools)
	for _, rule := range normalizedRules {
		if rowText(rule, "target_type") == "pool" && !normalizedPoolIDs[rowText(rule, "target_value")] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("rule.target_value references missing pool: %s", rowText(rule, "target_value"))}
		}
	}
	rulesRaw, err := json.Marshal(normalizedRules)
	if err != nil {
		return nil, err
	}
	poolsRaw, err := json.Marshal(normalizedPools)
	if err != nil {
		return nil, err
	}
	if err := writeStore.SetAssignmentConfigValue(ctx, assignmentRulesKey, string(rulesRaw)); err != nil {
		return nil, err
	}
	if err := writeStore.SetAssignmentConfigValue(ctx, assignmentPoolsKey, string(poolsRaw)); err != nil {
		return nil, err
	}
	for poolID := range normalizedPoolIDs {
		existingPoolIDs[poolID] = true
	}
	if err := service.resetAssignmentPoolStates(ctx, existingPoolIDs); err != nil {
		return nil, err
	}
	if service.AuditLogWriter != nil {
		detail := fmt.Sprintf("更新会话分配配置: rules=%d, pools=%d", len(normalizedRules), len(normalizedPools))
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: detail}); err != nil {
			return nil, err
		}
	}
	if service.AssignmentConfigEvents != nil {
		payload := map[string]any{"rules_count": len(normalizedRules), "pools_count": len(normalizedPools)}
		if err := service.AssignmentConfigEvents.Publish(ctx, "devices", "assignment.config.updated", "assignment.config", payload); err != nil {
			return nil, err
		}
	}
	payload, err := service.AssignmentConfig(ctx, AssignmentConfigRequest{Session: request.Session})
	if err != nil {
		return nil, err
	}
	payload["success"] = true
	return payload, nil
}

func decodeAssignmentConfigRows(raw string) []ProjectionRow {
	if strings.TrimSpace(raw) == "" {
		return []ProjectionRow{}
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return []ProjectionRow{}
	}
	rows := make([]ProjectionRow, 0, len(decoded))
	for _, item := range decoded {
		if len(item) == 0 {
			continue
		}
		rows = append(rows, ProjectionRow(item))
	}
	return rows
}

func rowIntDefault(row ProjectionRow, key string, fallback int) int {
	if value := rowInt(row, key); value != 0 {
		return value
	}
	return fallback
}

func normalizeAssignmentConfigRules(items []map[string]any, now time.Time) ([]ProjectionRow, error) {
	rows := make([]ProjectionRow, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		fieldName := assignmentItemText(item, "field_name")
		targetValue := assignmentItemText(item, "target_value")
		name := assignmentItemText(item, "name")
		operator := assignmentItemTextDefault(item, "operator", "equals")
		targetType := assignmentItemTextDefault(item, "target_type", "pool")
		if name == "" {
			return nil, AssignmentConfigValidationError{Detail: "rule.name is required"}
		}
		if fieldName == "" {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("rule.field_name is required: %s", name)}
		}
		if targetValue == "" {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("rule.target_value is required: %s", name)}
		}
		if !supportedAssignmentOperators[operator] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("unsupported rule.operator: %s", operator)}
		}
		if !supportedAssignmentTargetTypes[targetType] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("unsupported rule.target_type: %s", targetType)}
		}
		ruleID := assignmentItemText(item, "rule_id")
		if ruleID == "" {
			ruleID = "rule-" + randomAssignmentHex(16)
		}
		if strings.TrimSpace(ruleID) == "" {
			return nil, AssignmentConfigValidationError{Detail: "rule.rule_id is required"}
		}
		if seen[ruleID] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("duplicate rule.rule_id: %s", ruleID)}
		}
		seen[ruleID] = true
		priority, err := assignmentIntDefault(item["priority"], 100)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ProjectionRow{
			"rule_id":      ruleID,
			"name":         name,
			"priority":     priority,
			"enabled":      assignmentBoolDefault(item, "enabled", true),
			"field_name":   fieldName,
			"operator":     operator,
			"match_value":  assignmentItemText(item, "match_value"),
			"target_type":  targetType,
			"target_value": targetValue,
			"created_at":   assignmentCoerceDatetime(item["created_at"], now),
			"updated_at":   assignmentTimeString(now),
		})
	}
	return rows, nil
}

func normalizeAssignmentConfigPools(items []map[string]any, now time.Time) ([]ProjectionRow, error) {
	rows := make([]ProjectionRow, 0, len(items))
	seenPools := map[string]bool{}
	for _, item := range items {
		poolName := assignmentItemText(item, "pool_name")
		strategyType := assignmentItemTextDefault(item, "strategy_type", "round_robin")
		if poolName == "" {
			return nil, AssignmentConfigValidationError{Detail: "pool.pool_name is required"}
		}
		if !supportedAssignmentStrategies[strategyType] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("unsupported pool.strategy_type: %s", strategyType)}
		}
		poolID := assignmentItemText(item, "pool_id")
		if poolID == "" {
			poolID = "pool-" + randomAssignmentHex(16)
		}
		if strings.TrimSpace(poolID) == "" {
			return nil, AssignmentConfigValidationError{Detail: "pool.pool_id is required"}
		}
		if seenPools[poolID] {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("duplicate pool.pool_id: %s", poolID)}
		}
		seenPools[poolID] = true
		members, err := normalizeAssignmentPoolMembers(item["members"])
		if err != nil {
			return nil, err
		}
		if len(members) == 0 {
			return nil, AssignmentConfigValidationError{Detail: fmt.Sprintf("pool.members is required: %s", poolName)}
		}
		rows = append(rows, ProjectionRow{
			"pool_id":       poolID,
			"pool_name":     poolName,
			"strategy_type": strategyType,
			"members":       members,
			"enabled":       assignmentBoolDefault(item, "enabled", true),
			"created_at":    assignmentCoerceDatetime(item["created_at"], now),
			"updated_at":    assignmentTimeString(now),
		})
	}
	return rows, nil
}

func normalizeAssignmentPoolMembers(value any) ([]ProjectionRow, error) {
	items := assignmentMapList(value)
	rows := make([]ProjectionRow, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		assigneeID := assignmentItemText(item, "assignee_id")
		if assigneeID == "" || seen[assigneeID] {
			continue
		}
		seen[assigneeID] = true
		weight, err := assignmentIntDefault(item["weight"], 1)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ProjectionRow{"assignee_id": assigneeID, "weight": maxInt(1, weight)})
	}
	return rows, nil
}

func (service Service) assignmentConfigWriteStore() AssignmentConfigWriteStore {
	if service.AssignmentConfigWriteStore != nil {
		return service.AssignmentConfigWriteStore
	}
	if store, ok := service.AssignmentCfg.(AssignmentConfigWriteStore); ok {
		return store
	}
	return nil
}

func (service Service) resetAssignmentPoolStates(ctx context.Context, poolIDs map[string]bool) error {
	store := service.assignmentConfigWriteStore()
	if store == nil {
		return ErrAssignmentConfigStoreUnavailable
	}
	ids := make([]string, 0, len(poolIDs))
	for poolID := range poolIDs {
		poolID = strings.TrimSpace(poolID)
		if poolID != "" {
			ids = append(ids, poolID)
		}
	}
	sort.Strings(ids)
	for _, poolID := range ids {
		if err := store.SetAssignmentConfigValue(ctx, assignmentPoolStatePrefix+poolID, "{}"); err != nil {
			return err
		}
	}
	if service.AssignmentPoolRuntime != nil {
		return service.AssignmentPoolRuntime.ResetAssignmentPoolRuntime(ctx, ids)
	}
	return nil
}

func assignmentPoolIDs(rows []ProjectionRow) map[string]bool {
	output := map[string]bool{}
	for _, row := range rows {
		if poolID := rowText(row, "pool_id"); poolID != "" {
			output[poolID] = true
		}
	}
	return output
}

func assignmentItemText(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	return anyText(item[key])
}

func assignmentItemTextDefault(item map[string]any, key string, fallback string) string {
	value := assignmentItemText(item, key)
	if value == "" {
		return fallback
	}
	return value
}

func assignmentBoolDefault(item map[string]any, key string, fallback bool) bool {
	if item == nil {
		return fallback
	}
	value, ok := item[key]
	if !ok {
		return fallback
	}
	return assignmentPythonTruthy(value)
}

func assignmentPythonTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func assignmentIntDefault(value any, fallback int) (int, error) {
	switch typed := value.(type) {
	case nil:
		return fallback, nil
	case bool:
		if typed {
			return 1, nil
		}
		return fallback, nil
	case int:
		if typed == 0 {
			return fallback, nil
		}
		return typed, nil
	case int32:
		if typed == 0 {
			return fallback, nil
		}
		return int(typed), nil
	case int64:
		if typed == 0 {
			return fallback, nil
		}
		return int(typed), nil
	case float64:
		if typed == 0 {
			return fallback, nil
		}
		return int(typed), nil
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return fallback, nil
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, AssignmentConfigValidationError{Detail: fmt.Sprintf("invalid literal for int() with base 10: '%s'", raw)}
		}
		return parsed, nil
	default:
		raw := strings.TrimSpace(fmt.Sprint(typed))
		if raw == "" {
			return fallback, nil
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, AssignmentConfigValidationError{Detail: fmt.Sprintf("invalid literal for int() with base 10: '%s'", raw)}
		}
		return parsed, nil
	}
}

func assignmentMapList(value any) []map[string]any {
	switch typed := value.(type) {
	case nil:
		return []map[string]any{}
	case []map[string]any:
		return typed
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	default:
		return []map[string]any{}
	}
}

func assignmentCoerceDatetime(value any, fallback time.Time) string {
	text := anyText(value)
	if text == "" {
		return assignmentTimeString(fallback)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return assignmentTimeString(parsed)
		}
	}
	return assignmentTimeString(fallback)
}

func assignmentTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

func randomAssignmentHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
