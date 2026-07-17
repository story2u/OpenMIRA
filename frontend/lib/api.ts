import { createAuthApi } from '@story2u/radar-api/auth'
import { createRadarApiClient } from '@story2u/radar-api/client'
import { createMessagesApi } from '@story2u/radar-api/messages'
import { createOpportunityActionsApi } from '@story2u/radar-api/opportunity-actions'
import { createOpportunitiesApi } from '@story2u/radar-api/opportunities'
import { createSettingsApi } from '@story2u/radar-api/settings'
import { createSubscriptionsApi } from '@story2u/radar-api/subscriptions'
import { createTemplatesApi } from '@story2u/radar-api/templates'
import { createTelegramApi } from '@story2u/radar-api/telegram'
import type { MessagePage } from '@story2u/radar-contracts/messages'
import type { Opportunity as ContractOpportunity } from '@story2u/radar-contracts/opportunities'
import type {
  AuthUser,
  AuthTokenResponse,
  AgentAction,
  AgentAnalysisStatus,
  ChatMessage,
  ExtractedContacts,
  LinkVerification,
  OAuthProvider,
  Opportunity,
  InternalOpportunityStatus,
  ReplyTemplate,
  PlanEntitlements,
  SubscriptionUsage,
  SubscriptionCatalogPlan,
  SubscriptionManagement,
  TelegramDialog,
  TelegramConnection,
  TelegramConnectionAttempt,
  TelegramConnectionHealth,
  TelegramMtprotoDialog,
  TelegramUserConfig,
  TelegramUserConfigUpdate,
  DetectionSettings,
  WorkSchedule,
  NotificationSettings,
  SettingsBundle,
  WeComConnection,
  WeComConnectionCreate,
  WeComArchiveConnection,
  WeComArchiveConnectionCreate,
  PasswordActionResponse,
  JobFeedbackType,
  JobOpportunityDetail,
  JobsPage,
  JobSearchProfile,
  JobSearchProfileInput,
  JobSearchProfilePreview,
} from './types'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? ''
const AUTH_TOKEN_KEY = 'im_assistant_access_token'

interface ApiOpportunity {
  id: string
  platform: Opportunity['platform']
  contactName: string
  contactAvatar?: string
  summary: string
  matchedKeywords: string[]
  confidenceScore: number
  status: Opportunity['status']
  internalStatus?: Opportunity['internalStatus']
  priority: Opportunity['priority']
  lastMessagePreview: string
  createdAt: string
  sourceType?: Opportunity['sourceType']
  groupName?: string | null
  groupMemberRole?: Opportunity['groupMemberRole']
  rawMessageLinks?: string[]
  linkVerification?: LinkVerification
  extractedContacts?: ExtractedContacts
  friendRequestStatus?: Opportunity['friendRequestStatus']
  sopStage?: Opportunity['sopStage']
  trustScore?: number
  agentActions?: AgentAction[]
  agentAnalysisStatus?: AgentAnalysisStatus
  agentAnalysisError?: string | null
  agentAnalyzedAt?: string | null
  attentionRequired?: boolean
  archivedAt?: string | null
  archivedByUserId?: string | null
  archiveReason?: string | null
  aiReplyDraft?: string | null
  finalReply?: string | null
  detectionReason?: string | null
  assignedTo?: string | null
}

const defaultLinkVerification: LinkVerification = {
  status: 'unverified',
  verifiedAt: null,
  riskReasons: [],
  resolvedInfo: null,
}

const defaultContacts: ExtractedContacts = {
  phone: null,
  email: null,
  telegramHandle: null,
  wecomId: null,
  extractionSource: null,
}

const friendRequestStatuses = new Set<Opportunity['friendRequestStatus']>([
  'not_sent',
  'pending',
  'accepted',
  'rejected',
  'n/a',
])
const sopStages = new Set<Opportunity['sopStage']>([
  'detected',
  'analyzing',
  'verified',
  'contact_extracted',
  'friend_requested',
  'ready_to_chat',
  'chatting',
  'closed',
])
const linkVerificationStatuses = new Set<LinkVerification['status']>([
  'unverified',
  'verifying',
  'safe',
  'suspicious',
  'malicious',
])
const contactExtractionSources = new Set<NonNullable<ExtractedContacts['extractionSource']>>([
  'message_text',
  'link_content',
  'sop_manual',
])
const agentActionTypes = new Set<AgentAction['actionType']>([
  'send_email',
  'add_friend',
  'private_message',
  'notify_user',
])

function apiUrl(path: string) {
  return `${API_BASE_URL}${path}`
}

export function getAuthToken(): string | null {
  if (typeof window === 'undefined') return null
  return window.localStorage.getItem(AUTH_TOKEN_KEY)
}

export function setAuthToken(token: string | null) {
  if (typeof window === 'undefined') return
  if (token) {
    window.localStorage.setItem(AUTH_TOKEN_KEY, token)
  } else {
    window.localStorage.removeItem(AUTH_TOKEN_KEY)
  }
}

const sharedApiClient = createRadarApiClient({
  baseUrl: API_BASE_URL,
  fetch: (input, init) => fetch(input, init),
  getAccessToken: getAuthToken,
})
const sharedAuthApi = createAuthApi(sharedApiClient)
const sharedMessagesApi = createMessagesApi(sharedApiClient)
const sharedOpportunityActionsApi = createOpportunityActionsApi(sharedApiClient)
const sharedOpportunitiesApi = createOpportunitiesApi(sharedApiClient)
const sharedSettingsApi = createSettingsApi(sharedApiClient)
const sharedSubscriptionsApi = createSubscriptionsApi(sharedApiClient)
const sharedTemplatesApi = createTemplatesApi(sharedApiClient)
const sharedTelegramApi = createTelegramApi(sharedApiClient)

async function fetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getAuthToken()
  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  const response = await fetch(apiUrl(path), {
    ...init,
    headers,
    cache: 'no-store',
  })
  if (!response.ok) {
    let detail = `API ${path} failed with ${response.status}`
    try {
      const body = (await response.json()) as { detail?: unknown }
      if (typeof body.detail === 'string') {
        detail = body.detail
      }
    } catch {
      // Keep the HTTP status fallback when the response is not JSON.
    }
    throw new Error(detail)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

function normalizeSourceType(value: string | undefined): Opportunity['sourceType'] {
  return value === 'group' ? 'group' : 'private'
}

function normalizeGroupMemberRole(value: string | undefined): Opportunity['groupMemberRole'] {
  return value === 'unknown' ? 'unknown' : 'member'
}

function normalizeFriendRequestStatus(value: string | undefined, sourceType: Opportunity['sourceType']) {
  if (value && friendRequestStatuses.has(value as Opportunity['friendRequestStatus'])) {
    return value as Opportunity['friendRequestStatus']
  }
  return sourceType === 'group' ? 'not_sent' : 'n/a'
}

function normalizeSopStage(value: string | undefined): Opportunity['sopStage'] {
  return value && sopStages.has(value as Opportunity['sopStage'])
    ? value as Opportunity['sopStage']
    : 'detected'
}

function normalizeLinkVerification(value: unknown): LinkVerification {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return defaultLinkVerification
  const candidate = value as Partial<LinkVerification>
  return {
    status: candidate.status && linkVerificationStatuses.has(candidate.status) ? candidate.status : 'unverified',
    verifiedAt: typeof candidate.verifiedAt === 'string' ? candidate.verifiedAt : null,
    riskReasons: Array.isArray(candidate.riskReasons)
      ? candidate.riskReasons.filter((reason): reason is string => typeof reason === 'string')
      : [],
    resolvedInfo: typeof candidate.resolvedInfo === 'string' ? candidate.resolvedInfo : null,
  }
}

function normalizeExtractedContacts(value: unknown): ExtractedContacts {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return defaultContacts
  const candidate = value as Partial<ExtractedContacts>
  const textOrNull = (entry: unknown) => typeof entry === 'string' ? entry : null
  return {
    phone: textOrNull(candidate.phone),
    email: textOrNull(candidate.email),
    telegramHandle: textOrNull(candidate.telegramHandle),
    wecomId: textOrNull(candidate.wecomId),
    extractionSource:
      candidate.extractionSource && contactExtractionSources.has(candidate.extractionSource)
        ? candidate.extractionSource
        : null,
  }
}

function normalizeAgentActions(value: unknown): AgentAction[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((entry) => {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return []
    const candidate = entry as Partial<AgentAction>
    if (!candidate.actionType || !agentActionTypes.has(candidate.actionType) || typeof candidate.reason !== 'string') {
      return []
    }
    return [{
      actionType: candidate.actionType,
      reason: candidate.reason,
      target: typeof candidate.target === 'string' ? candidate.target : null,
      draft: typeof candidate.draft === 'string' ? candidate.draft : null,
      requiresApproval: candidate.requiresApproval !== false,
    }]
  })
}

export function toOpportunity(item: ApiOpportunity | ContractOpportunity): Opportunity {
  const sourceType = normalizeSourceType(item.sourceType)
  const detail = item as ApiOpportunity
  return {
    id: item.id,
    platform: item.platform,
    contactName: item.contactName,
    contactAvatar: item.contactAvatar || '/placeholder-user.jpg',
    summary: item.summary,
    matchedKeywords: item.matchedKeywords ?? [],
    confidenceScore: item.confidenceScore,
    status: item.status,
    internalStatus: item.internalStatus ?? 'pending_human',
    priority: item.priority,
    lastMessagePreview: item.lastMessagePreview,
    createdAt: item.createdAt,
    sourceType,
    groupName: item.groupName ?? null,
    groupMemberRole: normalizeGroupMemberRole(item.groupMemberRole),
    rawMessageLinks: item.rawMessageLinks ?? [],
    linkVerification: normalizeLinkVerification(item.linkVerification),
    extractedContacts: normalizeExtractedContacts(item.extractedContacts),
    friendRequestStatus: normalizeFriendRequestStatus(item.friendRequestStatus, sourceType),
    sopStage: normalizeSopStage(item.sopStage),
    trustScore: item.trustScore ?? 70,
    agentActions: normalizeAgentActions(item.agentActions),
    agentAnalysisStatus: item.agentAnalysisStatus ?? 'not_requested',
    agentAnalysisError: item.agentAnalysisError ?? null,
    agentAnalyzedAt: item.agentAnalyzedAt ?? null,
    attentionRequired: item.attentionRequired ?? false,
    archivedAt: item.archivedAt ?? null,
    archivedByUserId: item.archivedByUserId ?? null,
    archiveReason: item.archiveReason ?? null,
    aiReplyDraft: detail.aiReplyDraft ?? null,
    finalReply: detail.finalReply ?? null,
    detectionReason: detail.detectionReason ?? null,
    assignedTo: detail.assignedTo ?? null,
  }
}

export async function fetchOpportunities(archive: 'active' | 'archived' | 'all' = 'active'): Promise<Opportunity[]> {
  const items = await sharedOpportunitiesApi.list({ archive, limit: 200 })
  return items.map(toOpportunity)
}

export async function fetchOpportunity(
  opportunityId: string,
  signal?: AbortSignal,
): Promise<Opportunity> {
  return toOpportunity(await sharedOpportunitiesApi.getById(opportunityId, { signal }))
}

export function fetchMessagePage(
  opportunityId: string,
  options: { limit?: number; offset?: number; signal?: AbortSignal } = {},
): Promise<MessagePage> {
  return sharedMessagesApi.page({
    opportunity_id: opportunityId,
    limit: options.limit ?? 200,
    offset: options.offset ?? 0,
  }, { signal: options.signal })
}

export interface JobFilters {
  profileId?: string
  query?: string
  source?: 'telegram' | 'wecom'
  postedFrom?: string
  workMode?: string
  employmentType?: string
  seniority?: string
  country?: string
  city?: string
  salaryMin?: number
  salaryCurrency?: string
  salaryDisclosed?: boolean
  degreeLevel?: string
  englishLevel?: string
  visaSponsorship?: boolean
  minimumMatchScore?: number
  ageRequirementPresent?: boolean
  excludeExpired?: boolean
  sort?: 'match' | 'newest' | 'salary' | 'confidence' | 'source_reliability'
  limit?: number
  offset?: number
}

export async function fetchJobs(filters: JobFilters = {}): Promise<JobsPage> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value === undefined || value === '' || value === null) continue
    const apiKey = key.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`)
    params.set(apiKey, String(value))
  }
  return fetchJson<JobsPage>(`/api/v1/jobs?${params.toString()}`)
}

export async function fetchJob(opportunityId: string, profileId?: string): Promise<JobOpportunityDetail> {
  const suffix = profileId ? `?profile_id=${encodeURIComponent(profileId)}` : ''
  return fetchJson<JobOpportunityDetail>(`/api/v1/jobs/${opportunityId}${suffix}`)
}

export async function submitJobFeedback(
  opportunityId: string,
  feedbackType: JobFeedbackType,
  note?: string,
): Promise<void> {
  await fetchJson(`/api/v1/jobs/${opportunityId}/feedback`, {
    method: 'POST',
    body: JSON.stringify({ feedbackType, note: note || null }),
  })
}

export async function fetchJobSearchProfiles(): Promise<JobSearchProfile[]> {
  return fetchJson<JobSearchProfile[]>('/api/v1/job-search-profiles')
}

export async function createJobSearchProfile(payload: JobSearchProfileInput): Promise<JobSearchProfile> {
  return fetchJson<JobSearchProfile>('/api/v1/job-search-profiles', {
    method: 'POST', body: JSON.stringify(payload),
  })
}

export async function updateJobSearchProfile(
  profileId: string,
  payload: Partial<JobSearchProfileInput>,
): Promise<JobSearchProfile> {
  return fetchJson<JobSearchProfile>(`/api/v1/job-search-profiles/${profileId}`, {
    method: 'PATCH', body: JSON.stringify(payload),
  })
}

export async function deleteJobSearchProfile(profileId: string): Promise<void> {
  await fetchJson(`/api/v1/job-search-profiles/${profileId}`, { method: 'DELETE' })
}

export async function parseJobSearchProfile(text: string): Promise<JobSearchProfilePreview> {
  return fetchJson<JobSearchProfilePreview>('/api/v1/job-search-profiles/parse', {
    method: 'POST', body: JSON.stringify({ text }),
  })
}

export async function sendManualReply(
  opportunityId: string,
  text: string,
  idempotencyKey: string,
): Promise<{ opportunity: Opportunity; message: ChatMessage; messageTotal: number }> {
  const result = await sharedOpportunityActionsApi.manualReply(
    opportunityId,
    { text, mark_following: true },
    idempotencyKey,
  )
  return {
    opportunity: toOpportunity(result.opportunity),
    message: result.message,
    messageTotal: result.messageTotal,
  }
}

export async function generateAIDraft(opportunityId: string): Promise<string> {
  return (await sharedOpportunityActionsApi.generateAIDraft(opportunityId)).draft
}

export async function updateOpportunityStatus(
  opportunityId: string,
  nextStatus: InternalOpportunityStatus,
): Promise<Opportunity> {
  return toOpportunity(await sharedOpportunityActionsApi.updateStatus(opportunityId, nextStatus))
}

export async function claimOpportunity(opportunityId: string): Promise<Opportunity> {
  return toOpportunity(await sharedOpportunityActionsApi.claim(opportunityId))
}

/** 好友申请状态流转（发送/确认通过/确认被拒/重试）；非法流转后端返回 409。 */
export async function updateFriendRequest(
  opportunityId: string,
  status: Exclude<Opportunity['friendRequestStatus'], 'n/a'>,
): Promise<Opportunity> {
  const item = await fetchJson<ApiOpportunity>(`/api/v1/opportunities/${opportunityId}/friend-request`, {
    method: 'POST',
    body: JSON.stringify({ status }),
  })
  return toOpportunity(item)
}

export async function archiveOpportunity(opportunityId: string, reason?: string): Promise<Opportunity> {
  return toOpportunity(
    await fetchJson<ApiOpportunity>(`/api/v1/opportunities/${opportunityId}/archive`, {
      method: 'POST',
      body: JSON.stringify({ reason: reason || null }),
    }),
  )
}

export async function restoreOpportunity(opportunityId: string): Promise<Opportunity> {
  return toOpportunity(
    await fetchJson<ApiOpportunity>(`/api/v1/opportunities/${opportunityId}/restore`, { method: 'POST' }),
  )
}

export async function bulkArchiveOpportunities(opportunityIds: string[]): Promise<Opportunity[]> {
  const result = await fetchJson<{ archivedCount: number; opportunities: ApiOpportunity[] }>(
    '/api/v1/opportunities/bulk-archive',
    {
      method: 'POST',
      body: JSON.stringify({ opportunityIds, reason: null }),
    },
  )
  return result.opportunities.map(toOpportunity)
}

export async function enqueueAgentAnalysis(opportunityId: string): Promise<{
  messageId: string
  status: AgentAnalysisStatus
}> {
  return fetchJson(`/api/v1/opportunities/${opportunityId}/agent-analysis`, {
    method: 'POST',
    headers: { 'Idempotency-Key': crypto.randomUUID() },
  })
}

export async function fetchSubscriptionPlans(): Promise<PlanEntitlements[]> {
  return sharedSubscriptionsApi.plans()
}

export async function fetchMySubscription(): Promise<SubscriptionUsage> {
  return sharedSubscriptionsApi.usage()
}

export async function fetchSubscriptionCatalog(): Promise<SubscriptionCatalogPlan[]> {
  return sharedSubscriptionsApi.catalog()
}

export async function syncMySubscription(): Promise<SubscriptionUsage> {
  return sharedSubscriptionsApi.sync()
}

export async function fetchSubscriptionManagement(): Promise<SubscriptionManagement> {
  return sharedSubscriptionsApi.management('web')
}

export async function fetchReplyTemplates(): Promise<ReplyTemplate[]> {
  return sharedTemplatesApi.list()
}

export async function fetchOAuthAuthorizeUrl(provider: OAuthProvider): Promise<string> {
  return sharedAuthApi.getOAuthAuthorizeUrl(provider)
}

export async function fetchMe(accessToken?: string): Promise<AuthUser> {
  return sharedAuthApi.getCurrentUser(accessToken)
}

export async function passwordLogin(email: string, password: string): Promise<AuthTokenResponse> {
  return fetchJson<AuthTokenResponse>('/api/v1/auth/password/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function requestPasswordReset(email: string): Promise<PasswordActionResponse> {
  return fetchJson<PasswordActionResponse>('/api/v1/auth/password/reset/request', {
    method: 'POST',
    body: JSON.stringify({ email }),
  })
}

export async function confirmPasswordReset(payload: {
  newPassword: string
  token?: string
  email?: string
  code?: string
}): Promise<PasswordActionResponse> {
  return fetchJson<PasswordActionResponse>('/api/v1/auth/password/reset/confirm', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<PasswordActionResponse> {
  return fetchJson<PasswordActionResponse>('/api/v1/auth/password/change', {
    method: 'POST',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}

export async function fetchTelegramUserConfig(): Promise<TelegramUserConfig> {
  return fetchJson<TelegramUserConfig>('/api/v1/integrations/telegram-user/config')
}

export async function updateTelegramUserConfig(
  payload: TelegramUserConfigUpdate,
): Promise<TelegramUserConfig> {
  return fetchJson<TelegramUserConfig>('/api/v1/integrations/telegram-user/config', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function updateTelegramMonitorRetention(monitorIds: string[]): Promise<TelegramUserConfig> {
  return fetchJson<TelegramUserConfig>('/api/v1/integrations/telegram-user/monitors/retention', {
    method: 'PUT',
    body: JSON.stringify({ monitorIds }),
  })
}

export async function sendTelegramCode(apiId: number, apiHash: string, phone: string) {
  return fetchJson<{ loginId: string; expiresInSeconds: number }>('/api/v1/integrations/telegram-user/send-code', {
    method: 'POST',
    body: JSON.stringify({ apiId, apiHash, phone }),
  })
}

export async function verifyTelegramCode(loginId: string, code: string, password?: string) {
  return fetchJson<{ status: string; config: TelegramUserConfig | null }>(
    '/api/v1/integrations/telegram-user/verify-code',
    {
      method: 'POST',
      body: JSON.stringify({ loginId, code, password: password || null }),
    },
  )
}

export async function fetchTelegramDialogs(): Promise<TelegramDialog[]> {
  return fetchJson<TelegramDialog[]>('/api/v1/integrations/telegram-user/dialogs')
}

export async function fetchTelegramConnectionHealth(): Promise<TelegramConnectionHealth> {
  return sharedTelegramApi.health()
}

export async function fetchTelegramConnections(): Promise<TelegramConnection[]> {
  return sharedTelegramApi.connections()
}

export async function startTelegramBotChatConnection(): Promise<TelegramConnectionAttempt> {
  return sharedTelegramApi.startBotChat()
}

export async function startTelegramBusinessConnection(): Promise<TelegramConnectionAttempt> {
  return sharedTelegramApi.startBusiness()
}

export async function startTelegramMtprotoQrConnection(): Promise<TelegramConnectionAttempt> {
  return sharedTelegramApi.startMtprotoQr()
}

export async function fetchTelegramConnectionAttempt(attemptId: string): Promise<TelegramConnectionAttempt> {
  return sharedTelegramApi.attempt(attemptId)
}

export async function cancelTelegramConnectionAttempt(attemptId: string): Promise<TelegramConnectionAttempt> {
  return sharedTelegramApi.cancelAttempt(attemptId)
}

export async function updateTelegramConnection(
  connectionId: string,
  enabled: boolean,
): Promise<TelegramConnection> {
  return sharedTelegramApi.updateConnection(connectionId, enabled)
}

export async function updateTelegramConnectionSource(
  sourceId: string,
  autoReplyEnabled: boolean,
): Promise<TelegramConnection> {
  return fetchJson<TelegramConnection>(`/api/v1/integrations/telegram/sources/${sourceId}`, {
    method: 'PATCH',
    body: JSON.stringify({ autoReplyEnabled }),
  })
}

export async function deleteTelegramConnection(connectionId: string): Promise<void> {
  return sharedTelegramApi.deleteConnection(connectionId)
}

export async function deleteTelegramConnectionSource(sourceId: string): Promise<void> {
  return sharedTelegramApi.deleteSource(sourceId)
}

export async function generateOpportunityAiDraft(
  opportunityId: string,
): Promise<{ opportunity_id: string; draft: string }> {
  return fetchJson<{ opportunity_id: string; draft: string }>(
    `/api/v1/opportunities/${opportunityId}/ai-draft`,
    { method: 'POST' },
  )
}

export async function fetchTelegramMtprotoDialogs(connectionId: string): Promise<TelegramMtprotoDialog[]> {
  return sharedTelegramApi.dialogs(connectionId)
}

export async function addTelegramMtprotoSource(
  connectionId: string,
  chatId: string,
): Promise<TelegramConnection> {
  return sharedTelegramApi.addSource(connectionId, chatId)
}

export async function fetchWeComConnections(): Promise<WeComConnection[]> {
  return fetchJson<WeComConnection[]>('/api/v1/integrations/wecom/connections')
}

export async function createWeComConnection(
  body: WeComConnectionCreate,
): Promise<WeComConnection> {
  return fetchJson<WeComConnection>('/api/v1/integrations/wecom/connections', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function verifyWeComConnection(connectionId: string): Promise<WeComConnection> {
  return fetchJson<WeComConnection>(
    `/api/v1/integrations/wecom/connections/${connectionId}/verify`,
    { method: 'POST' },
  )
}

export async function deleteWeComConnection(connectionId: string): Promise<void> {
  return fetchJson<void>(`/api/v1/integrations/wecom/connections/${connectionId}`, {
    method: 'DELETE',
  })
}

export async function fetchWeComArchiveConnections(): Promise<WeComArchiveConnection[]> {
  return fetchJson<WeComArchiveConnection[]>('/api/v1/integrations/wecom/archive-connections')
}

export async function createWeComArchiveConnection(
  body: WeComArchiveConnectionCreate,
): Promise<WeComArchiveConnection> {
  return fetchJson<WeComArchiveConnection>('/api/v1/integrations/wecom/archive-connections', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function verifyWeComArchiveConnection(connectionId: string): Promise<void> {
  await fetchJson<{ accepted: boolean }>(
    `/api/v1/integrations/wecom/archive-connections/${connectionId}/verify`,
    { method: 'POST' },
  )
}

export async function syncWeComArchiveConnection(connectionId: string): Promise<void> {
  await fetchJson<{ accepted: boolean }>(
    `/api/v1/integrations/wecom/archive-connections/${connectionId}/sync`,
    { method: 'POST' },
  )
}

export async function deleteWeComArchiveConnection(connectionId: string): Promise<void> {
  return fetchJson<void>(`/api/v1/integrations/wecom/archive-connections/${connectionId}`, {
    method: 'DELETE',
  })
}

// MARK: 用户级设置（与 iOS/Android 共享同一后端设置源）

export async function fetchSettings(): Promise<SettingsBundle> {
  return sharedSettingsApi.get()
}

export async function updateDetectionSettings(
  body: DetectionSettings,
): Promise<DetectionSettings> {
  return sharedSettingsApi.updateDetection(body)
}

export async function updateWorkSchedule(
  body: Omit<WorkSchedule, 'isDefault'>,
): Promise<WorkSchedule> {
  return sharedSettingsApi.updateWorkSchedule(body)
}

export async function updateNotificationSettings(
  body: NotificationSettings,
): Promise<NotificationSettings> {
  return sharedSettingsApi.updateNotifications(body)
}
