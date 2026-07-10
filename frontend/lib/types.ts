export type Platform = 'telegram' | 'wecom'
export type OpportunityStatus = 'pending' | 'replied' | 'ignored'
export type Priority = 'low' | 'normal' | 'high' | 'urgent'
export type MessageSource = 'human' | 'ai' | null
export type AgentAnalysisStatus = 'not_requested' | 'queued' | 'running' | 'completed' | 'failed'
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
