export type Platform = 'telegram' | 'wecom'
export type PlanCode = 'free' | 'plus' | 'pro' | 'max'
export type SubscriptionStatus = 'active' | 'trialing' | 'past_due' | 'canceled' | 'inactive'
export type BillingStore = 'app_store' | 'play_store' | 'paddle' | 'test_store' | 'unknown'
export type BillingInterval = 'monthly' | 'annual' | 'unknown'
export type OpportunityStatus = 'pending' | 'replied' | 'ignored'
export type Priority = 'low' | 'normal' | 'high' | 'urgent'
export type MessageSource = 'human' | 'ai' | null
export type AgentAnalysisStatus =
  | 'not_requested'
  | 'quota_exceeded'
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
export type AgentActionType = 'send_email' | 'add_friend' | 'private_message' | 'notify_user'

export type SourceType = 'group' | 'private'
export type GroupMemberRole = 'member' | 'unknown'
export type LinkVerificationStatus = 'unverified' | 'verifying' | 'safe' | 'suspicious' | 'malicious'
export type ContactExtractionSource = 'message_text' | 'link_content' | 'sop_manual' | null
export type FriendRequestStatus = 'not_sent' | 'pending' | 'accepted' | 'rejected' | 'n/a'
export type SopStage =
  | 'detected'
  | 'analyzing'
  | 'verified'
  | 'contact_extracted'
  | 'friend_requested'
  | 'ready_to_chat'
  | 'chatting'
  | 'closed'

export interface LinkVerification {
  status: LinkVerificationStatus
  verifiedAt: string | null
  riskReasons: string[]
  resolvedInfo: string | null
}

export interface ExtractedContacts {
  phone: string | null
  email: string | null
  telegramHandle: string | null
  wecomId: string | null
  extractionSource: ContactExtractionSource
}

export interface AgentAction {
  actionType: AgentActionType
  reason: string
  target: string | null
  draft: string | null
  requiresApproval: boolean
}

export interface Opportunity {
  id: string
  platform: Platform
  contactName: string
  contactAvatar: string
  summary: string
  matchedKeywords: string[]
  confidenceScore: number
  status: OpportunityStatus
  priority: Priority
  lastMessagePreview: string
  createdAt: string
  sourceType: SourceType
  groupName: string | null
  groupMemberRole: GroupMemberRole
  rawMessageLinks: string[]
  linkVerification: LinkVerification
  extractedContacts: ExtractedContacts
  friendRequestStatus: FriendRequestStatus
  sopStage: SopStage
  trustScore: number
  agentActions: AgentAction[]
  agentAnalysisStatus: AgentAnalysisStatus
  agentAnalysisError: string | null
  agentAnalyzedAt: string | null
  attentionRequired: boolean
  archivedAt: string | null
  archivedByUserId: string | null
  archiveReason: string | null
}

export interface ChatMessage {
  id: string
  senderName: string
  content: string
  isFromContact: boolean
  sentAt: string
  source: MessageSource
}

export interface ReplyTemplate {
  id: string
  title: string
  content: string
  category: string
}

export interface AuthUser {
  id: string
  email: string
  displayName: string
  avatarUrl: string
  isAdmin: boolean
}

export interface AuthTokenResponse {
  accessToken: string
  tokenType: string
  user: AuthUser
}

export type OAuthProvider = 'google' | 'apple'

export interface OAuthAuthorizeResponse {
  authorizationUrl: string
}

export interface TelegramMonitor {
  id: string
  enabled: boolean
  name: string
  chatId: string
  chatTitle: string | null
  backfillLimit: number
  quotaPaused: boolean
  quotaReason: string | null
  lastError: string | null
  updatedAt: string | null
}

export interface TelegramUserConfig {
  apiId: number | null
  apiHashConfigured: boolean
  sessionConfigured: boolean
  monitors: TelegramMonitor[]
  monitorLimit: number
  canCreateMore: boolean
  activeMonitorCount: number
  storedMonitorCount: number
  retentionSelectionRequired: boolean
  retentionSelectedAt: string | null
  updatedAt: string | null
}

export interface TelegramUserConfigUpdate {
  enabled: boolean
  apiId?: number | null
  apiHash?: string
  sessionString?: string
  chats: Array<string | number>
  backfillLimit: number
}

export interface TelegramDialog {
  id: number
  name: string
  username: string | null
}

export type TelegramConnectionType = 'bot_chat' | 'business' | 'mtproto_qr'
export type TelegramConnectionStatus = 'pending' | 'connected' | 'disabled' | 'error' | 'expired'
export type TelegramConnectionAttemptStatus = 'pending' | 'completed' | 'cancelled' | 'expired' | 'failed'
export type TelegramSourceType = 'group' | 'channel' | 'private'

export interface TelegramConnectionHealth {
  mode: 'mock' | 'live'
  botConfigured: boolean
  botUsername: string | null
  businessAvailable: boolean
  mtprotoQrAvailable: boolean
  listenerMode: string
  legacyMonitoringActive: boolean
  legacyActiveSourceCount: number
  message: string | null
}

export interface TelegramConnectionSource {
  id: string
  connectionId: string
  sourceType: TelegramSourceType
  externalChatId: string
  displayName: string
  username: string | null
  enabled: boolean
  quotaPaused: boolean
  quotaReason: string | null
  lastError: string | null
  updatedAt: string
}

export interface TelegramConnection {
  id: string
  connectionType: TelegramConnectionType
  status: TelegramConnectionStatus
  enabled: boolean
  label: string
  capabilities: Record<string, boolean>
  lastError: string | null
  lastCheckedAt: string | null
  updatedAt: string
  sources: TelegramConnectionSource[]
}

export interface TelegramConnectionAttempt {
  id: string
  connectionType: TelegramConnectionType
  status: TelegramConnectionAttemptStatus
  expiresAt: string
  connectionId: string | null
  error: string | null
  telegramUrl: string | null
  qrCodeUrl: string | null
  instructions: string[]
  localMock: boolean
}

export interface TelegramMtprotoDialog {
  id: string
  sourceType: Extract<TelegramSourceType, 'group' | 'channel'>
  displayName: string
  username: string | null
}

export interface PlanEntitlements {
  planCode: PlanCode
  telegramGroupLimit: number | null
  wecomGroupLimit: number | null
  combinedGroupLimit: number
  piAgentAnalysisMonthlyLimit: number
}

export interface SubscriptionUsage {
  planCode: PlanCode
  subscriptionStatus: SubscriptionStatus
  periodStart: string
  periodEnd: string
  cancelAtPeriodEnd: boolean
  entitlements: PlanEntitlements
  telegramGroupsUsed: number
  wecomGroupsUsed: number
  combinedGroupsUsed: number
  aiAnalysesConsumed: number
  aiAnalysesReserved: number
  aiAnalysesRemaining: number
  effectiveStore: BillingStore | null
  billingInterval: BillingInterval | null
  billingPeriodStart: string | null
  billingPeriodEnd: string | null
  usagePeriodStart: string
  usagePeriodEnd: string
  entitlementExpiresAt: string | null
  willRenew: boolean
  billingIssue: boolean
  multipleActiveSubscriptions: boolean
  managementUrl: string | null
  lastSyncedAt: string | null
}

export interface SubscriptionCatalogPlan {
  planCode: PlanCode
  displayName: string
  rank: number
  entitlements: PlanEntitlements
  availableIntervals: BillingInterval[]
  revenuecatPackageIdentifiers: string[]
}

export interface SubscriptionManagement {
  store: BillingStore | null
  managementUrl: string | null
  instruction: string
  canOpenInCurrentClient: boolean
}

export interface DetectionSettings {
  keywords: string[]
  aiSemanticsEnabled: boolean
}

export interface WorkScheduleSlot {
  weekday: number
  start: string
  end: string
}

export interface WorkSchedule {
  timezone: string
  slots: WorkScheduleSlot[]
  autoReplyOutsideHours: boolean
  isDefault: boolean
}

export interface NotificationSettings {
  newOpportunityEnabled: boolean
  aiRepliedEnabled: boolean
  dailyDigestEnabled: boolean
  urgentOnly: boolean
}

export interface SettingsCapabilities {
  pushAvailable: boolean
  wecomUserBindingAvailable: boolean
}

export interface SettingsBundle {
  detection: DetectionSettings
  workSchedule: WorkSchedule
  notifications: NotificationSettings
  capabilities: SettingsCapabilities
}
