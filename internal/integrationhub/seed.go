package integrationhub

import (
	"fmt"
	"math"
	"time"
)

type Snapshot struct {
	Channels      []Channel             `json:"channels"`
	PipelineStats []PipelineStageStats  `json:"pipelineStats"`
	MessageEvents []MessageEvent        `json:"messageEvents"`
	Conversations []Conversation        `json:"conversations"`
	Messages      []ConversationMessage `json:"messages"`
	AIPolicies    []AIPolicy            `json:"aiPolicies"`
	SOPWorkflows  []SOPWorkflow         `json:"sopWorkflows"`
	OutboxItems   []OutboxItem          `json:"outboxItems"`
	AuditLog      []AuditLogEntry       `json:"auditLog"`
	Incidents     []RecentIncident      `json:"recentIncidents"`
	TrafficSeries []TrafficPoint        `json:"trafficSeries"`
	Settings      PlatformSettings      `json:"settings"`
}

func SeedSnapshot(now time.Time) Snapshot {
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute).UTC() }
	operatorSarah := "Sarah Chen"
	operatorWei := "Wei Zhao"
	quoteWorkflow := "Enterprise Quote Approval"
	shippingWorkflow := "Shipping Escalation"
	contractWorkflow := "Contract Signing"
	partnerWorkflow := "Partner Onboarding"
	refundWorkflow := "Refund Processing"
	orderWorkflow := "Order Confirmation"
	emailChannel := ChannelEmail
	whatsAppChannel := ChannelWhatsApp
	dingTalkChannel := ChannelDingTalk

	channels := []Channel{
		{
			ID: "ch_wecom", Kind: ChannelWeCom, Name: "WeCom - Sales Workspace", Status: ChannelConnected,
			ReceiveCapabilities: []ReceiveCapability{ReceiveWebhook}, SendCapabilities: []SendCapability{SendAPI},
			LastSyncAt: ago(1), ErrorCount24h: 2, MessagesToday: 4218, ActiveConversations: 312,
		},
		{
			ID: "ch_feishu", Kind: ChannelFeishu, Name: "Feishu - Customer Success", Status: ChannelConnected,
			ReceiveCapabilities: []ReceiveCapability{ReceiveWebhook}, SendCapabilities: []SendCapability{SendAPI},
			LastSyncAt: ago(2), ErrorCount24h: 0, MessagesToday: 2854, ActiveConversations: 201,
		},
		{
			ID: "ch_dingtalk", Kind: ChannelDingTalk, Name: "DingTalk - Partner Channel", Status: ChannelDegraded,
			ReceiveCapabilities: []ReceiveCapability{ReceiveWebhook, ReceivePolling}, SendCapabilities: []SendCapability{SendAPI, SendManualApproval},
			LastSyncAt: ago(18), ErrorCount24h: 47, MessagesToday: 963, ActiveConversations: 84,
		},
		{
			ID: "ch_whatsapp", Kind: ChannelWhatsApp, Name: "WhatsApp Business", Status: ChannelConnected,
			ReceiveCapabilities: []ReceiveCapability{ReceiveWebhook}, SendCapabilities: []SendCapability{SendAPI},
			LastSyncAt: ago(1), ErrorCount24h: 5, MessagesToday: 3109, ActiveConversations: 265,
		},
		{
			ID: "ch_telegram", Kind: ChannelTelegram, Name: "Telegram Bot - Community", Status: ChannelConnected,
			ReceiveCapabilities: []ReceiveCapability{ReceiveWebhook}, SendCapabilities: []SendCapability{SendAPI},
			LastSyncAt: ago(3), ErrorCount24h: 1, MessagesToday: 1542, ActiveConversations: 118,
		},
		{
			ID: "ch_email_rpa", Kind: ChannelEmail, Name: "Email - Support Mailbox (RPA)", Status: ChannelDisabled,
			ReceiveCapabilities: []ReceiveCapability{ReceivePolling, ReceiveRPA}, SendCapabilities: []SendCapability{SendRPA, SendManualApproval},
			LastSyncAt: ago(540), ErrorCount24h: 0, MessagesToday: 0, ActiveConversations: 0,
		},
	}

	messageEvents := make([]MessageEvent, 0, 64)
	eventTypes := []string{"message.received", "message.sent", "message.delivered", "message.failed", "sop.step.completed", "ai.reply_drafted", "ai.handoff_triggered"}
	channelKinds := []ChannelKind{ChannelWeCom, ChannelFeishu, ChannelDingTalk, ChannelWhatsApp, ChannelTelegram, ChannelEmail}
	statuses := []MessageEventStatus{EventSuccess, EventSuccess, EventSuccess, EventSuccess, EventPending, EventRetrying, EventFailed}
	for i := 0; i < 64; i++ {
		channel := channelKinds[i%len(channelKinds)]
		messageEvents = append(messageEvents, MessageEvent{
			ID:                fmt.Sprintf("evt_%d", 1000+i),
			Time:              ago(3 + i*4),
			Channel:           channel,
			Direction:         []MessageDirection{DirectionInbound, DirectionOutbound}[i%2],
			ConversationID:    fmt.Sprintf("conv_%d", 100+i%40),
			ConversationLabel: fmt.Sprintf("Conversation #%d", 100+i%40),
			EventType:         eventTypes[i%len(eventTypes)],
			Status:            statuses[i%len(statuses)],
			LatencyMs:         40 + (i*37)%900,
			TraceID:           fmt.Sprintf("trace-%06x", 100000+i*7919),
		})
	}

	conversations := []Conversation{
		{ID: "conv_101", Channel: ChannelWeCom, ContactName: "Li Wei", ContactHandle: "@liwei_procurement", LastMessagePreview: "Can you send the latest quote and payment terms?", LastMessageAt: ago(4), AssignedOperator: &operatorSarah, AIStatus: AIAutoReplying, SOPStage: SOPInProgress, SOPWorkflowName: &quoteWorkflow, Unread: 2, Tags: []string{"enterprise", "quote"}},
		{ID: "conv_102", Channel: ChannelWhatsApp, ContactName: "Maria Gonzalez", ContactHandle: "+52 55 1234 0098", LastMessagePreview: "The tracking number has not updated in 3 days.", LastMessageAt: ago(9), AIStatus: AIHandedOff, SOPStage: SOPWaitingHuman, SOPWorkflowName: &shippingWorkflow, Unread: 1, Tags: []string{"logistics", "escalation"}},
		{ID: "conv_103", Channel: ChannelFeishu, ContactName: "Zhang Min", ContactHandle: "@zhangmin", LastMessagePreview: "Contract terms confirmed; waiting for seal.", LastMessageAt: ago(21), AssignedOperator: &operatorWei, AIStatus: AIMonitoring, SOPStage: SOPCompleted, SOPWorkflowName: &contractWorkflow, Tags: []string{"contract"}},
		{ID: "conv_104", Channel: ChannelTelegram, ContactName: "Alex Petrov", ContactHandle: "@alexp", LastMessagePreview: "Is there a way to integrate with our own CRM?", LastMessageAt: ago(33), AssignedOperator: &operatorSarah, AIStatus: AIAutoReplying, SOPStage: SOPNone, Tags: []string{"pre-sales"}},
		{ID: "conv_105", Channel: ChannelDingTalk, ContactName: "Chen Hao", ContactHandle: "@chenhao_partner", LastMessagePreview: "Question about authentication in the integration docs.", LastMessageAt: ago(58), AIStatus: AIIdle, SOPStage: SOPFailed, SOPWorkflowName: &partnerWorkflow, Unread: 3, Tags: []string{"partner", "technical"}},
		{ID: "conv_106", Channel: ChannelEmail, ContactName: "Support Ticket #8821", ContactHandle: "j.turner@acme-corp.com", LastMessagePreview: "Following up on the refund request submitted last week.", LastMessageAt: ago(72), AssignedOperator: &operatorWei, AIStatus: AIHandedOff, SOPStage: SOPWaitingHuman, SOPWorkflowName: &refundWorkflow, Tags: []string{"billing"}},
		{ID: "conv_107", Channel: ChannelWhatsApp, ContactName: "Fatima Al-Sayed", ContactHandle: "+971 50 220 3344", LastMessagePreview: "Perfect, thank you for the quick response!", LastMessageAt: ago(96), AssignedOperator: &operatorSarah, AIStatus: AIMonitoring, SOPStage: SOPCompleted, SOPWorkflowName: &orderWorkflow, Tags: []string{"order"}},
	}

	messages := []ConversationMessage{
		{ID: "msg_1", ConversationID: "conv_101", Channel: ChannelWeCom, Direction: DirectionInbound, Author: "Li Wei", Content: "Hello, we would like to learn more about the enterprise pricing plan.", Time: ago(40)},
		{ID: "msg_2", ConversationID: "conv_101", Channel: ChannelWeCom, Direction: DirectionOutbound, Author: "AI Assistant", Content: "Hi Li Wei, I prepared the enterprise pricing overview with seats and annual fees. I can send the PDF shortly.", Time: ago(38), IsAIGenerated: true},
		{ID: "msg_3", ConversationID: "conv_101", Channel: ChannelWeCom, Direction: DirectionInbound, Author: "Li Wei", Content: "Can you send the latest quote and payment terms?", Time: ago(4)},
	}

	snapshot := Snapshot{
		Channels: channels,
		PipelineStats: []PipelineStageStats{
			{Stage: StageConnector, Label: "Connector", ThroughputPerMin: 214, Failures1h: 3, AvgLatencyMs: 82},
			{Stage: StageIngest, Label: "Ingest", ThroughputPerMin: 211, Failures1h: 2, AvgLatencyMs: 46},
			{Stage: StageNormalize, Label: "Normalize", ThroughputPerMin: 209, Failures1h: 1, AvgLatencyMs: 34},
			{Stage: StageStore, Label: "Store", ThroughputPerMin: 209, Failures1h: 0, AvgLatencyMs: 21},
			{Stage: StageSOPAI, Label: "SOP / AI", ThroughputPerMin: 187, Failures1h: 6, AvgLatencyMs: 640},
			{Stage: StageOutbox, Label: "Outbox", ThroughputPerMin: 164, Failures1h: 4, AvgLatencyMs: 118},
			{Stage: StageDelivery, Label: "Delivery", ThroughputPerMin: 159, Failures1h: 5, AvgLatencyMs: 245},
		},
		MessageEvents: messageEvents,
		Conversations: conversations,
		Messages:      messages,
		AIPolicies: []AIPolicy{
			{ID: "pol_1", Kind: PolicyIntentClassification, Name: "Inbound Intent Classifier", Enabled: true, Priority: 1, TriggerCondition: "On every inbound message", FallbackStrategy: "Route to default queue", SuccessRate7d: 0.97, Invocations24h: 8420},
			{ID: "pol_2", Kind: PolicyRiskDetection, Name: "Compliance & Risk Screening", Enabled: true, Priority: 1, TriggerCondition: "Message contains financial or legal terms", FallbackStrategy: "Flag for human review and block auto-reply", SuccessRate7d: 0.99, Invocations24h: 612},
			{ID: "pol_3", Kind: PolicyReplyDrafting, Name: "Sales Reply Drafting", Enabled: true, Priority: 2, TriggerCondition: "Intent = pre-sales and confidence > 0.8", FallbackStrategy: "Draft only, require operator approval", SuccessRate7d: 0.91, Invocations24h: 2140},
			{ID: "pol_4", Kind: PolicyKnowledgeRetrieval, Name: "Product Knowledge Retrieval", Enabled: true, Priority: 2, TriggerCondition: "Question matches knowledge base topics", FallbackStrategy: "Fall back to human handoff", SuccessRate7d: 0.94, Invocations24h: 3305},
			{ID: "pol_5", Kind: PolicyToolCalling, Name: "Order Status Lookup", Enabled: true, Priority: 3, TriggerCondition: "Intent = order_status", FallbackStrategy: "Retry once, then escalate", SuccessRate7d: 0.88, Invocations24h: 1876},
			{ID: "pol_6", Kind: PolicyHumanHandoff, Name: "Escalation Handoff Policy", Enabled: true, Priority: 1, TriggerCondition: "Risk flag or SLA breach imminent", FallbackStrategy: "Assign to on-call operator", SuccessRate7d: 0.99, Invocations24h: 214},
			{ID: "pol_7", Kind: PolicyAutoReply, Name: "After-hours Auto Reply", Enabled: false, Priority: 4, TriggerCondition: "Outside business hours and no operator online", FallbackStrategy: "Queue for next business day", SuccessRate7d: 0.95, Invocations24h: 0},
		},
		SOPWorkflows: []SOPWorkflow{
			{ID: "wf_1", Name: quoteWorkflow, Trigger: "Intent = pricing_request AND deal_size > 50k", Channels: []ChannelKind{ChannelWeCom, ChannelFeishu}, ActiveConversations: 18, CompletionRate: 0.86, SLAMinutes: 240, Status: WorkflowActive, Steps: []WorkflowStep{
				step("s1", "Classify request", "Inbound message received", ptr("Classify intent and extract deal size"), nil, 2, "Route to manual triage"),
				step("s2", "Generate quote draft", "Intent confirmed as pricing_request", ptr("Draft quote from pricing table"), nil, 5, "Notify sales ops"),
				step("s3", "Sales approval", "Quote draft ready", nil, ptr("Sales manager reviews and approves discount"), 120, "Escalate to regional director"),
				step("s4", "Send to customer", "Quote approved", ptr("Send PDF via original channel"), nil, 5, "Retry delivery, then alert operator"),
			}},
			{ID: "wf_2", Name: shippingWorkflow, Trigger: "Intent = shipping_delay AND days_delayed > 2", Channels: []ChannelKind{ChannelWhatsApp, ChannelEmail}, ActiveConversations: 7, CompletionRate: 0.72, SLAMinutes: 60, Status: WorkflowActive, Steps: []WorkflowStep{
				step("s1", "Verify tracking status", "Escalation triggered", ptr("Call logistics API for latest tracking event"), nil, 3, "Escalate directly to human"),
				step("s2", "Support review", "Tracking confirms delay", nil, ptr("Support agent contacts carrier"), 45, "Escalate to logistics manager"),
			}},
			{ID: "wf_3", Name: partnerWorkflow, Trigger: "New partner application submitted", Channels: []ChannelKind{ChannelDingTalk}, ActiveConversations: 4, CompletionRate: 0.64, SLAMinutes: 1440, Status: WorkflowPaused, Steps: []WorkflowStep{
				step("s1", "Document collection", "Application received", ptr("Send required document checklist"), nil, 60, "Remind after 24h"),
				step("s2", "Technical review", "Documents complete", nil, ptr("Solutions engineer reviews integration plan"), 720, "Escalate to partner manager"),
			}},
		},
		OutboxItems: []OutboxItem{
			{ID: "out_1", CreatedAt: ago(2), Channel: ChannelWeCom, ConversationID: "conv_101", ConversationLabel: "Li Wei - Enterprise Quote", MessageType: "Document", Sender: "AI Assistant", DeliveryMethod: SendAPI, Status: OutboxSending},
			{ID: "out_2", CreatedAt: ago(6), Channel: ChannelDingTalk, ConversationID: "conv_105", ConversationLabel: "Chen Hao - Partner Onboarding", MessageType: "Text", Sender: "System", DeliveryMethod: SendRPA, Status: OutboxFailed, RetryCount: 3, LastError: ptr("RPA session timeout after 30s")},
			{ID: "out_3", CreatedAt: ago(11), Channel: ChannelEmail, ConversationID: "conv_106", ConversationLabel: "Ticket #8821 - Refund", MessageType: "Email", Sender: "Wei Zhao", DeliveryMethod: SendManualApproval, Status: OutboxRequiresApproval},
			{ID: "out_4", CreatedAt: ago(14), Channel: ChannelWhatsApp, ConversationID: "conv_102", ConversationLabel: "Maria Gonzalez - Shipping", MessageType: "Text", Sender: "AI Assistant", DeliveryMethod: SendAPI, Status: OutboxSent},
			{ID: "out_5", CreatedAt: ago(19), Channel: ChannelTelegram, ConversationID: "conv_104", ConversationLabel: "Alex Petrov - Pre-sales", MessageType: "Text", Sender: "AI Assistant", DeliveryMethod: SendAPI, Status: OutboxPending},
			{ID: "out_6", CreatedAt: ago(26), Channel: ChannelFeishu, ConversationID: "conv_103", ConversationLabel: "Zhang Min - Contract", MessageType: "Card", Sender: "System", DeliveryMethod: SendAPI, Status: OutboxSent, RetryCount: 1},
			{ID: "out_7", CreatedAt: ago(31), Channel: ChannelWhatsApp, ConversationID: "conv_107", ConversationLabel: "Fatima Al-Sayed - Order", MessageType: "Text", Sender: "AI Assistant", DeliveryMethod: SendAPI, Status: OutboxFailed, RetryCount: 2, LastError: ptr("Recipient number opted out of messages")},
		},
		AuditLog: []AuditLogEntry{
			{ID: "aud_1", Time: ago(3), Actor: "Sarah Chen", ActorType: ActorUser, Action: "Approved outbound message", Target: "out_3", Channel: &emailChannel, Result: AuditSuccess, IP: ptr("10.20.4.18")},
			{ID: "aud_2", Time: ago(12), Actor: "AI Assistant", ActorType: ActorAI, Action: "Drafted reply", Target: "conv_101", Channel: ptr(ChannelWeCom), Result: AuditSuccess},
			{ID: "aud_3", Time: ago(24), Actor: "System", ActorType: ActorSystem, Action: "Disabled channel after repeated failures", Target: "ch_email_rpa", Channel: &emailChannel, Result: AuditSuccess},
			{ID: "aud_4", Time: ago(40), Actor: "Wei Zhao", ActorType: ActorUser, Action: "Updated SOP workflow", Target: "wf_3", Result: AuditSuccess, IP: ptr("10.20.4.42")},
			{ID: "aud_5", Time: ago(58), Actor: "System", ActorType: ActorSystem, Action: "API key rotation failed", Target: "ch_dingtalk", Channel: &dingTalkChannel, Result: AuditFailure},
		},
		Incidents: []RecentIncident{
			{ID: "inc_1", Time: ago(18), Severity: IncidentWarning, Summary: "DingTalk webhook latency exceeded 5s threshold", Channel: &dingTalkChannel},
			{ID: "inc_2", Time: ago(52), Severity: IncidentCritical, Summary: "Email RPA connector disabled after 12 consecutive failures", Channel: &emailChannel},
			{ID: "inc_3", Time: ago(95), Severity: IncidentInfo, Summary: "WhatsApp template message rejected - outdated template ID", Channel: &whatsAppChannel},
		},
		TrafficSeries: seedTrafficSeries(),
		Settings: PlatformSettings{
			Environment:      "production",
			Region:           "ap-southeast-1",
			RetentionDays:    90,
			WebhookURL:       "https://hooks.imintegration.local/events",
			EnabledProviders: []string{"openai", "internal-rules"},
		},
	}
	return snapshot
}

func seedTrafficSeries() []TrafficPoint {
	points := make([]TrafficPoint, 0, 24)
	for i := 0; i < 24; i++ {
		base := 300 + math.Sin(float64(i)/2.4)*120
		points = append(points, TrafficPoint{
			Hour:     fmt.Sprintf("%02d:00", i),
			Inbound:  max(20, int(math.Round(base+float64((i*37)%60)))),
			Outbound: max(15, int(math.Round(base*0.82+float64((i*29)%50)))),
		})
	}
	return points
}

func step(id, name, condition string, aiAction, humanAction *string, timeoutMinutes int, fallback string) WorkflowStep {
	return WorkflowStep{ID: id, Name: name, Condition: condition, AIAction: aiAction, HumanAction: humanAction, TimeoutMinutes: timeoutMinutes, Fallback: fallback}
}

func ptr[T any](value T) *T {
	return &value
}
