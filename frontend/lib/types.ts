import type {
  AuthToken as ContractAuthToken,
  AuthUser as ContractAuthUser,
} from '@story2u/radar-contracts/auth'

export type {
  DetectionSettings,
  NotificationSettings,
  SettingsBundle,
  SettingsCapabilities,
  WorkSchedule,
  WorkScheduleSlot,
} from '@story2u/radar-contracts/settings'
export type {
  TelegramConnection,
  TelegramConnectionAttempt,
  TelegramConnectionAttemptStatus,
  TelegramConnectionHealth,
  TelegramConnectionSource,
  TelegramConnectionStatus,
  TelegramConnectionType,
  TelegramMtprotoDialog,
  TelegramSourceType,
} from '@story2u/radar-contracts/telegram'
export type {
  BillingInterval,
  BillingStore,
  PlanCode,
  PlanEntitlements,
  SubscriptionCatalogPlan,
  SubscriptionManagement,
  SubscriptionStatus,
  SubscriptionUsage,
} from '@story2u/radar-contracts/subscriptions'

export type Platform = 'telegram' | 'wecom'
export type OpportunityStatus = 'pending' | 'replied' | 'ignored'
export type InternalOpportunityStatus =
  | 'pending_human'
  | 'ai_auto_reply'
  | 'replied'
  | 'following'
  | 'ignored'
  | 'closed'
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
  internalStatus: InternalOpportunityStatus
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
  aiReplyDraft: string | null
  finalReply: string | null
  detectionReason: string | null
  assignedTo: string | null
}

export interface JobSearchProfileInput {
  name: string
  isDefault: boolean
  enabled: boolean
  targetRoles: string[]
  excludedRoles: string[]
  targetIndustries: string[]
  preferredSeniority: JobSeniority[]
  candidateSkills: string[]
  yearsExperience: number | null
  educationLevel: string | null
  englishLevel: string | null
  otherLanguages: string[]
  preferredCountries: string[]
  preferredCities: string[]
  preferredTimezones: string[]
  workModes: JobWorkMode[]
  employmentTypes: JobEmploymentType[]
  minimumSalary: number | null
  salaryCurrency: string | null
  salaryPeriod: SalaryPeriod | null
  visaSponsorshipRequired: boolean | null
  relocationAcceptable: boolean | null
  requiredKeywords: string[]
  preferredKeywords: string[]
  excludedKeywords: string[]
  requireSalaryDisclosed: boolean
  minimumMatchScore: number
  notificationEnabled: boolean
}

export interface JobSearchProfile extends JobSearchProfileInput {
  id: string
  createdAt: string
  updatedAt: string
}

export interface JobSearchProfilePreview extends JobSearchProfileInput {
  requiresConfirmation: true
}

export interface JobMatch {
  eligibility: JobEligibility
  matchScore: number
  matchedReasons: string[]
  mismatchReasons: string[]
  unknownConstraints: string[]
  scoreBreakdown: Record<string, number>
}

export interface JobSource {
  id: string
  channel: Platform
  chatName: string | null
  authorName: string | null
  postedAt: string
  sourceMessageUrl: string | null
  reliabilityScore: number
}

export interface JobOpportunity {
  opportunityId: string
  jobTitle: string
  companyName: string | null
  sourceChannel: Platform
  sourceChatName: string | null
  postedAt: string
  locationText: string | null
  countryCode: string | null
  city: string | null
  workMode: JobWorkMode
  employmentType: JobEmploymentType
  seniority: JobSeniority
  salaryRaw: string | null
  salaryMin: number | null
  salaryMax: number | null
  salaryCurrency: string | null
  salaryPeriod: SalaryPeriod
  requiredSkills: string[]
  degreeLevel: string | null
  englishLevel: string | null
  visaSponsorship: boolean | null
  applicationDeadline: string | null
  sourceReliabilityScore: number
  extractionConfidence: number
  sourceCount: number
  conflictingSourceData: boolean
  complianceFlags: string[]
  isExpired: boolean
  match: JobMatch | null
}

export interface JobOpportunityDetail extends JobOpportunity {
  sourceMessageUrl: string | null
  sourceAuthorName: string | null
  department: string | null
  companyIndustry: string | null
  companyStage: string | null
  timezone: string | null
  salaryNegotiable: boolean | null
  equityMentioned: boolean | null
  requirementsSummary: string | null
  preferredSkills: string[]
  minimumYearsExperience: number | null
  maximumYearsExperience: number | null
  degreeRequired: boolean | null
  degreeField: string | null
  otherLanguageRequirements: string[]
  workAuthorizationText: string | null
  relocationSupport: boolean | null
  ageRequirementText: string | null
  ageRequirementPresent: boolean
  applicationUrl: string | null
  contactMethods: Array<{ type: string; value: string }>
  missingFields: string[]
  fieldEvidence: Record<string, string>
  rawExcerpt: string
  expiredReason: string | null
  sources: JobSource[]
}

export interface JobsPage {
  items: JobOpportunity[]
  total: number
  limit: number
  offset: number
  filterSummary: Record<string, unknown>
  profile: JobSearchProfile | null
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

export type AuthUser = ContractAuthUser
export type AuthTokenResponse = ContractAuthToken

export interface PasswordActionResponse {
  message: string
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

export type WeComConnectionStatus = 'pending' | 'active' | 'disabled' | 'error'
export type WeComSourceType =
  | 'private'
  | 'internal_group'
  | 'external_group'
  | 'customer_service'

export interface WeComSource {
  id: string
  connectionId?: string | null
  archiveConnectionId?: string | null
  sourceType: WeComSourceType
  externalConversationId: string
  displayName: string
  receiveCapability: 'app_callback' | 'message_archive' | 'customer_service'
  sendCapability: 'app_message' | 'customer_service' | 'manual_only'
  enabled: boolean
  quotaPaused: boolean
  quotaReason: string | null
  lastMessageAt: string | null
  lastError: string | null
}

export interface WeComConnection {
  id: string
  connectionType: 'internal_app' | 'message_archive' | 'customer_service'
  status: WeComConnectionStatus
  enabled: boolean
  displayName: string
  corpId: string
  agentId: string
  callbackUrl: string
  credentialConfigured: boolean
  lastVerifiedAt: string | null
  lastError: string | null
  updatedAt: string
  sources: WeComSource[]
}

export interface WeComConnectionCreate {
  displayName: string
  corpId: string
  agentId: string
  secret: string
  token: string
  encodingAesKey: string
}
