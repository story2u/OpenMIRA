import type {
  AuthUser,
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
  TelegramDialog,
  TelegramConnection,
  TelegramConnectionAttempt,
  TelegramConnectionHealth,
  TelegramUserConfig,
  TelegramUserConfigUpdate,
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
  }
}

export async function fetchOpportunities(): Promise<Opportunity[]> {
  const items = await fetchJson<ApiOpportunity[]>('/api/v1/opportunities?limit=200')
  return items.map(toOpportunity)
}

export async function fetchOpportunity(opportunityId: string): Promise<Opportunity> {
  return toOpportunity(await fetchJson<ApiOpportunity>(`/api/v1/opportunities/${opportunityId}`))
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

export async function updateTelegramMonitorRetention(
  monitorIds: string[],
): Promise<TelegramUserConfig> {
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
