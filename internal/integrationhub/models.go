package integrationhub

import "time"

type ChannelKind string

const (
	ChannelWeCom    ChannelKind = "wecom"
	ChannelFeishu   ChannelKind = "feishu"
	ChannelDingTalk ChannelKind = "dingtalk"
	ChannelWhatsApp ChannelKind = "whatsapp"
	ChannelTelegram ChannelKind = "telegram"
	ChannelEmail    ChannelKind = "email"
)

type ChannelStatus string

const (
	ChannelConnected ChannelStatus = "connected"
	ChannelDegraded  ChannelStatus = "degraded"
	ChannelDisabled  ChannelStatus = "disabled"
)

type ReceiveCapability string

const (
	ReceiveWebhook ReceiveCapability = "webhook"
	ReceivePolling ReceiveCapability = "polling"
	ReceiveRPA     ReceiveCapability = "rpa"
)

type SendCapability string

const (
	SendAPI            SendCapability = "api"
	SendRPA            SendCapability = "rpa"
	SendManualApproval SendCapability = "manual_approval"
)

type Channel struct {
	ID                  string              `json:"id"`
	Kind                ChannelKind         `json:"kind"`
	Name                string              `json:"name"`
	Status              ChannelStatus       `json:"status"`
	ReceiveCapabilities []ReceiveCapability `json:"receiveCapabilities"`
	SendCapabilities    []SendCapability    `json:"sendCapabilities"`
	LastSyncAt          time.Time           `json:"lastSyncAt"`
	ErrorCount24h       int                 `json:"errorCount24h"`
	MessagesToday       int                 `json:"messagesToday"`
	ActiveConversations int                 `json:"activeConversations"`
}

type PipelineStage string

const (
	StageConnector PipelineStage = "connector"
	StageIngest    PipelineStage = "ingest"
	StageNormalize PipelineStage = "normalize"
	StageStore     PipelineStage = "store"
	StageSOPAI     PipelineStage = "sop_ai"
	StageOutbox    PipelineStage = "outbox"
	StageDelivery  PipelineStage = "delivery"
)

type PipelineStageStats struct {
	Stage            PipelineStage `json:"stage"`
	Label            string        `json:"label"`
	ThroughputPerMin int           `json:"throughputPerMin"`
	Failures1h       int           `json:"failures1h"`
	AvgLatencyMs     int           `json:"avgLatencyMs"`
}

type MessageDirection string

const (
	DirectionInbound  MessageDirection = "inbound"
	DirectionOutbound MessageDirection = "outbound"
)

type MessageEventStatus string

const (
	EventSuccess  MessageEventStatus = "success"
	EventFailed   MessageEventStatus = "failed"
	EventRetrying MessageEventStatus = "retrying"
	EventPending  MessageEventStatus = "pending"
)

type MessageEvent struct {
	ID                string             `json:"id"`
	Time              time.Time          `json:"time"`
	Channel           ChannelKind        `json:"channel"`
	Direction         MessageDirection   `json:"direction"`
	ConversationID    string             `json:"conversationId"`
	ConversationLabel string             `json:"conversationLabel"`
	EventType         string             `json:"eventType"`
	Status            MessageEventStatus `json:"status"`
	LatencyMs         int                `json:"latencyMs"`
	TraceID           string             `json:"traceId"`
}

type SOPStage string

const (
	SOPNone         SOPStage = "none"
	SOPInProgress   SOPStage = "in_progress"
	SOPWaitingHuman SOPStage = "waiting_human"
	SOPCompleted    SOPStage = "completed"
	SOPFailed       SOPStage = "failed"
)

type AIStatus string

const (
	AIAutoReplying AIStatus = "auto_replying"
	AIMonitoring   AIStatus = "monitoring"
	AIHandedOff    AIStatus = "handed_off"
	AIIdle         AIStatus = "idle"
)

type Conversation struct {
	ID                 string      `json:"id"`
	Channel            ChannelKind `json:"channel"`
	ContactName        string      `json:"contactName"`
	ContactHandle      string      `json:"contactHandle"`
	LastMessagePreview string      `json:"lastMessagePreview"`
	LastMessageAt      time.Time   `json:"lastMessageAt"`
	AssignedOperator   *string     `json:"assignedOperator"`
	AIStatus           AIStatus    `json:"aiStatus"`
	SOPStage           SOPStage    `json:"sopStage"`
	SOPWorkflowName    *string     `json:"sopWorkflowName"`
	Unread             int         `json:"unread"`
	Tags               []string    `json:"tags"`
}

type ConversationMessage struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversationId"`
	Channel        ChannelKind      `json:"channel"`
	Direction      MessageDirection `json:"direction"`
	Author         string           `json:"author"`
	Content        string           `json:"content"`
	Time           time.Time        `json:"time"`
	IsAIGenerated  bool             `json:"isAiGenerated,omitempty"`
}

type AIPolicyKind string

const (
	PolicyIntentClassification AIPolicyKind = "intent_classification"
	PolicyRiskDetection        AIPolicyKind = "risk_detection"
	PolicyReplyDrafting        AIPolicyKind = "reply_drafting"
	PolicyKnowledgeRetrieval   AIPolicyKind = "knowledge_retrieval"
	PolicyToolCalling          AIPolicyKind = "tool_calling"
	PolicyHumanHandoff         AIPolicyKind = "human_handoff"
	PolicyAutoReply            AIPolicyKind = "auto_reply_policy"
)

type AIPolicy struct {
	ID               string       `json:"id"`
	Kind             AIPolicyKind `json:"kind"`
	Name             string       `json:"name"`
	Enabled          bool         `json:"enabled"`
	Priority         int          `json:"priority"`
	TriggerCondition string       `json:"triggerCondition"`
	FallbackStrategy string       `json:"fallbackStrategy"`
	SuccessRate7d    float64      `json:"successRate7d"`
	Invocations24h   int          `json:"invocations24h"`
}

type WorkflowStatus string

const (
	WorkflowActive WorkflowStatus = "active"
	WorkflowPaused WorkflowStatus = "paused"
	WorkflowDraft  WorkflowStatus = "draft"
)

type WorkflowStep struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Condition      string  `json:"condition"`
	AIAction       *string `json:"aiAction"`
	HumanAction    *string `json:"humanAction"`
	TimeoutMinutes int     `json:"timeoutMinutes"`
	Fallback       string  `json:"fallback"`
}

type SOPWorkflow struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Trigger             string         `json:"trigger"`
	Channels            []ChannelKind  `json:"channels"`
	ActiveConversations int            `json:"activeConversations"`
	CompletionRate      float64        `json:"completionRate"`
	SLAMinutes          int            `json:"slaMinutes"`
	Status              WorkflowStatus `json:"status"`
	Steps               []WorkflowStep `json:"steps"`
}

type OutboxStatus string

const (
	OutboxPending          OutboxStatus = "pending"
	OutboxSending          OutboxStatus = "sending"
	OutboxFailed           OutboxStatus = "failed"
	OutboxSent             OutboxStatus = "sent"
	OutboxRequiresApproval OutboxStatus = "requires_approval"
	OutboxCanceled         OutboxStatus = "canceled"
)

type OutboxItem struct {
	ID                string         `json:"id"`
	CreatedAt         time.Time      `json:"createdAt"`
	Channel           ChannelKind    `json:"channel"`
	ConversationID    string         `json:"conversationId"`
	ConversationLabel string         `json:"conversationLabel"`
	MessageType       string         `json:"messageType"`
	Sender            string         `json:"sender"`
	DeliveryMethod    SendCapability `json:"deliveryMethod"`
	Status            OutboxStatus   `json:"status"`
	RetryCount        int            `json:"retryCount"`
	LastError         *string        `json:"lastError"`
}

type AuditActorType string

const (
	ActorUser   AuditActorType = "user"
	ActorSystem AuditActorType = "system"
	ActorAI     AuditActorType = "ai"
)

type AuditResult string

const (
	AuditSuccess AuditResult = "success"
	AuditFailure AuditResult = "failure"
)

type AuditLogEntry struct {
	ID        string         `json:"id"`
	Time      time.Time      `json:"time"`
	Actor     string         `json:"actor"`
	ActorType AuditActorType `json:"actorType"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Channel   *ChannelKind   `json:"channel"`
	Result    AuditResult    `json:"result"`
	IP        *string        `json:"ip"`
}

type IncidentSeverity string

const (
	IncidentCritical IncidentSeverity = "critical"
	IncidentWarning  IncidentSeverity = "warning"
	IncidentInfo     IncidentSeverity = "info"
)

type RecentIncident struct {
	ID       string           `json:"id"`
	Time     time.Time        `json:"time"`
	Severity IncidentSeverity `json:"severity"`
	Summary  string           `json:"summary"`
	Channel  *ChannelKind     `json:"channel"`
}

type TrafficPoint struct {
	Hour     string `json:"hour"`
	Inbound  int    `json:"inbound"`
	Outbound int    `json:"outbound"`
}

type OverviewStats struct {
	ActiveChannels        int     `json:"activeChannels"`
	TotalChannels         int     `json:"totalChannels"`
	MessagesIngestedToday int     `json:"messagesIngestedToday"`
	AIActionsToday        int     `json:"aiActionsToday"`
	OutboxPending         int     `json:"outboxPending"`
	ErrorRate             float64 `json:"errorRate"`
	P95LatencyMs          int     `json:"p95LatencyMs"`
}

type PlatformSettings struct {
	Environment      string   `json:"environment"`
	Region           string   `json:"region"`
	RetentionDays    int      `json:"retentionDays"`
	WebhookURL       string   `json:"webhookUrl"`
	EnabledProviders []string `json:"enabledProviders"`
}
