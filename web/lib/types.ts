export type ChannelKind =
  | "wecom"
  | "feishu"
  | "dingtalk"
  | "whatsapp"
  | "telegram"
  | "email";

export type ChannelStatus = "connected" | "degraded" | "disabled";

export type ReceiveCapability = "webhook" | "polling" | "rpa";
export type SendCapability = "api" | "rpa" | "manual_approval";

export interface Channel {
  id: string;
  kind: ChannelKind;
  name: string;
  status: ChannelStatus;
  receiveCapabilities: ReceiveCapability[];
  sendCapabilities: SendCapability[];
  lastSyncAt: string;
  errorCount24h: number;
  messagesToday: number;
  activeConversations: number;
}

export type PipelineStage =
  | "connector"
  | "ingest"
  | "normalize"
  | "store"
  | "sop_ai"
  | "outbox"
  | "delivery";

export interface PipelineStageStats {
  stage: PipelineStage;
  label: string;
  throughputPerMin: number;
  failures1h: number;
  avgLatencyMs: number;
}

export type MessageDirection = "inbound" | "outbound";
export type MessageEventStatus = "success" | "failed" | "retrying" | "pending";

export interface MessageEvent {
  id: string;
  time: string;
  channel: ChannelKind;
  direction: MessageDirection;
  conversationId: string;
  conversationLabel: string;
  eventType: string;
  status: MessageEventStatus;
  latencyMs: number;
  traceId: string;
}

export type SopStage = "none" | "in_progress" | "waiting_human" | "completed" | "failed";
export type AiStatus = "auto_replying" | "monitoring" | "handed_off" | "idle";

export interface Conversation {
  id: string;
  channel: ChannelKind;
  contactName: string;
  contactHandle: string;
  lastMessagePreview: string;
  lastMessageAt: string;
  assignedOperator: string | null;
  aiStatus: AiStatus;
  sopStage: SopStage;
  sopWorkflowName: string | null;
  unread: number;
  tags: string[];
}

export interface ConversationMessage {
  id: string;
  conversationId: string;
  channel: ChannelKind;
  direction: MessageDirection;
  author: string;
  content: string;
  time: string;
  isAiGenerated?: boolean;
}

export type AiPolicyKind =
  | "intent_classification"
  | "risk_detection"
  | "reply_drafting"
  | "knowledge_retrieval"
  | "tool_calling"
  | "human_handoff"
  | "auto_reply_policy";

export interface AiPolicy {
  id: string;
  kind: AiPolicyKind;
  name: string;
  enabled: boolean;
  priority: number;
  triggerCondition: string;
  fallbackStrategy: string;
  successRate7d: number;
  invocations24h: number;
}

export type WorkflowStatus = "active" | "paused" | "draft";

export interface WorkflowStep {
  id: string;
  name: string;
  condition: string;
  aiAction: string | null;
  humanAction: string | null;
  timeoutMinutes: number;
  fallback: string;
}

export interface SopWorkflow {
  id: string;
  name: string;
  trigger: string;
  channels: ChannelKind[];
  activeConversations: number;
  completionRate: number;
  slaMinutes: number;
  status: WorkflowStatus;
  steps: WorkflowStep[];
}

export type OutboxStatus =
  | "pending"
  | "sending"
  | "failed"
  | "sent"
  | "requires_approval";

export interface OutboxItem {
  id: string;
  createdAt: string;
  channel: ChannelKind;
  conversationId: string;
  conversationLabel: string;
  messageType: string;
  sender: string;
  deliveryMethod: SendCapability;
  status: OutboxStatus;
  retryCount: number;
  lastError: string | null;
}

export interface AuditLogEntry {
  id: string;
  time: string;
  actor: string;
  actorType: "user" | "system" | "ai";
  action: string;
  target: string;
  channel: ChannelKind | null;
  result: "success" | "failure";
  ip: string | null;
}

export interface RecentIncident {
  id: string;
  time: string;
  severity: "critical" | "warning" | "info";
  summary: string;
  channel: ChannelKind | null;
}

export interface TrafficPoint {
  hour: string;
  inbound: number;
  outbound: number;
}
