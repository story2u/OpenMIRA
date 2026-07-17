import type { DeliveryMode, PreferenceExample } from '@story2u/radar-core/attention/model';
import {
  selectTeachingCards,
  summarizeTeachingExamples,
  type TeachingCandidate,
} from '@story2u/radar-core/attention/learning';

import {
  appendSignalAppetiteEvent,
  readPreferenceExamples,
  readTeachingSession,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from './signalAppetiteStore';

export type TeachingMessageCategory = 'opportunity' | 'job' | 'advertising' | 'discussion' | 'unknown';

export interface TeachingMessageCard {
  messageId: string;
  platform: string;
  conversationName: string;
  conversationKind: 'group' | 'private';
  groupFunctionSummary: string | null;
  senderName: string;
  sentAt: string;
  body: string;
  topics: readonly string[];
  initialDecision: DeliveryMode;
  confidence: number;
  confidenceLevel: 'high' | 'medium' | 'low';
  hasLink: boolean;
  duplicate: boolean;
  category: TeachingMessageCategory;
  piUncertain: boolean;
  sourceKey: string;
  selectionScore: number;
  selectionReasons: readonly string[];
}

interface CandidateRow {
  message_id: string;
  message_payload: string;
  opportunity_payload: string | null;
  current_decision: DeliveryMode | null;
  decision_confidence: number | null;
}

interface MessagePayload {
  id: string;
  opportunityId: string | null;
  senderName: string;
  content: string;
  isFromContact: boolean;
  sentAt: string;
  source: string | null;
}

interface OpportunityPayload {
  id: string;
  platform?: string;
  contactName?: string;
  summary?: string;
  matchedKeywords?: string[];
  confidenceScore?: number;
  sourceType?: string;
  groupName?: string | null;
  detectionReason?: string;
  attentionRequired?: boolean;
}

export interface StartTeachingSessionInput {
  ownerId: string;
  deviceId: string;
  targetCount?: number;
  now?: Date;
  createId?: () => string;
}

export interface CaptureTeachingExampleInput {
  ownerId: string;
  deviceId: string;
  sessionId: string;
  messageId: string;
  label: PreferenceExample['label'];
  reasons?: readonly string[];
  freeformReason?: string | null;
  now?: Date;
  createId?: () => string;
}

function createLocalId() {
  const value = globalThis.crypto?.randomUUID?.();
  if (!value) throw new Error('attention_id_generator_unavailable');
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function readMessagePayload(serialized: string): MessagePayload {
  const value: unknown = JSON.parse(serialized);
  if (
    !isRecord(value)
    || typeof value.id !== 'string'
    || typeof value.senderName !== 'string'
    || typeof value.content !== 'string'
    || typeof value.isFromContact !== 'boolean'
    || typeof value.sentAt !== 'string'
  ) throw new Error('teaching_message_projection_corrupt');
  return value as unknown as MessagePayload;
}

function readOpportunityPayload(serialized: string | null): OpportunityPayload | null {
  if (!serialized) return null;
  const value: unknown = JSON.parse(serialized);
  if (!isRecord(value) || typeof value.id !== 'string') {
    throw new Error('teaching_opportunity_projection_corrupt');
  }
  return value as unknown as OpportunityPayload;
}

function normalizedContent(value: string) {
  return value.trim().toLocaleLowerCase().replace(/\s+/g, ' ');
}

function categoryFor(message: MessagePayload, opportunity: OpportunityPayload | null): TeachingMessageCategory {
  const haystack = [message.content, opportunity?.summary, ...(opportunity?.matchedKeywords ?? [])]
    .filter(Boolean)
    .join(' ')
    .toLocaleLowerCase();
  if (/\b(job|hiring|salary|remote|招聘|职位|薪资|求职)\b/i.test(haystack)) return 'job';
  if (/\b(ad|advert|promotion|course|training|广告|推广|培训|招生)\b/i.test(haystack)) return 'advertising';
  if (opportunity) return 'opportunity';
  if (message.content.length >= 20) return 'discussion';
  return 'unknown';
}

function defaultDecision(
  category: TeachingMessageCategory,
  opportunity: OpportunityPayload | null,
): DeliveryMode {
  if (opportunity?.attentionRequired) return 'immediate';
  if (category === 'opportunity' || category === 'job') return 'inbox';
  if (category === 'advertising') return 'suppress';
  return 'digest';
}

function confidenceLevel(confidence: number): TeachingMessageCard['confidenceLevel'] {
  if (confidence >= 0.78) return 'high';
  if (confidence >= 0.5) return 'medium';
  return 'low';
}

async function nextAggregateVersion(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  aggregateId: string,
) {
  const row = await database.getFirstAsync<{ maximum: number | null }>(
    `SELECT MAX(aggregate_version) AS maximum FROM attention_events
     WHERE owner_id = ? AND aggregate_id = ?`,
    ownerId,
    aggregateId,
  );
  return (row?.maximum ?? 0) + 1;
}

export async function loadTeachingCandidates(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  options: { limit?: number } = {},
): Promise<Array<TeachingMessageCard & TeachingCandidate>> {
  const limit = Math.min(500, Math.max(20, options.limit ?? 250));
  const sync = await database.getFirstAsync<{ phase: string }>(
    `SELECT phase FROM sync_state WHERE owner_id = ? AND stream = 'business'`,
    ownerId,
  );
  if (!sync || sync.phase !== 'ready') throw new Error('teaching_projection_not_ready');
  const rows = await database.getAllAsync<CandidateRow>(
    `SELECT
      message.id AS message_id,
      message.payload_json AS message_payload,
      opportunity.payload_json AS opportunity_payload,
      decision.decision AS current_decision,
      decision.confidence AS decision_confidence
     FROM message_projection AS message
     LEFT JOIN opportunity_projection AS opportunity
       ON opportunity.owner_id = message.owner_id
      AND opportunity.id = message.opportunity_id
      AND opportunity.deleted_at IS NULL
     LEFT JOIN message_filter_decisions AS decision
       ON decision.owner_id = message.owner_id AND decision.message_id = message.id
     WHERE message.owner_id = ? AND message.deleted_at IS NULL
       AND CAST(json_extract(message.payload_json, '$.isFromContact') AS INTEGER) = 1
       AND length(trim(CAST(json_extract(message.payload_json, '$.content') AS TEXT))) > 0
     ORDER BY message.sent_at DESC, message.id DESC LIMIT ?`,
    ownerId,
    limit,
  );
  const decoded = rows.map((row) => ({
    row,
    message: readMessagePayload(row.message_payload),
    opportunity: readOpportunityPayload(row.opportunity_payload),
  }));
  const duplicateCounts = new Map<string, number>();
  decoded.forEach(({ message }) => {
    const content = normalizedContent(message.content);
    duplicateCounts.set(content, (duplicateCounts.get(content) ?? 0) + 1);
  });

  return decoded.map(({ row, message, opportunity }) => {
    const category = categoryFor(message, opportunity);
    const duplicate = (duplicateCounts.get(normalizedContent(message.content)) ?? 0) > 1;
    const initialDecision = row.current_decision ?? defaultDecision(category, opportunity);
    const confidence = Math.min(1, Math.max(0, row.decision_confidence
      ?? opportunity?.confidenceScore
      ?? (category === 'unknown' ? 0.42 : 0.66)));
    const topics = (opportunity?.matchedKeywords ?? []).slice(0, 6);
    const conversationName = opportunity?.groupName
      || opportunity?.contactName
      || message.senderName;
    const sourceKey = `${opportunity?.platform ?? 'unknown'}:${conversationName}`;
    const candidateDecision = category === 'advertising'
      ? 'suppress'
      : category === 'opportunity' || category === 'job'
        ? 'inbox'
        : 'digest';
    return {
      messageId: message.id,
      platform: opportunity?.platform ?? 'unknown',
      conversationName,
      conversationKind: opportunity?.sourceType === 'group' ? 'group' : 'private',
      groupFunctionSummary: opportunity?.sourceType === 'group'
        ? opportunity.summary?.slice(0, 160) ?? null
        : null,
      senderName: message.senderName,
      sentAt: message.sentAt,
      body: message.content,
      topics,
      initialDecision,
      confidence,
      confidenceLevel: confidenceLevel(confidence),
      hasLink: /https?:\/\//i.test(message.content),
      duplicate,
      category,
      piUncertain: confidence >= 0.35 && confidence <= 0.68,
      sourceKey,
      selectionScore: 0,
      selectionReasons: [],
      topicKeys: topics.length > 0 ? topics : [category],
      currentDecision: initialDecision,
      candidateDecision,
      openedRecently: false,
      ignoredRecently: false,
      likelyNoise: category === 'advertising',
      highValueDomain: category === 'opportunity' || category === 'job',
      allowedForTeaching: true,
      sensitive: false,
    };
  });
}

export async function startTeachingSession(
  database: SignalAppetiteStoreDatabase,
  input: StartTeachingSessionInput,
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const sessionId = createId();
  const targetCount = Math.min(15, Math.max(5, input.targetCount ?? 8));
  const candidates = await loadTeachingCandidates(database, input.ownerId);
  const selected = selectTeachingCards(candidates, { targetCount });
  await appendSignalAppetiteEvent(database, {
    eventId: createId(),
    ownerId: input.ownerId,
    deviceId: input.deviceId,
    aggregateId: sessionId,
    aggregateVersion: 1,
    schemaVersion: 1,
    occurredAt: now.toISOString(),
    type: 'TeachingSessionStarted',
    payload: { sessionId, targetCount },
  });
  for (const [index, card] of selected.entries()) {
    await appendSignalAppetiteEvent(database, {
      eventId: createId(),
      ownerId: input.ownerId,
      deviceId: input.deviceId,
      aggregateId: sessionId,
      aggregateVersion: index + 2,
      schemaVersion: 1,
      occurredAt: now.toISOString(),
      type: 'TeachingCardPresented',
      payload: {
        sessionId,
        messageId: card.messageId,
        selectionScore: card.selectionScore,
      },
    });
  }
  return {
    sessionId,
    cards: selected.map((selectedCard) => {
      const card = candidates.find((candidate) => candidate.messageId === selectedCard.messageId);
      if (!card) throw new Error('teaching_card_missing');
      return {
        ...card,
        selectionScore: selectedCard.selectionScore,
        selectionReasons: selectedCard.selectionReasons,
      } satisfies TeachingMessageCard;
    }),
  };
}

export async function captureTeachingExample(
  database: SignalAppetiteStoreDatabase,
  input: CaptureTeachingExampleInput,
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const session = await readTeachingSession(database, input.ownerId, input.sessionId);
  if (!session || session.status !== 'active') throw new Error('teaching_session_not_active');
  const example: PreferenceExample = {
    id: createId(),
    messageId: input.messageId,
    label: input.label,
    selectedReasons: [...new Set(input.reasons ?? [])].slice(0, 8),
    freeformReason: input.freeformReason?.trim().slice(0, 1_000) || null,
    capturedAt: now.toISOString(),
    teachingSessionId: input.sessionId,
    revertedAt: null,
  };
  const aggregateVersion = await nextAggregateVersion(database, input.ownerId, input.sessionId);
  await appendSignalAppetiteEvent(database, {
    eventId: createId(),
    ownerId: input.ownerId,
    deviceId: input.deviceId,
    aggregateId: input.sessionId,
    aggregateVersion,
    schemaVersion: 1,
    occurredAt: now.toISOString(),
    type: 'PreferenceExampleCaptured',
    payload: { example },
  });
  return example;
}

export async function undoTeachingExamples(
  database: SignalAppetiteStoreDatabase,
  input: {
    ownerId: string;
    deviceId: string;
    sessionId: string;
    count?: number;
    now?: Date;
    createId?: () => string;
  },
) {
  const count = Math.min(10, Math.max(1, input.count ?? 1));
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const examples = (await readPreferenceExamples(database, input.ownerId, input.sessionId))
    .filter((example) => !example.revertedAt)
    .slice(-count)
    .reverse();
  let version = await nextAggregateVersion(database, input.ownerId, input.sessionId);
  for (const example of examples) {
    await appendSignalAppetiteEvent(database, {
      eventId: createId(),
      ownerId: input.ownerId,
      deviceId: input.deviceId,
      aggregateId: input.sessionId,
      aggregateVersion: version,
      schemaVersion: 1,
      occurredAt: now.toISOString(),
      type: 'PreferenceExampleReverted',
      payload: { exampleId: example.id, revertedAt: now.toISOString() },
    });
    version += 1;
  }
  return examples.map((example) => example.id);
}

export async function annotateTeachingExample(
  database: SignalAppetiteStoreDatabase,
  input: {
    ownerId: string;
    deviceId: string;
    sessionId: string;
    exampleId: string;
    reasons: readonly string[];
    freeformReason?: string | null;
    now?: Date;
    createId?: () => string;
  },
) {
  const examples = await readPreferenceExamples(database, input.ownerId, input.sessionId);
  const existing = examples.find((example) => example.id === input.exampleId);
  if (!existing || existing.revertedAt) throw new Error('teaching_example_not_active');
  const updated: PreferenceExample = {
    ...existing,
    selectedReasons: [...new Set(input.reasons)].slice(0, 8),
    freeformReason: input.freeformReason?.trim().slice(0, 1_000) || null,
  };
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  await appendSignalAppetiteEvent(database, {
    eventId: createId(),
    ownerId: input.ownerId,
    deviceId: input.deviceId,
    aggregateId: input.sessionId,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, input.sessionId),
    schemaVersion: 1,
    occurredAt: now.toISOString(),
    type: 'PreferenceExampleCaptured',
    payload: { example: updated },
  });
  return updated;
}

export async function completeTeachingSession(
  database: SignalAppetiteStoreDatabase,
  input: {
    ownerId: string;
    deviceId: string;
    sessionId: string;
    reasonLabels?: Readonly<Record<string, string>>;
    now?: Date;
    createId?: () => string;
  },
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const session = await readTeachingSession(database, input.ownerId, input.sessionId);
  if (!session || session.status !== 'active') throw new Error('teaching_session_not_active');
  const examples = await readPreferenceExamples(database, input.ownerId, input.sessionId);
  const summary = summarizeTeachingExamples(examples, input.reasonLabels);
  await appendSignalAppetiteEvent(database, {
    eventId: createId(),
    ownerId: input.ownerId,
    deviceId: input.deviceId,
    aggregateId: input.sessionId,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, input.sessionId),
    schemaVersion: 1,
    occurredAt: now.toISOString(),
    type: 'TeachingSessionCompleted',
    payload: {
      sessionId: input.sessionId,
      completedAt: now.toISOString(),
      summary: { increase: summary.increase, reduce: summary.reduce },
    },
  });
  return summary;
}
