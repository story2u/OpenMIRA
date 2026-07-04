// SOP policies expose day-level SOP rules. Dispatch tasks, analytics, and
// media upload flows remain with Python while config writes move behind flags.
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrSOPPolicyStoreUnavailable means SOP policies cannot be loaded.
	ErrSOPPolicyStoreUnavailable = errors.New("workbench sop policy store is unavailable")
)

// SOPPolicyRecord is the stable HTTP shape for sop_policies rows.
type SOPPolicyRecord struct {
	PolicyID         string
	FlowID           string
	Name             string
	DayStage         string
	StageTag         string
	CustomerState    string
	DispatchQueue    string
	TriggerEvent     string
	Enabled          bool
	Priority         int
	ReplyMode        string
	PromptTemplate   string
	ReplyText        string
	ImageURLs        string
	VideoURLs        string
	MessageSequence  string
	NeedRAG          bool
	NeedAIRewrite    bool
	MediaStrategy    string
	HumanHandoffRule string
	RiskKeywords     string
	CreatedAt        string
	UpdatedAt        string
}

// SOPPolicyCommand carries one policy upsert command.
type SOPPolicyCommand struct {
	PolicyID         string
	FlowID           string
	Name             string
	DayStage         string
	StageTag         string
	CustomerState    string
	DispatchQueue    string
	TriggerEvent     string
	Enabled          bool
	Priority         int
	ReplyMode        string
	PromptTemplate   string
	ReplyText        string
	ImageURLs        string
	VideoURLs        string
	MessageSequence  string
	NeedRAG          bool
	NeedAIRewrite    bool
	MediaStrategy    string
	HumanHandoffRule string
	RiskKeywords     string
}

// SOPPoliciesRequest carries filters and the authenticated management session.
type SOPPoliciesRequest struct {
	FlowID   string
	DayStage string
	Session  auth.Session
}

// SOPPolicyUpsertBody is the JSON input for POST /admin/sop/policies.
type SOPPolicyUpsertBody struct {
	PolicyID         string `json:"policy_id"`
	FlowID           string `json:"flow_id"`
	Name             string `json:"name"`
	DayStage         string `json:"day_stage"`
	StageTag         string `json:"stage_tag"`
	CustomerState    string `json:"customer_state"`
	DispatchQueue    string `json:"dispatch_queue"`
	TriggerEvent     string `json:"trigger_event"`
	Enabled          *bool  `json:"enabled"`
	Priority         *int   `json:"priority"`
	ReplyMode        string `json:"reply_mode"`
	PromptTemplate   string `json:"prompt_template"`
	ReplyText        string `json:"reply_text"`
	ImageURLs        string `json:"image_urls"`
	VideoURLs        string `json:"video_urls"`
	MessageSequence  string `json:"message_sequence"`
	NeedRAG          bool   `json:"need_rag"`
	NeedAIRewrite    bool   `json:"need_ai_rewrite"`
	MediaStrategy    string `json:"media_strategy"`
	HumanHandoffRule string `json:"human_handoff_rule"`
	RiskKeywords     string `json:"risk_keywords"`
}

// SOPPolicyUpsertRequest carries the legacy POST request body.
type SOPPolicyUpsertRequest struct {
	Session auth.Session
	Command SOPPolicyCommand
}

// SOPPolicyDeleteRequest carries the legacy DELETE path parameter.
type SOPPolicyDeleteRequest struct {
	Session  auth.Session
	PolicyID string
}

// NewSOPPoliciesRequest normalizes /api/v1/admin/sop/policies query params.
func NewSOPPoliciesRequest(values url.Values, session auth.Session) SOPPoliciesRequest {
	return SOPPoliciesRequest{
		FlowID:   strings.TrimSpace(values.Get("flow_id")),
		DayStage: strings.TrimSpace(values.Get("day_stage")),
		Session:  session,
	}
}

// NewSOPPolicyUpsertRequest normalizes the policy upsert request boundary.
func NewSOPPolicyUpsertRequest(body SOPPolicyUpsertBody, session auth.Session) SOPPolicyUpsertRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	priority := 100
	if body.Priority != nil {
		priority = *body.Priority
	}
	return SOPPolicyUpsertRequest{
		Session: session,
		Command: SOPPolicyCommand{
			PolicyID:         strings.TrimSpace(body.PolicyID),
			FlowID:           defaultText(strings.TrimSpace(body.FlowID), "default"),
			Name:             strings.TrimSpace(body.Name),
			DayStage:         strings.TrimSpace(body.DayStage),
			StageTag:         strings.TrimSpace(body.StageTag),
			CustomerState:    defaultText(strings.TrimSpace(body.CustomerState), "undecided"),
			DispatchQueue:    defaultText(strings.TrimSpace(body.DispatchQueue), "slow"),
			TriggerEvent:     strings.TrimSpace(body.TriggerEvent),
			Enabled:          enabled,
			Priority:         priority,
			ReplyMode:        defaultText(strings.TrimSpace(body.ReplyMode), "sop_only"),
			PromptTemplate:   strings.TrimSpace(body.PromptTemplate),
			ReplyText:        strings.TrimSpace(body.ReplyText),
			ImageURLs:        strings.TrimSpace(body.ImageURLs),
			VideoURLs:        strings.TrimSpace(body.VideoURLs),
			MessageSequence:  strings.TrimSpace(body.MessageSequence),
			NeedRAG:          body.NeedRAG,
			NeedAIRewrite:    body.NeedAIRewrite,
			MediaStrategy:    defaultText(strings.TrimSpace(body.MediaStrategy), "fixed"),
			HumanHandoffRule: strings.TrimSpace(body.HumanHandoffRule),
			RiskKeywords:     strings.TrimSpace(body.RiskKeywords),
		},
	}
}

// NewSOPPolicyDeleteRequest normalizes the policy delete path parameter.
func NewSOPPolicyDeleteRequest(policyID string, session auth.Session) SOPPolicyDeleteRequest {
	return SOPPolicyDeleteRequest{Session: session, PolicyID: strings.TrimSpace(policyID)}
}

// SOPPolicies builds the read-only /api/v1/admin/sop/policies payload.
func (service Service) SOPPolicies(ctx context.Context, request SOPPoliciesRequest) (Payload, error) {
	if service.SOPPolicyStore == nil {
		return nil, ErrSOPPolicyStoreUnavailable
	}
	if service.SOPFlowStore == nil {
		return nil, ErrSOPFlowStoreUnavailable
	}
	policies, err := service.SOPPolicyStore.ListSOPPolicies(ctx)
	if err != nil {
		return nil, err
	}
	flows, err := service.SOPFlowStore.ListSOPFlows(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{
		"policies": service.filteredSOPPolicyPayload(policies, request),
		"flows":    service.sopPolicyGroupsPayload(policies, flows, request),
	}, nil
}

// UpsertSOPPolicy handles POST /api/v1/admin/sop/policies.
func (service Service) UpsertSOPPolicy(ctx context.Context, request SOPPolicyUpsertRequest) (Payload, error) {
	store := service.sopPolicyWriteStore()
	if store == nil {
		return nil, ErrSOPPolicyStoreUnavailable
	}
	command, err := service.normalizeSOPPolicyCommand(ctx, request.Command)
	if err != nil {
		return nil, err
	}
	policy, err := store.UpsertSOPPolicy(ctx, command)
	if err != nil {
		return nil, err
	}
	payload := service.sopPolicyRecordPayload(policy)
	if service.SOPEvents != nil {
		if err := service.SOPEvents.Publish(ctx, "devices", "sop.policy.updated", "sop.changed", map[string]any(payload)); err != nil {
			return nil, err
		}
	}
	if service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: fmt.Sprintf("新增/更新SOP策略: %s", command.Name)}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": true, "policy": payload}, nil
}

// DeleteSOPPolicy handles DELETE /api/v1/admin/sop/policies/{policy_id}.
func (service Service) DeleteSOPPolicy(ctx context.Context, request SOPPolicyDeleteRequest) (Payload, error) {
	store := service.sopPolicyWriteStore()
	if store == nil {
		return nil, ErrSOPPolicyStoreUnavailable
	}
	policyID := strings.TrimSpace(request.PolicyID)
	deleted, err := store.DeleteSOPPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	if deleted && service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "config", Detail: fmt.Sprintf("删除SOP策略: %s", policyID)}); err != nil {
			return nil, err
		}
	}
	if deleted && service.SOPEvents != nil {
		if err := service.SOPEvents.Publish(ctx, "devices", "sop.policy.deleted", "sop.changed", map[string]any{"policy_id": policyID}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": deleted}, nil
}

func (service Service) filteredSOPPolicyPayload(policies []SOPPolicyRecord, request SOPPoliciesRequest) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(policies))
	for _, policy := range policies {
		if !sopPolicyMatchesRequest(policy, request) {
			continue
		}
		row := service.sopPolicyRecordPayload(policy)
		if row != nil {
			payload = append(payload, row)
		}
	}
	return payload
}

func (service Service) sopPolicyGroupsPayload(policies []SOPPolicyRecord, flows []SOPFlowRecord, request SOPPoliciesRequest) []ProjectionRow {
	configsByFlow := make(map[string]SOPFlowRecord, len(flows))
	flowIDs := make(map[string]struct{}, len(flows))
	for _, flow := range flows {
		flowID := normalizeSOPFlowID(flow.FlowID)
		configsByFlow[flowID] = flow
		flowIDs[flowID] = struct{}{}
	}
	grouped := make(map[string][]SOPPolicyRecord)
	for _, policy := range policies {
		if !sopPolicyMatchesRequest(policy, request) {
			continue
		}
		flowID := normalizeSOPFlowID(policy.FlowID)
		grouped[flowID] = append(grouped[flowID], policy)
		flowIDs[flowID] = struct{}{}
	}
	if request.FlowID != "" {
		flowIDs[request.FlowID] = struct{}{}
	}
	orderedFlowIDs := make([]string, 0, len(flowIDs))
	for flowID := range flowIDs {
		orderedFlowIDs = append(orderedFlowIDs, flowID)
	}
	sort.Slice(orderedFlowIDs, func(i int, j int) bool {
		left := orderedFlowIDs[i]
		right := orderedFlowIDs[j]
		if left == "default" || right == "default" {
			return left == "default"
		}
		return left < right
	})
	payload := make([]ProjectionRow, 0, len(orderedFlowIDs))
	for _, flowID := range orderedFlowIDs {
		groupPolicies := append([]SOPPolicyRecord(nil), grouped[flowID]...)
		if request.DayStage != "" && len(groupPolicies) == 0 {
			if _, ok := configsByFlow[flowID]; !ok {
				continue
			}
		}
		sort.SliceStable(groupPolicies, func(i int, j int) bool {
			left := groupPolicies[i]
			right := groupPolicies[j]
			if left.DayStage != right.DayStage {
				return left.DayStage < right.DayStage
			}
			if left.Priority != right.Priority {
				return left.Priority < right.Priority
			}
			return left.UpdatedAt < right.UpdatedAt
		})
		var flowConfig any
		if flow, ok := configsByFlow[flowID]; ok {
			flowConfig = sopFlowRecordPayload(flow)
		}
		payload = append(payload, ProjectionRow{
			"flow_id":     flowID,
			"flow_config": flowConfig,
			"policies":    service.filteredSOPPolicyPayload(groupPolicies, SOPPoliciesRequest{}),
		})
	}
	return payload
}

func sopPolicyMatchesRequest(policy SOPPolicyRecord, request SOPPoliciesRequest) bool {
	if request.FlowID != "" && normalizeSOPFlowID(policy.FlowID) != request.FlowID {
		return false
	}
	if request.DayStage != "" && strings.TrimSpace(policy.DayStage) != request.DayStage {
		return false
	}
	return true
}

func (service Service) sopPolicyRecordPayload(policy SOPPolicyRecord) ProjectionRow {
	policyID := strings.TrimSpace(policy.PolicyID)
	if policyID == "" {
		return nil
	}
	return ProjectionRow{
		"policy_id":          policyID,
		"flow_id":            normalizeSOPFlowID(policy.FlowID),
		"name":               strings.TrimSpace(policy.Name),
		"day_stage":          strings.TrimSpace(policy.DayStage),
		"stage_tag":          strings.TrimSpace(policy.StageTag),
		"customer_state":     strings.TrimSpace(policy.CustomerState),
		"dispatch_queue":     strings.TrimSpace(policy.DispatchQueue),
		"trigger_event":      strings.TrimSpace(policy.TriggerEvent),
		"enabled":            policy.Enabled,
		"priority":           policy.Priority,
		"reply_mode":         strings.TrimSpace(policy.ReplyMode),
		"prompt_template":    strings.TrimSpace(policy.PromptTemplate),
		"reply_text":         strings.TrimSpace(policy.ReplyText),
		"image_urls":         strings.TrimSpace(policy.ImageURLs),
		"video_urls":         strings.TrimSpace(policy.VideoURLs),
		"message_sequence":   strings.TrimSpace(policy.MessageSequence),
		"need_rag":           policy.NeedRAG,
		"need_ai_rewrite":    policy.NeedAIRewrite,
		"media_strategy":     strings.TrimSpace(policy.MediaStrategy),
		"human_handoff_rule": strings.TrimSpace(policy.HumanHandoffRule),
		"risk_keywords":      strings.TrimSpace(policy.RiskKeywords),
		"created_at":         nilIfBlank(strings.TrimSpace(policy.CreatedAt)),
		"updated_at":         nilIfBlank(strings.TrimSpace(policy.UpdatedAt)),
		"messages":           service.sopPolicyMessages(policy),
	}
}

func (service Service) sopPolicyMessages(policy SOPPolicyRecord) []ProjectionRow {
	messages := parseSOPMessageSequence(policy.MessageSequence)
	if len(messages) == 0 {
		replyText := strings.TrimSpace(policy.ReplyText)
		if replyText != "" {
			messages = append(messages, sopPolicyMessage{Type: "text", Content: replyText})
		}
		for _, line := range strings.Split(policy.ImageURLs, "\n") {
			if content := strings.TrimSpace(line); content != "" {
				messages = append(messages, sopPolicyMessage{Type: "image", Content: content})
			}
		}
		for _, line := range strings.Split(policy.VideoURLs, "\n") {
			if content := strings.TrimSpace(line); content != "" {
				messages = append(messages, sopPolicyMessage{Type: "video", Content: content})
			}
		}
	}
	payload := make([]ProjectionRow, 0, len(messages))
	policyID := strings.TrimSpace(policy.PolicyID)
	if policyID == "" {
		policyID = "sop"
	}
	for index, message := range messages {
		messageType := strings.ToLower(strings.TrimSpace(message.Type))
		if messageType == "" {
			messageType = "text"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		previewURL := strings.TrimSpace(message.PreviewURL)
		if previewURL == "" {
			previewURL = content
		}
		if messageType == "image" || messageType == "video" || messageType == "file" {
			previewURL = service.sopMediaPreviewURL(content, policyID, index)
		}
		payload = append(payload, ProjectionRow{
			"type":        messageType,
			"content":     content,
			"preview_url": previewURL,
		})
	}
	return payload
}

type sopPolicyMessage struct {
	Type       string
	Content    string
	PreviewURL string
}

func parseSOPMessageSequence(raw string) []sopPolicyMessage {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil
	}
	messages := make([]sopPolicyMessage, 0, len(parsed))
	for _, item := range parsed {
		messages = append(messages, sopPolicyMessage{
			Type:       stringFromAny(item["type"]),
			Content:    stringFromAny(item["content"]),
			PreviewURL: stringFromAny(item["preview_url"]),
		})
	}
	return messages
}

func (service Service) sopMediaPreviewURL(content string, policyID string, index int) string {
	normalized := strings.TrimSpace(content)
	if normalized == "" {
		return ""
	}
	lowered := strings.ToLower(normalized)
	if strings.HasPrefix(lowered, "http://") || strings.HasPrefix(lowered, "https://") || strings.HasPrefix(lowered, "data:") {
		return normalized
	}
	if strings.HasPrefix(lowered, "local://") {
		return "/api/v1/admin/sop/media/local?object_url=" + strings.ReplaceAll(url.QueryEscape(normalized), "+", "%20")
	}
	if service.MediaURLBuilder != nil {
		return service.MediaURLBuilder.BuildAccessURL(policyID+"-"+strconv.Itoa(index), normalized)
	}
	return normalized
}

func normalizeSOPFlowID(value string) string {
	flowID := strings.TrimSpace(value)
	if flowID == "" {
		return "default"
	}
	return flowID
}

func (service Service) normalizeSOPPolicyCommand(ctx context.Context, command SOPPolicyCommand) (SOPPolicyCommand, error) {
	command.PolicyID = strings.TrimSpace(command.PolicyID)
	command.FlowID = defaultText(strings.TrimSpace(command.FlowID), "default")
	command.Name = strings.TrimSpace(command.Name)
	command.DayStage = strings.TrimSpace(command.DayStage)
	command.TriggerEvent = strings.TrimSpace(command.TriggerEvent)
	command.PromptTemplate = strings.TrimSpace(command.PromptTemplate)
	command.ReplyText = strings.TrimSpace(command.ReplyText)
	if command.Name == "" {
		return command, SOPConfigValidationError{Detail: "name is required"}
	}
	if command.DayStage == "" {
		return command, SOPConfigValidationError{Detail: "day_stage is required"}
	}
	if command.TriggerEvent == "" {
		return command, SOPConfigValidationError{Detail: "trigger_event is required"}
	}
	if command.PromptTemplate == "" && command.ReplyText == "" {
		return command, SOPConfigValidationError{Detail: "reply_text or prompt_template is required"}
	}
	if service.SOPFlowStore != nil {
		flows, err := service.SOPFlowStore.ListSOPFlows(ctx)
		if err != nil {
			return command, err
		}
		if flow, ok := findSOPFlow(flows, command.FlowID); ok && strings.TrimSpace(flow.ExecutionMode) == "platform_pull" {
			command.TriggerEvent = "friend_added"
		}
	}
	command.CustomerState = defaultText(strings.TrimSpace(command.CustomerState), "undecided")
	command.DispatchQueue = defaultText(strings.TrimSpace(command.DispatchQueue), "slow")
	command.ReplyMode = defaultText(strings.TrimSpace(command.ReplyMode), "sop_only")
	command.ImageURLs = strings.TrimSpace(command.ImageURLs)
	command.VideoURLs = strings.TrimSpace(command.VideoURLs)
	command.MessageSequence = strings.TrimSpace(command.MessageSequence)
	command.MediaStrategy = defaultText(strings.TrimSpace(command.MediaStrategy), "fixed")
	command.HumanHandoffRule = strings.TrimSpace(command.HumanHandoffRule)
	command.RiskKeywords = strings.TrimSpace(command.RiskKeywords)
	return command, nil
}

func (service Service) sopPolicyWriteStore() SOPPolicyWriteStore {
	if service.SOPPolicyWriteStore != nil {
		return service.SOPPolicyWriteStore
	}
	if store, ok := service.SOPPolicyStore.(SOPPolicyWriteStore); ok {
		return store
	}
	return nil
}
