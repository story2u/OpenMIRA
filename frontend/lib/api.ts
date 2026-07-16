import type {
  AuthUser,
  AuthTokenResponse,
  AgentAction,
  AgentAnalysisStatus,
  ExtractedContacts,
  LinkVerification,
  OAuthAuthorizeResponse,
  OAuthProvider,
  Opportunity,
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

export function toOpportunity(item: ApiOpportunity): Opportunity {
  return {
    id: item.id,
    platform: item.platform,
    contactName: item.contactName,
    contactAvatar: item.contactAvatar || '/placeholder-user.jpg',
    summary: item.summary,
    matchedKeywords: item.matchedKeywords ?? [],
    confidenceScore: item.confidenceScore,
    status: item.status,
    priority: item.priority,
    lastMessagePreview: item.lastMessagePreview,
    createdAt: item.createdAt,
    sourceType: item.sourceType ?? 'private',
    groupName: item.groupName ?? null,
    groupMemberRole: item.groupMemberRole ?? 'member',
    rawMessageLinks: item.rawMessageLinks ?? [],
    linkVerification: item.linkVerification ?? defaultLinkVerification,
    extractedContacts: item.extractedContacts ?? defaultContacts,
    friendRequestStatus: item.friendRequestStatus ?? (item.sourceType === 'group' ? 'not_sent' : 'n/a'),
    sopStage: item.sopStage ?? 'detected',
    trustScore: item.trustScore ?? 70,
    agentActions: item.agentActions ?? [],
    agentAnalysisStatus: item.agentAnalysisStatus ?? 'not_requested',
    agentAnalysisError: item.agentAnalysisError ?? null,
    agentAnalyzedAt: item.agentAnalyzedAt ?? null,
    attentionRequired: item.attentionRequired ?? false,
    archivedAt: item.archivedAt ?? null,
    archivedByUserId: item.archivedByUserId ?? null,
    archiveReason: item.archiveReason ?? null,
  }
}

export async function fetchOpportunities(archive: 'active' | 'archived' | 'all' = 'active'): Promise<Opportunity[]> {
  const items = await fetchJson<ApiOpportunity[]>(`/api/v1/opportunities?limit=200&archive=${archive}`)
  return items.map(toOpportunity)
}

export async function fetchOpportunity(opportunityId: string): Promise<Opportunity> {
  return toOpportunity(await fetchJson<ApiOpportunity>(`/api/v1/opportunities/${opportunityId}`))
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
): Promise<Opportunity> {
  const item = await fetchJson<ApiOpportunity>(
    `/api/v1/opportunities/${opportunityId}/manual-reply`,
    {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: JSON.stringify({ text, mark_following: true }),
    },
  )
  return toOpportunity(item)
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
  return fetchJson<PlanEntitlements[]>('/api/v1/subscriptions/plans')
}

export async function fetchMySubscription(): Promise<SubscriptionUsage> {
  return fetchJson<SubscriptionUsage>('/api/v1/subscriptions/me')
}

export async function fetchSubscriptionCatalog(): Promise<SubscriptionCatalogPlan[]> {
  return fetchJson<SubscriptionCatalogPlan[]>('/api/v1/subscriptions/catalog')
}

export async function syncMySubscription(): Promise<SubscriptionUsage> {
  return fetchJson<SubscriptionUsage>('/api/v1/subscriptions/sync', { method: 'POST' })
}

export async function fetchSubscriptionManagement(): Promise<SubscriptionManagement> {
  return fetchJson<SubscriptionManagement>('/api/v1/subscriptions/management?client=web')
}

export async function fetchReplyTemplates(): Promise<ReplyTemplate[]> {
  return fetchJson<ReplyTemplate[]>('/api/v1/templates')
}

export async function fetchOAuthAuthorizeUrl(provider: OAuthProvider): Promise<string> {
  const result = await fetchJson<OAuthAuthorizeResponse>(`/api/v1/auth/oauth/${provider}/authorize`)
  return result.authorizationUrl
}

export async function fetchMe(): Promise<AuthUser> {
  return fetchJson<AuthUser>('/api/v1/auth/me')
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
  return fetchJson<TelegramConnectionHealth>('/api/v1/integrations/telegram/health')
}

export async function fetchTelegramConnections(): Promise<TelegramConnection[]> {
  return fetchJson<TelegramConnection[]>('/api/v1/integrations/telegram/connections')
}

export async function startTelegramBotChatConnection(): Promise<TelegramConnectionAttempt> {
  return fetchJson<TelegramConnectionAttempt>('/api/v1/integrations/telegram/connect/bot-chat', {
    method: 'POST',
  })
}

export async function startTelegramBusinessConnection(): Promise<TelegramConnectionAttempt> {
  return fetchJson<TelegramConnectionAttempt>('/api/v1/integrations/telegram/connect/business', {
    method: 'POST',
  })
}

export async function startTelegramMtprotoQrConnection(): Promise<TelegramConnectionAttempt> {
  return fetchJson<TelegramConnectionAttempt>('/api/v1/integrations/telegram/connect/mtproto-qr', {
    method: 'POST',
  })
}

export async function fetchTelegramConnectionAttempt(attemptId: string): Promise<TelegramConnectionAttempt> {
  return fetchJson<TelegramConnectionAttempt>(`/api/v1/integrations/telegram/connect/attempts/${attemptId}`)
}

export async function cancelTelegramConnectionAttempt(attemptId: string): Promise<TelegramConnectionAttempt> {
  return fetchJson<TelegramConnectionAttempt>(`/api/v1/integrations/telegram/connect/attempts/${attemptId}/cancel`, {
    method: 'POST',
  })
}

export async function updateTelegramConnection(
  connectionId: string,
  enabled: boolean,
): Promise<TelegramConnection> {
  return fetchJson<TelegramConnection>(`/api/v1/integrations/telegram/connections/${connectionId}`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
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
  return fetchJson<void>(`/api/v1/integrations/telegram/connections/${connectionId}`, {
    method: 'DELETE',
  })
}

export async function deleteTelegramConnectionSource(sourceId: string): Promise<void> {
  return fetchJson<void>(`/api/v1/integrations/telegram/sources/${sourceId}`, {
    method: 'DELETE',
  })
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
  return fetchJson<TelegramMtprotoDialog[]>(`/api/v1/integrations/telegram/connections/${connectionId}/dialogs`)
}

export async function addTelegramMtprotoSource(
  connectionId: string,
  chatId: string,
): Promise<TelegramConnection> {
  return fetchJson<TelegramConnection>(`/api/v1/integrations/telegram/connections/${connectionId}/sources`, {
    method: 'POST',
    body: JSON.stringify({ chatId }),
  })
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
  return fetchJson<SettingsBundle>('/api/v1/settings/me')
}

export async function updateDetectionSettings(
  body: DetectionSettings,
): Promise<DetectionSettings> {
  return fetchJson<DetectionSettings>('/api/v1/settings/detection', {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}

export async function updateWorkSchedule(
  body: Omit<WorkSchedule, 'isDefault'>,
): Promise<WorkSchedule> {
  return fetchJson<WorkSchedule>('/api/v1/settings/work-schedule', {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}

export async function updateNotificationSettings(
  body: NotificationSettings,
): Promise<NotificationSettings> {
  return fetchJson<NotificationSettings>('/api/v1/settings/notifications', {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}
