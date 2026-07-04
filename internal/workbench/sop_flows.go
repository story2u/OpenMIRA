// SOP flows expose flow-level SOP configuration. Write candidates mutate only
// flow/policy config tables and publish the same sop.changed events as Python.
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrSOPFlowStoreUnavailable means SOP flow configs cannot be loaded.
	ErrSOPFlowStoreUnavailable = errors.New("workbench sop flow store is unavailable")
)

// SOPConfigValidationError maps SOP config validation details to HTTP 422.
type SOPConfigValidationError struct {
	Detail string
}

func (err SOPConfigValidationError) Error() string {
	return err.Detail
}

// SOPFlowRecord is the stable HTTP shape for sop_flow_configs rows.
type SOPFlowRecord struct {
	FlowID                string
	FlowName              string
	TargetAudience        string
	ExecutionMode         string
	DayCount              int
	PlatformPullDriver    string
	PlatformTaskLimit     int
	PlatformDispatchQueue string
	PlatformTaskURL       string
	ExecutionTimeWindows  string
	Enabled               bool
	HumanHandoffRule      string
	RiskKeywords          string
	CreatedAt             string
	UpdatedAt             string
}

// SOPFlowCommand carries one flow config upsert command.
type SOPFlowCommand struct {
	FlowID                string
	FlowName              string
	TargetAudience        string
	ExecutionMode         string
	DayCount              int
	PlatformPullDriver    string
	PlatformTaskLimit     int
	PlatformDispatchQueue string
	PlatformTaskURL       string
	ExecutionTimeWindows  string
	Enabled               bool
	HumanHandoffRule      string
	RiskKeywords          string
}

// SOPFlowsRequest carries the authenticated management session.
type SOPFlowsRequest struct {
	Session auth.Session
}

// SOPFlowUpsertBody is the JSON input for POST /admin/sop/flows.
type SOPFlowUpsertBody struct {
	FlowID                string           `json:"flow_id"`
	FlowName              *string          `json:"flow_name"`
	TargetAudience        string           `json:"target_audience"`
	ExecutionMode         string           `json:"execution_mode"`
	DayCount              int              `json:"day_count"`
	PlatformPullDriver    string           `json:"platform_pull_driver"`
	PlatformTaskLimit     *int             `json:"platform_task_limit"`
	PlatformDispatchQueue string           `json:"platform_dispatch_queue"`
	PlatformTaskURL       string           `json:"platform_task_url"`
	ExecutionTimeWindows  []map[string]any `json:"execution_time_windows"`
	Enabled               bool             `json:"enabled"`
	HumanHandoffRule      string           `json:"human_handoff_rule"`
	RiskKeywords          string           `json:"risk_keywords"`
}

// SOPFlowUpsertRequest carries the legacy POST request body.
type SOPFlowUpsertRequest struct {
	Session auth.Session
	Command SOPFlowCommand
}

// SOPFlowDeleteRequest carries the legacy DELETE path parameter.
type SOPFlowDeleteRequest struct {
	Session auth.Session
	FlowID  string
}

// NewSOPFlowsRequest normalizes the SOP flows request boundary.
func NewSOPFlowsRequest(session auth.Session) SOPFlowsRequest {
	return SOPFlowsRequest{Session: session}
}

// NewSOPFlowUpsertRequest normalizes the flow upsert request boundary.
func NewSOPFlowUpsertRequest(body SOPFlowUpsertBody, session auth.Session) SOPFlowUpsertRequest {
	flowID := strings.TrimSpace(body.FlowID)
	if flowID == "" {
		flowID = "default"
	}
	executionMode := strings.TrimSpace(body.ExecutionMode)
	if executionMode == "" {
		executionMode = "local_days"
	}
	platformPullDriver := strings.TrimSpace(body.PlatformPullDriver)
	if platformPullDriver == "" {
		platformPullDriver = "conversation"
	}
	if platformPullDriver != "conversation" && platformPullDriver != "platform_task" {
		platformPullDriver = "conversation"
	}
	platformTaskLimit := 20
	if body.PlatformTaskLimit != nil {
		platformTaskLimit = *body.PlatformTaskLimit
	}
	flowName := "default"
	if body.FlowName != nil {
		flowName = defaultText(strings.TrimSpace(*body.FlowName), flowID)
	}
	return SOPFlowUpsertRequest{
		Session: session,
		Command: SOPFlowCommand{
			FlowID:                flowID,
			FlowName:              flowName,
			TargetAudience:        strings.TrimSpace(body.TargetAudience),
			ExecutionMode:         executionMode,
			DayCount:              maxInt(1, body.DayCount),
			PlatformPullDriver:    platformPullDriver,
			PlatformTaskLimit:     maxInt(1, platformTaskLimit),
			PlatformDispatchQueue: defaultText(strings.TrimSpace(body.PlatformDispatchQueue), "slow"),
			PlatformTaskURL:       strings.TrimSpace(body.PlatformTaskURL),
			ExecutionTimeWindows:  normalizeSOPExecutionTimeWindows(body.ExecutionTimeWindows),
			Enabled:               body.Enabled,
			HumanHandoffRule:      strings.TrimSpace(body.HumanHandoffRule),
			RiskKeywords:          strings.TrimSpace(body.RiskKeywords),
		},
	}
}

// NewSOPFlowDeleteRequest normalizes the flow delete path parameter.
func NewSOPFlowDeleteRequest(flowID string, session auth.Session) SOPFlowDeleteRequest {
	return SOPFlowDeleteRequest{Session: session, FlowID: strings.TrimSpace(flowID)}
}

// SOPFlows builds the read-only /api/v1/admin/sop/flows payload.
func (service Service) SOPFlows(ctx context.Context, request SOPFlowsRequest) (Payload, error) {
	if service.SOPFlowStore == nil {
		return nil, ErrSOPFlowStoreUnavailable
	}
	flows, err := service.SOPFlowStore.ListSOPFlows(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"flows": sopFlowPayload(flows)}, nil
}

// UpsertSOPFlow handles POST /api/v1/admin/sop/flows.
func (service Service) UpsertSOPFlow(ctx context.Context, request SOPFlowUpsertRequest) (Payload, error) {
	store := service.sopFlowWriteStore()
	if store == nil || service.SOPFlowStore == nil {
		return nil, ErrSOPFlowStoreUnavailable
	}
	flows, err := service.SOPFlowStore.ListSOPFlows(ctx)
	if err != nil {
		return nil, err
	}
	command, err := service.normalizeSOPFlowCommand(request.Command, flows)
	if err != nil {
		return nil, err
	}
	flow, err := store.UpsertSOPFlow(ctx, command)
	if err != nil {
		return nil, err
	}
	payload := sopFlowRecordPayload(flow)
	if service.SOPEvents != nil {
		if err := service.SOPEvents.Publish(ctx, "devices", "sop.flow.updated", "sop.changed", map[string]any(payload)); err != nil {
			return nil, err
		}
	}
	if service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: fmt.Sprintf("新增/更新SOP流程配置: %s", command.FlowID)}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": true, "flow": payload}, nil
}

// DeleteSOPFlow handles DELETE /api/v1/admin/sop/flows/{flow_id}.
func (service Service) DeleteSOPFlow(ctx context.Context, request SOPFlowDeleteRequest) (Payload, error) {
	store := service.sopFlowWriteStore()
	if store == nil {
		return nil, ErrSOPFlowStoreUnavailable
	}
	flowID := strings.TrimSpace(request.FlowID)
	if flowID == "" {
		return nil, SOPConfigValidationError{Detail: "flow_id is required"}
	}
	if flowID == "default" {
		return nil, SOPConfigValidationError{Detail: "default flow cannot be deleted"}
	}
	deleted, err := store.DeleteSOPFlow(ctx, flowID)
	if err != nil {
		return nil, err
	}
	if deleted && service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: fmt.Sprintf("删除SOP流程配置: %s", flowID)}); err != nil {
			return nil, err
		}
	}
	if deleted && service.SOPEvents != nil {
		if err := service.SOPEvents.Publish(ctx, "devices", "sop.flow.deleted", "sop.changed", map[string]any{"flow_id": flowID}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": deleted}, nil
}

func sopFlowPayload(flows []SOPFlowRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(flows))
	for _, flow := range flows {
		row := sopFlowRecordPayload(flow)
		if row != nil {
			payload = append(payload, row)
		}
	}
	return payload
}

func sopFlowRecordPayload(flow SOPFlowRecord) ProjectionRow {
	flowID := strings.TrimSpace(flow.FlowID)
	if flowID == "" {
		return nil
	}
	return ProjectionRow{
		"flow_id":                 flowID,
		"flow_name":               strings.TrimSpace(flow.FlowName),
		"target_audience":         strings.TrimSpace(flow.TargetAudience),
		"execution_mode":          strings.TrimSpace(flow.ExecutionMode),
		"day_count":               flow.DayCount,
		"platform_pull_driver":    strings.TrimSpace(flow.PlatformPullDriver),
		"platform_task_limit":     flow.PlatformTaskLimit,
		"platform_dispatch_queue": strings.TrimSpace(flow.PlatformDispatchQueue),
		"platform_task_url":       strings.TrimSpace(flow.PlatformTaskURL),
		"execution_time_windows":  strings.TrimSpace(flow.ExecutionTimeWindows),
		"enabled":                 flow.Enabled,
		"human_handoff_rule":      strings.TrimSpace(flow.HumanHandoffRule),
		"risk_keywords":           strings.TrimSpace(flow.RiskKeywords),
		"created_at":              nilIfBlank(strings.TrimSpace(flow.CreatedAt)),
		"updated_at":              nilIfBlank(strings.TrimSpace(flow.UpdatedAt)),
	}
}

func (service Service) normalizeSOPFlowCommand(command SOPFlowCommand, flows []SOPFlowRecord) (SOPFlowCommand, error) {
	command.FlowID = defaultText(strings.TrimSpace(command.FlowID), "default")
	command.FlowName = defaultText(strings.TrimSpace(command.FlowName), command.FlowID)
	command.ExecutionMode = defaultText(strings.TrimSpace(command.ExecutionMode), "local_days")
	command.DayCount = maxInt(1, command.DayCount)
	if command.PlatformPullDriver != "conversation" && command.PlatformPullDriver != "platform_task" {
		command.PlatformPullDriver = "conversation"
	}
	command.PlatformTaskLimit = maxInt(1, command.PlatformTaskLimit)
	command.PlatformDispatchQueue = defaultText(strings.TrimSpace(command.PlatformDispatchQueue), "slow")
	current, currentOK := findExactSOPFlow(flows, command.FlowID)
	targetAudience := normalizeSOPTargetAudience(command.TargetAudience, currentOK && strings.TrimSpace(current.TargetAudience) == "")
	command.TargetAudience = targetAudience
	candidateSelection := parseSOPTargetAudienceSelection(targetAudience, false)
	if command.Enabled && candidateSelection.mode == "none" {
		return command, SOPConfigValidationError{Detail: "启用规则集前请先选择客服，或点击全部客服"}
	}
	currentMode := ""
	currentSelection := sopTargetAudienceSelection{mode: "none", ids: map[string]bool{}}
	if currentOK {
		currentMode = defaultText(strings.TrimSpace(current.ExecutionMode), "local_days")
		currentSelection = parseSOPTargetAudienceSelection(current.TargetAudience, true)
	}
	modeChanged := !currentOK || currentMode != command.ExecutionMode
	audienceChanged := !currentOK || !sameSOPTargetAudience(currentSelection, candidateSelection)
	if modeChanged || audienceChanged {
		for _, existing := range flows {
			existingFlowID := strings.TrimSpace(existing.FlowID)
			if existingFlowID == "" || existingFlowID == command.FlowID {
				continue
			}
			existingMode := defaultText(strings.TrimSpace(existing.ExecutionMode), "local_days")
			if existingMode != command.ExecutionMode {
				continue
			}
			existingSelection := parseSOPTargetAudienceSelection(existing.TargetAudience, true)
			if candidateSelection.mode == "none" || existingSelection.mode == "none" {
				continue
			}
			if sopTargetAudienceOverlaps(candidateSelection, existingSelection) {
				modeLabel := "本地规则"
				if command.ExecutionMode == "platform_pull" {
					modeLabel = "接口拉任务"
				}
				return command, SOPConfigValidationError{Detail: fmt.Sprintf("%s不能共用客服：与规则集 %s 存在冲突，请调整适用客服", modeLabel, existingFlowID)}
			}
		}
	}
	return command, nil
}

func normalizeSOPExecutionTimeWindows(items []map[string]any) string {
	windows := make([]ProjectionRow, 0, len(items))
	for _, item := range items {
		start := anyText(item["start"])
		end := anyText(item["end"])
		if start == "" || end == "" {
			continue
		}
		if _, err := time.Parse("15:04", start); err != nil {
			continue
		}
		if _, err := time.Parse("15:04", end); err != nil {
			continue
		}
		windows = append(windows, ProjectionRow{"start": start, "end": end})
	}
	raw, err := json.Marshal(windows)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

type sopTargetAudienceSelection struct {
	mode string
	ids  map[string]bool
}

func parseSOPTargetAudienceSelection(value string, legacyEmptyIsAll bool) sopTargetAudienceSelection {
	normalized := normalizeSOPTargetAudience(value, legacyEmptyIsAll)
	switch normalized {
	case defaultTargetAudienceAll:
		return sopTargetAudienceSelection{mode: "all", ids: map[string]bool{}}
	case defaultTargetAudienceNone:
		return sopTargetAudienceSelection{mode: "none", ids: map[string]bool{}}
	default:
		ids := map[string]bool{}
		for _, item := range strings.Split(normalized, ",") {
			if candidate := strings.TrimSpace(item); candidate != "" {
				ids[candidate] = true
			}
		}
		return sopTargetAudienceSelection{mode: "specific", ids: ids}
	}
}

func normalizeSOPTargetAudience(value string, legacyEmptyIsAll bool) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		if legacyEmptyIsAll {
			return defaultTargetAudienceAll
		}
		return defaultTargetAudienceNone
	}
	if normalized == defaultTargetAudienceAll || normalized == defaultTargetAudienceNone {
		return normalized
	}
	normalized = strings.NewReplacer("\n", ",", "，", ",", "；", ",").Replace(normalized)
	parts := strings.Split(normalized, ",")
	values := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
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

func sameSOPTargetAudience(left sopTargetAudienceSelection, right sopTargetAudienceSelection) bool {
	if left.mode != right.mode || len(left.ids) != len(right.ids) {
		return false
	}
	for id := range left.ids {
		if !right.ids[id] {
			return false
		}
	}
	return true
}

func sopTargetAudienceOverlaps(left sopTargetAudienceSelection, right sopTargetAudienceSelection) bool {
	if left.mode == "all" || right.mode == "all" {
		return true
	}
	for id := range left.ids {
		if right.ids[id] {
			return true
		}
	}
	return false
}

func findSOPFlow(flows []SOPFlowRecord, flowID string) (SOPFlowRecord, bool) {
	normalized := defaultText(strings.TrimSpace(flowID), "default")
	var fallback SOPFlowRecord
	hasFallback := false
	for _, flow := range flows {
		currentID := defaultText(strings.TrimSpace(flow.FlowID), "default")
		if currentID == normalized {
			return flow, true
		}
		if currentID == "default" {
			fallback = flow
			hasFallback = true
		}
	}
	return fallback, hasFallback && normalized != "default"
}

func findExactSOPFlow(flows []SOPFlowRecord, flowID string) (SOPFlowRecord, bool) {
	normalized := defaultText(strings.TrimSpace(flowID), "default")
	for _, flow := range flows {
		if defaultText(strings.TrimSpace(flow.FlowID), "default") == normalized {
			return flow, true
		}
	}
	return SOPFlowRecord{}, false
}

func (service Service) sopFlowWriteStore() SOPFlowWriteStore {
	if service.SOPFlowWriteStore != nil {
		return service.SOPFlowWriteStore
	}
	if store, ok := service.SOPFlowStore.(SOPFlowWriteStore); ok {
		return store
	}
	return nil
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
