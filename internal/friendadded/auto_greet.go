package friendadded

import (
	"context"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/incomingwrite"
	"wework-go/internal/outbox"
	"wework-go/internal/workbench"
)

const defaultFriendAddedContent = "\u65b0\u597d\u53cb\u7533\u8bf7"

// AccountStore loads WeCom account facts needed by friend-added auto greet selection.
type AccountStore interface {
	ListAccounts(ctx context.Context) ([]workbench.AccountRecord, error)
}

// SOPFlowStore loads SOP flow configs needed by friend-added auto greet selection.
type SOPFlowStore interface {
	ListSOPFlows(ctx context.Context) ([]workbench.SOPFlowRecord, error)
}

// SOPPolicyStore loads SOP policies needed by friend-added auto greet selection.
type SOPPolicyStore interface {
	ListSOPPolicies(ctx context.Context) ([]workbench.SOPPolicyRecord, error)
}

// OutboxEnqueuer appends legacy outbox_events rows for async auto-reply workers.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

type autoGreetDecision struct {
	ShouldQueue    bool
	ConversationID string
	TenantID       string
	AccountID      string
	WeWorkUserID   string
	FriendIdentity string
	FriendName     string
	Content        string
	FlowID         string
	PolicyID       string
	ReplyMode      string
	OccurredAt     time.Time
}

func (service Service) queueAutoGreet(ctx context.Context, request Request) bool {
	if service.Accounts == nil || service.SOPFlows == nil || service.SOPPolicies == nil || service.Outbox == nil {
		return false
	}
	decision, err := service.autoGreetDecision(ctx, request)
	if err != nil || !decision.ShouldQueue {
		return false
	}
	_, err = service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{buildAutoGreetOutboxEvent(request, decision)})
	return err == nil
}

func (service Service) autoGreetDecision(ctx context.Context, request Request) (autoGreetDecision, error) {
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return autoGreetDecision{}, err
	}
	account, hasAccount := findAccountByDevice(accounts, request.DeviceID)
	flows, err := service.SOPFlows.ListSOPFlows(ctx)
	if err != nil {
		return autoGreetDecision{}, err
	}
	explicitFlowID := ""
	if hasAccount {
		explicitFlowID = strings.TrimSpace(account.SOPFlowID)
	}
	flowID := defaultText(explicitFlowID, "default")
	flow, hasFlow := findFlowConfig(flows, flowID)
	if hasAccount && explicitFlowID == "" {
		if audienceFlow, ok := findEnabledFlowForAssignee(flows, account.AssigneeID, "platform_pull"); ok {
			flow = audienceFlow
			hasFlow = true
			flowID = defaultText(strings.TrimSpace(audienceFlow.FlowID), "default")
		}
	}
	policies, err := service.SOPPolicies.ListSOPPolicies(ctx)
	if err != nil {
		return autoGreetDecision{}, err
	}
	policy, hasPolicy := chooseSOPPolicy(policies, "day1", "friend_added", flowID)
	platformPullEnabled := hasFlow && flow.Enabled && strings.TrimSpace(flow.ExecutionMode) == "platform_pull"
	if !hasPolicy && !platformPullEnabled {
		return autoGreetDecision{}, nil
	}
	friendIdentity := firstNonBlank(request.FriendID, request.FriendName)
	weworkUserID := ""
	accountID := optionalTrimmedPointerValue(request.AccountID)
	tenantID := optionalTrimmedPointerValue(request.TenantID)
	if hasAccount {
		weworkUserID = strings.TrimSpace(account.WeWorkUserID)
		accountID = defaultText(accountID, strings.TrimSpace(account.AccountID))
		tenantID = defaultText(tenantID, strings.TrimSpace(account.EnterpriseID))
	}
	conversationID := incomingmodel.BuildConversationID(incomingmodel.IncomingMessage{
		DeviceID:         request.DeviceID,
		SenderID:         friendIdentity,
		ConversationName: request.FriendName,
		WeWorkUserID:     weworkUserID,
		ExternalUserID:   friendIdentity,
	})
	replyMode := "platform_pull"
	policyID := ""
	if hasPolicy {
		policyID = strings.TrimSpace(policy.PolicyID)
		replyMode = defaultText(strings.TrimSpace(policy.ReplyMode), "sop_only")
	}
	content := strings.TrimSpace(request.AutoGreetContent)
	if content == "" {
		content = request.Source
	}
	if content == "" {
		content = defaultFriendAddedContent
	}
	return autoGreetDecision{
		ShouldQueue:    true,
		ConversationID: conversationID,
		TenantID:       tenantID,
		AccountID:      accountID,
		WeWorkUserID:   weworkUserID,
		FriendIdentity: friendIdentity,
		FriendName:     request.FriendName,
		Content:        content,
		FlowID:         flowID,
		PolicyID:       policyID,
		ReplyMode:      replyMode,
		OccurredAt:     request.Timestamp.UTC(),
	}, nil
}

func buildAutoGreetOutboxEvent(request Request, decision autoGreetDecision) outbox.EventEnvelope {
	occurredAt := decision.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	traceID := strings.TrimSpace(request.TraceID)
	partitionKey := strings.TrimSpace(request.DeviceID) + ":" + strings.TrimSpace(decision.FriendIdentity)
	payload := map[string]any{
		"conversation_id":   decision.ConversationID,
		"tenant_id":         decision.TenantID,
		"account_id":        decision.AccountID,
		"wework_user_id":    decision.WeWorkUserID,
		"device_id":         strings.TrimSpace(request.DeviceID),
		"sender_id":         strings.TrimSpace(decision.FriendIdentity),
		"sender_name":       strings.TrimSpace(decision.FriendName),
		"conversation_name": strings.TrimSpace(decision.FriendName),
		"content":           decision.Content,
		"first_message_at":  timeOrNil(occurredAt),
		"trigger_event":     "friend_added",
		"trace_id":          traceID,
		"flow_id":           decision.FlowID,
		"policy_id":         decision.PolicyID,
		"reply_mode":        decision.ReplyMode,
		"source":            "friend_added_event",
	}
	return outbox.EventEnvelope{
		EventID:       traceID + ":auto-reply",
		EventType:     incomingwrite.EventConversationAutoReply,
		AggregateType: "conversation",
		AggregateID:   decision.ConversationID,
		TenantID:      decision.TenantID,
		PartitionKey:  partitionKey,
		TraceID:       traceID,
		Payload:       payload,
		OccurredAt:    occurredAt,
		AvailableAt:   occurredAt.Add(time.Millisecond),
	}
}

func findAccountByDevice(accounts []workbench.AccountRecord, deviceID string) (workbench.AccountRecord, bool) {
	normalized := strings.TrimSpace(deviceID)
	for _, account := range accounts {
		if strings.TrimSpace(account.DeviceID) == normalized {
			return account, true
		}
	}
	return workbench.AccountRecord{}, false
}

func findFlowConfig(flows []workbench.SOPFlowRecord, flowID string) (workbench.SOPFlowRecord, bool) {
	normalized := defaultText(strings.TrimSpace(flowID), "default")
	var defaultFlow *workbench.SOPFlowRecord
	for index := range flows {
		flow := flows[index]
		resolved := defaultText(strings.TrimSpace(flow.FlowID), "default")
		if resolved == normalized {
			return flow, true
		}
		if resolved == "default" {
			copy := flow
			defaultFlow = &copy
		}
	}
	if defaultFlow != nil {
		return *defaultFlow, true
	}
	return workbench.SOPFlowRecord{}, false
}

func findEnabledFlowForAssignee(flows []workbench.SOPFlowRecord, assigneeID string, executionMode string) (workbench.SOPFlowRecord, bool) {
	normalizedAssignee := strings.TrimSpace(assigneeID)
	normalizedMode := strings.TrimSpace(executionMode)
	type candidate struct {
		audienceScore int
		defaultScore  int
		flowID        string
		flow          workbench.SOPFlowRecord
	}
	candidates := make([]candidate, 0)
	for _, flow := range flows {
		if !flow.Enabled {
			continue
		}
		mode := defaultText(strings.TrimSpace(flow.ExecutionMode), "local_days")
		if normalizedMode != "" && mode != normalizedMode {
			continue
		}
		audienceMode, audienceIDs := parseTargetAudience(flow.TargetAudience)
		if audienceMode == "none" {
			continue
		}
		if audienceMode == "specific" && (normalizedAssignee == "" || !stringSetContains(audienceIDs, normalizedAssignee)) {
			continue
		}
		flowID := defaultText(strings.TrimSpace(flow.FlowID), "default")
		audienceScore := 1
		if audienceMode == "specific" {
			audienceScore = 0
		}
		defaultScore := 0
		if flowID == "default" {
			defaultScore = 1
		}
		candidates = append(candidates, candidate{audienceScore: audienceScore, defaultScore: defaultScore, flowID: flowID, flow: flow})
	}
	if len(candidates) == 0 {
		return workbench.SOPFlowRecord{}, false
	}
	best := candidates[0]
	for _, item := range candidates[1:] {
		if item.audienceScore < best.audienceScore ||
			(item.audienceScore == best.audienceScore && item.defaultScore < best.defaultScore) ||
			(item.audienceScore == best.audienceScore && item.defaultScore == best.defaultScore && item.flowID < best.flowID) {
			best = item
		}
	}
	return best.flow, true
}

func chooseSOPPolicy(policies []workbench.SOPPolicyRecord, dayStage string, triggerEvent string, flowID string) (workbench.SOPPolicyRecord, bool) {
	normalizedFlowID := strings.TrimSpace(flowID)
	matched := matchingSOPPolicies(policies, dayStage, triggerEvent, normalizedFlowID)
	if len(matched) == 0 && normalizedFlowID != "" {
		matched = matchingSOPPolicies(policies, dayStage, triggerEvent, "default")
	}
	if len(matched) == 0 {
		return workbench.SOPPolicyRecord{}, false
	}
	best := matched[0]
	for _, policy := range matched[1:] {
		if policy.Priority < best.Priority || (policy.Priority == best.Priority && strings.TrimSpace(policy.UpdatedAt) < strings.TrimSpace(best.UpdatedAt)) {
			best = policy
		}
	}
	return best, true
}

func matchingSOPPolicies(policies []workbench.SOPPolicyRecord, dayStage string, triggerEvent string, flowID string) []workbench.SOPPolicyRecord {
	matched := make([]workbench.SOPPolicyRecord, 0)
	for _, policy := range policies {
		policyFlowID := defaultText(strings.TrimSpace(policy.FlowID), "default")
		if policy.Enabled &&
			strings.TrimSpace(policy.DayStage) == dayStage &&
			strings.TrimSpace(policy.TriggerEvent) == triggerEvent &&
			(flowID == "" || policyFlowID == flowID) {
			matched = append(matched, policy)
		}
	}
	return matched
}

func parseTargetAudience(raw string) (string, []string) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" || normalized == "__ALL__" {
		return "all", nil
	}
	if normalized == "__NONE__" {
		return "none", nil
	}
	replacer := strings.NewReplacer("\n", ",", "\uff0c", ",", "\uff1b", ",")
	parts := strings.Split(replacer.Replace(normalized), ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part)
		if text == "" || text == "__ALL__" || text == "__NONE__" {
			continue
		}
		ids = append(ids, text)
	}
	if len(ids) == 0 {
		return "none", nil
	}
	return "specific", ids
}

func stringSetContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func timeOrNil(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
