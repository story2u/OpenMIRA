import type {
  Dashboard,
  DashboardQuery,
  Opportunity,
  OpportunityDetail,
  OpportunityListQuery,
} from '@story2u/radar-contracts/opportunities';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

export type DashboardSort = 'confidence' | 'newest' | 'oldest' | 'trust';
export type DashboardTrustLevel = 'risky' | 'suspicious' | 'trusted' | 'unverified';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';

const nullableString = Type.Union([Type.String(), Type.Null()]);
const nullableDateTime = Type.Union([
  Type.String({ pattern: dateTimePattern }),
  Type.Null(),
]);
const jsonObject = Type.Record(Type.String(), Type.Unknown(), { maxProperties: 64 });

const AgentActionSchema = Type.Object(
  {
    actionType: Type.Union([
      Type.Literal('send_email'),
      Type.Literal('add_friend'),
      Type.Literal('private_message'),
      Type.Literal('notify_user'),
    ]),
    reason: Type.String({ maxLength: 10_000 }),
    target: Type.Optional(nullableString),
    draft: Type.Optional(nullableString),
    requiresApproval: Type.Boolean(),
  },
  { additionalProperties: false },
);

const opportunityProperties = {
    id: Type.String({ pattern: uuidPattern }),
    platform: Type.Union([Type.Literal('telegram'), Type.Literal('wecom')]),
    contactName: Type.String(),
    contactAvatar: Type.String(),
    summary: Type.String(),
    matchedKeywords: Type.Array(Type.String(), { maxItems: 64 }),
    confidenceScore: Type.Number({ minimum: 0, maximum: 1 }),
    status: Type.Union([
      Type.Literal('pending'),
      Type.Literal('replied'),
      Type.Literal('ignored'),
    ]),
    internalStatus: Type.Union([
      Type.Literal('pending_human'),
      Type.Literal('ai_auto_reply'),
      Type.Literal('replied'),
      Type.Literal('following'),
      Type.Literal('ignored'),
      Type.Literal('closed'),
    ]),
    priority: Type.Union([
      Type.Literal('low'),
      Type.Literal('normal'),
      Type.Literal('high'),
      Type.Literal('urgent'),
    ]),
    lastMessagePreview: Type.String(),
    createdAt: Type.String({ pattern: dateTimePattern }),
    updatedAt: Type.String({ pattern: dateTimePattern }),
    sourceType: Type.Union([Type.Literal('group'), Type.Literal('private')]),
    groupName: Type.Optional(nullableString),
    groupMemberRole: Type.String(),
    rawMessageLinks: Type.Optional(Type.Array(Type.String(), { maxItems: 64 })),
    linkVerification: Type.Optional(jsonObject),
    extractedContacts: Type.Optional(jsonObject),
    friendRequestStatus: Type.String(),
    sopStage: Type.String(),
    trustScore: Type.Integer({ minimum: 0, maximum: 100 }),
    agentActions: Type.Optional(Type.Array(AgentActionSchema, { maxItems: 64 })),
    agentAnalysisStatus: Type.Union([
      Type.Literal('not_requested'),
      Type.Literal('quota_exceeded'),
      Type.Literal('queued'),
      Type.Literal('running'),
      Type.Literal('completed'),
      Type.Literal('failed'),
    ]),
    agentAnalysisError: Type.Optional(nullableString),
    agentAnalyzedAt: Type.Optional(nullableDateTime),
    attentionRequired: Type.Boolean(),
    archivedAt: Type.Optional(nullableDateTime),
    archivedByUserId: Type.Optional(Type.Union([
      Type.String({ pattern: uuidPattern }),
      Type.Null(),
    ])),
    archiveReason: Type.Optional(nullableString),
};

export const OpportunitySchema = Type.Object(
  opportunityProperties,
  { additionalProperties: false },
);

export const OpportunityDetailSchema = Type.Object(
  {
    ...opportunityProperties,
    aiReplyDraft: Type.Optional(nullableString),
    finalReply: Type.Optional(nullableString),
    detectionReason: Type.Optional(nullableString),
    assignedTo: Type.Optional(nullableString),
  },
  { additionalProperties: false },
);

export const DashboardSchema = Type.Object(
  {
    items: Type.Array(OpportunitySchema, { maxItems: 100 }),
    total: Type.Integer({ minimum: 0 }),
    limit: Type.Integer({ minimum: 1, maximum: 100 }),
    offset: Type.Integer({ minimum: 0 }),
    pendingCount: Type.Integer({ minimum: 0 }),
    attentionItems: Type.Optional(Type.Array(OpportunitySchema, { maxItems: 100 })),
    keywordOptions: Type.Optional(Type.Array(Type.String(), { maxItems: 256 })),
  },
  { additionalProperties: false },
);

const parseOpportunity = typeboxDecoder(OpportunitySchema);
const parseOpportunityDetail = typeboxDecoder(OpportunityDetailSchema);
const parseOpportunityList = typeboxDecoder(Type.Array(OpportunitySchema));
const parseDashboard = typeboxDecoder(DashboardSchema);

function assertUniqueOpportunityIds(items: readonly { id: string }[], label: string) {
  const ids = new Set(items.map((item) => item.id));
  if (ids.size !== items.length) throw new Error(`${label} contains duplicate opportunity ids`);
}

export const decodeOpportunityResponse: ResponseDecoder<Opportunity> = (value) =>
  parseOpportunity(value) as Opportunity;

export const decodeOpportunityDetailResponse: ResponseDecoder<OpportunityDetail> = (value) =>
  parseOpportunityDetail(value) as OpportunityDetail;

export const decodeOpportunityListResponse: ResponseDecoder<Opportunity[]> = (value) => {
  const parsed = parseOpportunityList(value);
  assertUniqueOpportunityIds(parsed, 'Opportunity response');
  return parsed as Opportunity[];
};

export const decodeDashboardResponse: ResponseDecoder<Dashboard> = (value) => {
  const parsed = parseDashboard(value);
  if (parsed.items.length > parsed.limit || parsed.items.length > parsed.total) {
    throw new Error('Dashboard pagination metadata is inconsistent');
  }
  assertUniqueOpportunityIds(parsed.items, 'Dashboard page');
  assertUniqueOpportunityIds(parsed.attentionItems ?? [], 'Dashboard attention list');
  return parsed as Dashboard;
};

const dashboardStatuses = new Set(['pending', 'replied', 'ignored']);
const platforms = new Set(['telegram', 'wecom']);
const sourceTypes = new Set(['group', 'private']);
const dashboardSorts = new Set(['confidence', 'newest', 'oldest', 'trust']);
const trustLevels = new Set(['risky', 'suspicious', 'trusted', 'unverified']);
const sopStages = new Set([
  'detected',
  'analyzing',
  'verified',
  'contact_extracted',
  'friend_requested',
  'ready_to_chat',
  'chatting',
  'closed',
]);
const archiveScopes = new Set(['active', 'archived', 'all']);

function requireEnum(value: string | null | undefined, allowed: ReadonlySet<string>, label: string) {
  if (value !== undefined && value !== null && !allowed.has(value)) {
    throw new Error(`Invalid ${label}`);
  }
}

function requireInteger(value: number | undefined, minimum: number, maximum: number, label: string) {
  if (value !== undefined && (!Number.isInteger(value) || value < minimum || value > maximum)) {
    throw new Error(`Invalid ${label}`);
  }
}

function requireDateTime(value: string | null | undefined, label: string) {
  if (value !== undefined && value !== null && !Number.isFinite(Date.parse(value))) {
    throw new Error(`Invalid ${label}`);
  }
}

function requireUuid(value: string, label: string) {
  if (!new RegExp(uuidPattern).test(value)) throw new Error(`Invalid ${label}`);
}

function append(pairs: string[], name: string, value: string | number | null | undefined) {
  if (value === undefined || value === null) return;
  pairs.push(`${encodeURIComponent(name)}=${encodeURIComponent(String(value))}`);
}

function appendList(
  pairs: string[],
  name: string,
  values: string[] | null | undefined,
  allowed?: ReadonlySet<string>,
) {
  if (!values) return;
  if (values.length > 64) throw new Error(`Too many ${name} values`);
  const normalized = Array.from(new Set(values.map((value) => value.trim()).filter(Boolean))).sort();
  for (const value of normalized) {
    if (value.length > 128 || (allowed && !allowed.has(value))) throw new Error(`Invalid ${name}`);
    append(pairs, name, value);
  }
}

function withQuery(path: string, pairs: string[]) {
  return pairs.length > 0 ? `${path}?${pairs.join('&')}` : path;
}

export function dashboardRequestPath(query: DashboardQuery = {}) {
  requireEnum(query.status, dashboardStatuses, 'dashboard status');
  requireEnum(query.platform, platforms, 'dashboard platform');
  requireEnum(query.source_type, sourceTypes, 'dashboard source type');
  requireEnum(query.sort, dashboardSorts, 'dashboard sort');
  requireDateTime(query.created_from, 'dashboard created_from');
  requireDateTime(query.created_to, 'dashboard created_to');
  if (
    query.created_from &&
    query.created_to &&
    Date.parse(query.created_from) > Date.parse(query.created_to)
  ) {
    throw new Error('dashboard created_from must not be after created_to');
  }
  requireInteger(query.limit, 1, 100, 'dashboard limit');
  requireInteger(query.offset, 0, Number.MAX_SAFE_INTEGER, 'dashboard offset');

  const pairs: string[] = [];
  append(pairs, 'status', query.status);
  append(pairs, 'platform', query.platform);
  append(pairs, 'source_type', query.source_type);
  append(pairs, 'created_from', query.created_from);
  append(pairs, 'created_to', query.created_to);
  appendList(pairs, 'trust_levels', query.trust_levels, trustLevels);
  appendList(pairs, 'sop_stages', query.sop_stages, sopStages);
  appendList(pairs, 'keywords', query.keywords);
  append(pairs, 'sort', query.sort);
  append(pairs, 'limit', query.limit);
  append(pairs, 'offset', query.offset);
  return withQuery('/api/v1/opportunities/dashboard', pairs);
}

export function opportunityListRequestPath(query: OpportunityListQuery = {}) {
  requireEnum(query.status, dashboardStatuses, 'opportunity status');
  requireEnum(query.platform, platforms, 'opportunity platform');
  requireEnum(query.archive, archiveScopes, 'opportunity archive scope');
  requireInteger(query.limit, 1, 200, 'opportunity limit');
  requireInteger(query.offset, 0, Number.MAX_SAFE_INTEGER, 'opportunity offset');

  const pairs: string[] = [];
  append(pairs, 'status', query.status);
  append(pairs, 'platform', query.platform);
  append(pairs, 'archive', query.archive);
  append(pairs, 'limit', query.limit);
  append(pairs, 'offset', query.offset);
  return withQuery('/api/v1/opportunities', pairs);
}

export function opportunityDetailRequestPath(opportunityId: string) {
  requireUuid(opportunityId, 'opportunity id');
  return `/api/v1/opportunities/${encodeURIComponent(opportunityId)}`;
}

export function createOpportunitiesApi(client: RadarApiClient) {
  return {
    getDashboard(query: DashboardQuery = {}, init: Pick<RequestInit, 'signal'> = {}): Promise<Dashboard> {
      return client.request(dashboardRequestPath(query), {
        ...init,
        decode: decodeDashboardResponse,
      });
    },

    list(query: OpportunityListQuery = {}, init: Pick<RequestInit, 'signal'> = {}): Promise<Opportunity[]> {
      return client.request(opportunityListRequestPath(query), {
        ...init,
        decode: decodeOpportunityListResponse,
      });
    },

    getById(opportunityId: string, init: Pick<RequestInit, 'signal'> = {}): Promise<OpportunityDetail> {
      return client.request(opportunityDetailRequestPath(opportunityId), {
        ...init,
        decode: decodeOpportunityDetailResponse,
      });
    },
  };
}
