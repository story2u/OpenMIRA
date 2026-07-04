// Assignment auto-assign migrates the guarded admin/supervisor bulk assignment
// path. It keeps rule matching and capacity checks in Go while Redis pool
// runtime queues and archive identity enrichment stay outside this candidate.
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"wework-go/internal/auth"
)

var (
	ErrAssignmentAutoStoreUnavailable = errors.New("workbench assignment auto assign store is unavailable")
	ErrNoEnabledCSUsers               = errors.New("no enabled cs users")
)

// AssignmentAutoCandidateStore loads bounded unassigned unread projection rows.
type AssignmentAutoCandidateStore interface {
	ListAutoAssignCandidates(ctx context.Context, tenantID string, limit int) ([]ProjectionRow, error)
}

// AssignmentAutoAssignBody is the JSON input for POST /assignments/auto-assign.
type AssignmentAutoAssignBody struct {
	Limit int `json:"limit"`
}

// AssignmentAutoAssignRequest carries normalized bulk auto-assign input.
type AssignmentAutoAssignRequest struct {
	Session auth.Session
	Limit   int
}

type assignmentAutoDecision struct {
	AssigneeID   string
	AssigneeName string
	RuleID       any
	RuleName     any
	PoolID       any
	StrategyType string
	StatePayload map[string]any
	FallbackUsed bool
}

// NewAssignmentAutoAssignRequest applies Python's 1..1000 limit clamp.
func NewAssignmentAutoAssignRequest(body AssignmentAutoAssignBody, session auth.Session) AssignmentAutoAssignRequest {
	limit := body.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	return AssignmentAutoAssignRequest{Session: session, Limit: limit}
}

// AutoAssignAssignments builds POST /api/v1/assignments/auto-assign.
func (service Service) AutoAssignAssignments(ctx context.Context, request AssignmentAutoAssignRequest) (Payload, error) {
	if service.CSUsers == nil || service.AssignmentCfg == nil {
		return nil, ErrAssignmentAutoStoreUnavailable
	}
	candidates := service.assignmentAutoCandidateStore()
	counts := service.assignmentCountStore()
	writes := service.assignmentWriteStore()
	if candidates == nil || counts == nil || writes == nil {
		return nil, ErrAssignmentAutoStoreUnavailable
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return nil, err
	}
	users = enabledAssignmentAutoUsers(users)
	if len(users) == 0 {
		return nil, ErrNoEnabledCSUsers
	}
	tenantID := sessionTenantID(request.Session)
	assigneeIDs := make([]string, 0, len(users))
	for _, user := range users {
		assigneeIDs = appendUniqueStrings(assigneeIDs, user.AssigneeID)
	}
	loadMap, err := service.assignmentLoadMap(ctx, counts, assigneeIDs, tenantID)
	if err != nil {
		return nil, err
	}
	rules, err := service.assignmentAutoRules(ctx)
	if err != nil {
		return nil, err
	}
	pools, err := service.assignmentAutoPools(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := candidates.ListAutoAssignCandidates(ctx, tenantID, assignmentAutoProjectionLimit(request.Limit))
	if err != nil {
		return nil, err
	}
	if len(rows) > request.Limit {
		rows = rows[:request.Limit]
	}
	assignedRows := make([]ProjectionRow, 0)
	skippedRows := make([]ProjectionRow, 0)
	for _, row := range rows {
		conversationID := rowText(row, "conversation_id")
		if conversationID == "" {
			continue
		}
		decision := service.chooseAssignmentAutoDecision(ctx, assignmentAutoContext(row), rules, pools, users, loadMap)
		if decision == nil {
			skippedRows = append(skippedRows, ProjectionRow{"conversation_id": conversationID, "reason": "no assignee capacity"})
			continue
		}
		record, err := writes.ClaimAssignment(ctx, AssignmentClaimCommand{
			ConversationID: conversationID,
			AssigneeID:     decision.AssigneeID,
			AssigneeName:   decision.AssigneeName,
			Force:          false,
			TenantID:       firstNonBlank(rowText(row, "tenant_id"), tenantID),
		})
		if err != nil {
			if errors.As(assignmentWriteError(err), &AssignmentConflictError{}) {
				skippedRows = append(skippedRows, ProjectionRow{"conversation_id": conversationID, "reason": "already assigned"})
				continue
			}
			return nil, assignmentWriteError(err)
		}
		service.syncAssignmentClaimState(ctx, record)
		if err := service.commitAssignmentAutoDecision(ctx, decision); err != nil {
			return nil, err
		}
		loadMap[decision.AssigneeID] = loadMap[decision.AssigneeID] + 1
		assignmentPayload := assignmentRecordPayload(record)
		if service.AssignmentEvents != nil {
			if err := service.AssignmentEvents.Publish(ctx, "conversations", "conversation.assigned", "conversation.assignment", assignmentEventPayload(record)); err != nil {
				return nil, err
			}
		}
		assignmentPayload["decision"] = decision.Payload()
		assignedRows = append(assignedRows, assignmentPayload)
	}
	if len(assignedRows) > 0 {
		service.invalidateAllReadModelNamespaces(ctx)
	}
	service.recordAssignmentAudit(ctx, request.Session, fmt.Sprintf("自动分配会话: assigned=%d, skipped=%d", len(assignedRows), len(skippedRows)))
	return Payload{
		"success":        true,
		"assigned_count": len(assignedRows),
		"skipped_count":  len(skippedRows),
		"assignments":    assignedRows,
		"skipped":        skippedRows,
	}, nil
}

func (decision assignmentAutoDecision) Payload() ProjectionRow {
	return ProjectionRow{
		"rule_id":       decision.RuleID,
		"rule_name":     decision.RuleName,
		"pool_id":       decision.PoolID,
		"strategy_type": decision.StrategyType,
	}
}

func (service Service) assignmentAutoCandidateStore() AssignmentAutoCandidateStore {
	if service.Projection == nil {
		return nil
	}
	store, ok := service.Projection.(AssignmentAutoCandidateStore)
	if !ok {
		return nil
	}
	return store
}

func (service Service) assignmentLoadMap(ctx context.Context, counts AssignmentCountStore, assigneeIDs []string, tenantID string) (map[string]int, error) {
	counter, ok := service.AssignmentRuntimeState.(AssignmentRuntimeLoadCounter)
	if !ok || counter == nil {
		return counts.CountByAssigneeIDs(ctx, assigneeIDs, tenantID)
	}
	cached, missing, err := counter.CountAssignmentLoadState(ctx, tenantID, assigneeIDs)
	if err != nil || len(cached) == 0 {
		return counts.CountByAssigneeIDs(ctx, assigneeIDs, tenantID)
	}
	output := make(map[string]int, len(assigneeIDs))
	for assigneeID, count := range cached {
		output[strings.TrimSpace(assigneeID)] = count
	}
	if len(missing) > 0 {
		loaded, err := counts.CountByAssigneeIDs(ctx, missing, tenantID)
		if err != nil {
			return nil, err
		}
		for assigneeID, count := range loaded {
			output[strings.TrimSpace(assigneeID)] = count
		}
	}
	for _, assigneeID := range assigneeIDs {
		assigneeID = strings.TrimSpace(assigneeID)
		if assigneeID == "" {
			continue
		}
		if _, ok := output[assigneeID]; !ok {
			output[assigneeID] = 0
		}
	}
	return output, nil
}

func assignmentAutoProjectionLimit(limit int) int {
	if limit <= 0 {
		limit = 1
	}
	value := limit * 3
	if value < 60 {
		value = 60
	}
	if value > 1000 {
		value = 1000
	}
	return value
}

func enabledAssignmentAutoUsers(users []CSUserRecord) []CSUserRecord {
	output := make([]CSUserRecord, 0, len(users))
	for _, user := range users {
		if !user.Enabled || strings.TrimSpace(user.AssigneeID) == "" {
			continue
		}
		role := strings.TrimSpace(user.Role)
		if role == "" {
			role = "cs"
		}
		if role != "cs" {
			continue
		}
		output = append(output, user)
	}
	return output
}

func (service Service) assignmentAutoRules(ctx context.Context) ([]ProjectionRow, error) {
	raw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentRulesKey)
	if err != nil {
		return nil, err
	}
	rules := decodeAssignmentConfigRows(raw)
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
	return rules, nil
}

func (service Service) assignmentAutoPools(ctx context.Context) (map[string]ProjectionRow, error) {
	raw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentPoolsKey)
	if err != nil {
		return nil, err
	}
	rows := decodeAssignmentConfigRows(raw)
	pools := make(map[string]ProjectionRow, len(rows))
	for _, row := range rows {
		if poolID := rowText(row, "pool_id"); poolID != "" {
			pools[poolID] = row
		}
	}
	return pools, nil
}

func (service Service) chooseAssignmentAutoDecision(ctx context.Context, contextRow ProjectionRow, rules []ProjectionRow, pools map[string]ProjectionRow, users []CSUserRecord, loadMap map[string]int) *assignmentAutoDecision {
	userMap := map[string]CSUserRecord{}
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		if assigneeID != "" {
			userMap[assigneeID] = user
		}
	}
	for _, rule := range rules {
		if !assignmentRowBoolDefault(rule, "enabled", true) || !assignmentRuleMatches(rule, contextRow) {
			continue
		}
		targetValue := rowText(rule, "target_value")
		if rowText(rule, "target_type") == "assignee" {
			user, ok := userMap[targetValue]
			if ok && assignmentAutoUserAvailable(user, loadMap) {
				return &assignmentAutoDecision{
					AssigneeID:   strings.TrimSpace(user.AssigneeID),
					AssigneeName: strings.TrimSpace(user.AssigneeName),
					RuleID:       rowText(rule, "rule_id"),
					RuleName:     rowText(rule, "name"),
					PoolID:       nil,
					StrategyType: "direct_assignee",
				}
			}
			continue
		}
		pool := pools[targetValue]
		if len(pool) == 0 || !assignmentRowBoolDefault(pool, "enabled", true) {
			continue
		}
		decision := service.selectAssignmentAutoPoolUser(ctx, pool, userMap, loadMap)
		if decision == nil {
			continue
		}
		decision.RuleID = rowText(rule, "rule_id")
		decision.RuleName = rowText(rule, "name")
		decision.PoolID = rowText(pool, "pool_id")
		decision.StrategyType = assignmentPoolStrategy(pool)
		return decision
	}
	return pickAssignmentAutoFallback(users, loadMap)
}

func assignmentAutoContext(row ProjectionRow) ProjectionRow {
	conversationID := rowText(row, "conversation_id")
	return ProjectionRow{
		"conversation_id":        conversationID,
		"conversation_key":       firstNonBlank(rowText(row, "resolved_conversation_id"), rowText(row, "conversation_key"), conversationID),
		"tenant_id":              rowText(row, "tenant_id"),
		"wework_user_id":         firstNonBlank(rowText(row, "wework_user_id"), rowText(row, "account_wework_user_id")),
		"device_id":              firstNonBlank(rowText(row, "device_id"), rowText(row, "resolved_device_scope")),
		"sender_id":              firstNonBlank(rowText(row, "sender_id"), rowText(row, "external_userid")),
		"sender_name":            firstNonBlank(rowText(row, "sender_name"), rowText(row, "customer_name"), rowText(row, "conversation_name")),
		"sender_remark":          rowText(row, "sender_remark"),
		"conversation_name":      firstNonBlank(rowText(row, "conversation_name"), rowText(row, "customer_name")),
		"account_id":             rowText(row, "account_id"),
		"account_name":           rowText(row, "account_name"),
		"account_device_id":      rowText(row, "account_device_id"),
		"account_wework_user_id": rowText(row, "account_wework_user_id"),
		"enterprise_id":          firstNonBlank(rowText(row, "enterprise_id"), rowText(row, "tenant_id")),
		"organization_name":      rowText(row, "organization_name"),
	}
}

func assignmentRuleMatches(rule ProjectionRow, contextRow ProjectionRow) bool {
	text := rowText(contextRow, rowText(rule, "field_name"))
	matchValue := rowText(rule, "match_value")
	switch rowText(rule, "operator") {
	case "empty":
		return text == ""
	case "contains":
		return matchValue != "" && strings.Contains(text, matchValue)
	case "starts_with":
		return matchValue != "" && strings.HasPrefix(text, matchValue)
	case "in":
		values := strings.Split(matchValue, ",")
		for _, value := range values {
			if strings.TrimSpace(value) == text && text != "" {
				return true
			}
		}
		return false
	default:
		return text == matchValue
	}
}

func (service Service) selectAssignmentAutoPoolUser(ctx context.Context, pool ProjectionRow, userMap map[string]CSUserRecord, loadMap map[string]int) *assignmentAutoDecision {
	members := assignmentPoolMembers(pool)
	available := make([]CSUserRecord, 0, len(members))
	for _, member := range members {
		user, ok := userMap[member.AssigneeID]
		if ok && assignmentAutoUserAvailable(user, loadMap) {
			available = append(available, user)
		}
	}
	if len(available) == 0 {
		return nil
	}
	if assignmentPoolStrategy(pool) == "ratio" {
		return service.selectAssignmentAutoRatio(ctx, pool, members, available)
	}
	return service.selectAssignmentAutoRoundRobin(ctx, pool, members, available)
}

func (service Service) selectAssignmentAutoRoundRobin(ctx context.Context, pool ProjectionRow, members []assignmentPoolMember, available []CSUserRecord) *assignmentAutoDecision {
	if selector := service.AssignmentPoolRuntimeSelector; selector != nil {
		selectedID, ok, err := selector.SelectRoundRobinPoolUser(ctx, rowText(pool, "pool_id"), assignmentPoolMemberIDs(members), assignmentAutoUserIDs(available))
		if err == nil && ok {
			if selected, found := assignmentAutoUserByID(available, selectedID); found {
				return &assignmentAutoDecision{
					AssigneeID:   strings.TrimSpace(selected.AssigneeID),
					AssigneeName: strings.TrimSpace(selected.AssigneeName),
				}
			}
		}
	}
	state := service.assignmentPoolState(ctx, rowText(pool, "pool_id"))
	nextIndex := maxInt(0, rowInt(state, "next_index"))
	chosenIndex := nextIndex % len(available)
	user := available[chosenIndex]
	return &assignmentAutoDecision{
		AssigneeID:   strings.TrimSpace(user.AssigneeID),
		AssigneeName: strings.TrimSpace(user.AssigneeName),
		StatePayload: map[string]any{"next_index": (chosenIndex + 1) % len(available)},
	}
}

func (service Service) selectAssignmentAutoRatio(ctx context.Context, pool ProjectionRow, members []assignmentPoolMember, available []CSUserRecord) *assignmentAutoDecision {
	if selector := service.AssignmentPoolRuntimeSelector; selector != nil {
		selectedID, ok, err := selector.SelectRatioPoolUser(ctx, rowText(pool, "pool_id"), assignmentPoolWeights(members), assignmentAutoUserIDs(available))
		if err == nil && ok {
			if selected, found := assignmentAutoUserByID(available, selectedID); found {
				return &assignmentAutoDecision{
					AssigneeID:   strings.TrimSpace(selected.AssigneeID),
					AssigneeName: strings.TrimSpace(selected.AssigneeName),
				}
			}
		}
	}
	state := service.assignmentPoolState(ctx, rowText(pool, "pool_id"))
	counts := map[string]int{}
	if rawCounts, ok := state["counts"].(map[string]any); ok {
		for assigneeID, value := range rawCounts {
			counts[strings.TrimSpace(assigneeID)] = anyInt(value)
		}
	}
	weights := map[string]int{}
	for _, member := range members {
		weights[member.AssigneeID] = maxInt(1, member.Weight)
	}
	type candidateScore struct {
		score      float64
		assigneeID string
		user       CSUserRecord
	}
	scores := make([]candidateScore, 0, len(available))
	for _, user := range available {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		weight := maxInt(1, weights[assigneeID])
		scores = append(scores, candidateScore{score: float64(counts[assigneeID]) / float64(weight), assigneeID: assigneeID, user: user})
	}
	sort.SliceStable(scores, func(left int, right int) bool {
		if scores[left].score != scores[right].score {
			return scores[left].score < scores[right].score
		}
		return scores[left].assigneeID < scores[right].assigneeID
	})
	selected := scores[0].user
	selectedID := strings.TrimSpace(selected.AssigneeID)
	counts[selectedID] = counts[selectedID] + 1
	payloadCounts := map[string]any{}
	for key, value := range counts {
		payloadCounts[key] = value
	}
	return &assignmentAutoDecision{
		AssigneeID:   selectedID,
		AssigneeName: strings.TrimSpace(selected.AssigneeName),
		StatePayload: map[string]any{"counts": payloadCounts},
	}
}

func (service Service) assignmentPoolState(ctx context.Context, poolID string) ProjectionRow {
	raw, err := service.AssignmentCfg.GetAssignmentConfigValue(ctx, assignmentPoolStatePrefix+strings.TrimSpace(poolID))
	if err != nil || strings.TrimSpace(raw) == "" {
		return ProjectionRow{}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ProjectionRow{}
	}
	return ProjectionRow(decoded)
}

func (service Service) commitAssignmentAutoDecision(ctx context.Context, decision *assignmentAutoDecision) error {
	if decision == nil || len(decision.StatePayload) == 0 {
		return nil
	}
	poolID, _ := decision.PoolID.(string)
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return nil
	}
	store := service.assignmentConfigWriteStore()
	if store == nil {
		return ErrAssignmentConfigStoreUnavailable
	}
	raw, err := json.Marshal(decision.StatePayload)
	if err != nil {
		return err
	}
	return store.SetAssignmentConfigValue(ctx, assignmentPoolStatePrefix+poolID, string(raw))
}

type assignmentPoolMember struct {
	AssigneeID string
	Weight     int
}

func assignmentPoolMembers(pool ProjectionRow) []assignmentPoolMember {
	items := assignmentMapList(pool["members"])
	rows := make([]assignmentPoolMember, 0, len(items))
	for _, item := range items {
		assigneeID := assignmentItemText(item, "assignee_id")
		if assigneeID == "" {
			continue
		}
		weight, err := assignmentIntDefault(item["weight"], 1)
		if err != nil {
			weight = 1
		}
		rows = append(rows, assignmentPoolMember{AssigneeID: assigneeID, Weight: maxInt(1, weight)})
	}
	return rows
}

func assignmentPoolMemberIDs(members []assignmentPoolMember) []string {
	output := make([]string, 0, len(members))
	for _, member := range members {
		output = appendUniqueStrings(output, member.AssigneeID)
	}
	return output
}

func assignmentPoolWeights(members []assignmentPoolMember) map[string]int {
	output := make(map[string]int, len(members))
	for _, member := range members {
		assigneeID := strings.TrimSpace(member.AssigneeID)
		if assigneeID == "" {
			continue
		}
		output[assigneeID] = maxInt(1, member.Weight)
	}
	return output
}

func assignmentAutoUserIDs(users []CSUserRecord) []string {
	output := make([]string, 0, len(users))
	for _, user := range users {
		output = appendUniqueStrings(output, user.AssigneeID)
	}
	return output
}

func assignmentAutoUserByID(users []CSUserRecord, assigneeID string) (CSUserRecord, bool) {
	assigneeID = strings.TrimSpace(assigneeID)
	for _, user := range users {
		if strings.TrimSpace(user.AssigneeID) == assigneeID {
			return user, true
		}
	}
	return CSUserRecord{}, false
}

func assignmentPoolStrategy(pool ProjectionRow) string {
	strategy := rowText(pool, "strategy_type")
	if strategy == "" {
		return "round_robin"
	}
	return strategy
}

func assignmentRowBoolDefault(row ProjectionRow, key string, fallback bool) bool {
	value, ok := row[key]
	if !ok {
		return fallback
	}
	return assignmentPythonTruthy(value)
}

func assignmentAutoUserAvailable(user CSUserRecord, loadMap map[string]int) bool {
	if !user.Enabled {
		return false
	}
	maxSessions := user.MaxSessions
	current := loadMap[strings.TrimSpace(user.AssigneeID)]
	return maxSessions <= 0 || current < maxSessions
}

func pickAssignmentAutoFallback(users []CSUserRecord, loadMap map[string]int) *assignmentAutoDecision {
	available := make([]CSUserRecord, 0, len(users))
	for _, user := range users {
		if assignmentAutoUserAvailable(user, loadMap) {
			available = append(available, user)
		}
	}
	if len(available) == 0 {
		return nil
	}
	sort.SliceStable(available, func(left int, right int) bool {
		leftID := strings.TrimSpace(available[left].AssigneeID)
		rightID := strings.TrimSpace(available[right].AssigneeID)
		leftLoad := loadMap[leftID]
		rightLoad := loadMap[rightID]
		if leftLoad != rightLoad {
			return leftLoad < rightLoad
		}
		return leftID < rightID
	})
	return &assignmentAutoDecision{
		AssigneeID:   strings.TrimSpace(available[0].AssigneeID),
		AssigneeName: strings.TrimSpace(available[0].AssigneeName),
		RuleID:       nil,
		RuleName:     nil,
		PoolID:       nil,
		StrategyType: "least_load_fallback",
		FallbackUsed: true,
	}
}

func anyInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}
