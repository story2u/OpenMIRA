import {
  evaluateMessage,
  simulateAppetite,
  type AppetiteMessage,
} from '@story2u/radar-core/attention/evaluator';
import type {
  AppetiteSimulationSummary,
  AttentionIntent,
  AttentionPreference,
  AttentionScheduleWindow,
  MessageFilterDecision,
  TemporaryFocus,
} from '@story2u/radar-core/attention/model';

import {
  appendSignalAppetiteEvent,
  readActivePreference,
  readLatestPreferenceVersion,
  readMessageFilterDecisions,
  readPreferenceIntents,
  readPreferenceVersion,
  readSignalAppetiteEvents,
  readTeachingSession,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from './signalAppetiteStore';
import { loadTeachingCandidates } from './teachingService';

interface ServiceContext {
  ownerId: string;
  deviceId: string;
  now?: Date;
  createId?: () => string;
}

const reasonConcepts: Readonly<Record<string, {
  concept: string;
  intentType: AttentionIntent['intentType'];
  deliveryMode: AttentionIntent['deliveryMode'];
}>> = {
  important_customer: { concept: 'important_customer', intentType: 'include', deliveryMode: 'immediate' },
  purchase_intent: { concept: 'purchase_intent', intentType: 'include', deliveryMode: 'immediate' },
  needs_reply: { concept: 'needs_reply', intentType: 'include', deliveryMode: 'immediate' },
  suitable_job: { concept: 'suitable_job', intentType: 'include', deliveryMode: 'inbox' },
  current_project: { concept: 'current_project', intentType: 'include', deliveryMode: 'inbox' },
  deadline: { concept: 'deadline', intentType: 'include', deliveryMode: 'immediate' },
  industry_signal: { concept: 'industry_signal', intentType: 'include', deliveryMode: 'digest' },
  advertising: { concept: 'advertising', intentType: 'reduce', deliveryMode: 'suppress' },
  training: { concept: 'training', intentType: 'reduce', deliveryMode: 'suppress' },
  duplicate: { concept: 'duplicate', intentType: 'reduce', deliveryMode: 'suppress' },
  unrelated_chat: { concept: 'unrelated_chat', intentType: 'reduce', deliveryMode: 'digest' },
  expired: { concept: 'expired', intentType: 'reduce', deliveryMode: 'suppress' },
  untrusted_source: { concept: 'untrusted_source', intentType: 'reduce', deliveryMode: 'digest' },
};

const defaultSchedule: readonly AttentionScheduleWindow[] = [
  {
    id: 'workday',
    days: [1, 2, 3, 4, 5],
    startMinute: 9 * 60,
    endMinute: 18 * 60,
    label: 'work_mode',
    activeIntentIds: [],
    fallbackDeliveryMode: 'digest',
  },
  {
    id: 'evening',
    days: [0, 1, 2, 3, 4, 5, 6],
    startMinute: 18 * 60,
    endMinute: 21 * 60,
    label: 'interest_mode',
    activeIntentIds: [],
    fallbackDeliveryMode: 'inbox',
  },
];

function createLocalId() {
  const value = globalThis.crypto?.randomUUID?.();
  if (!value) throw new Error('attention_id_generator_unavailable');
  return value;
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

function messageFromCard(card: Awaited<ReturnType<typeof loadTeachingCandidates>>[number]): AppetiteMessage {
  const semanticScores = Object.fromEntries(card.topicKeys.map((topic) => [topic, 0.86]));
  return {
    id: card.messageId,
    sourceKey: card.sourceKey,
    sourceTrusted: true,
    senderImportant: false,
    duplicate: card.duplicate,
    explicitAdvertisingSource: card.category === 'advertising',
    hardExcluded: false,
    needsReply: card.initialDecision === 'immediate',
    topicKeys: card.topicKeys,
    semanticScores,
    sentAt: card.sentAt,
  };
}

function baseEvaluation(
  message: AppetiteMessage,
  preferenceVersion: number,
  intents: readonly AttentionIntent[],
  schedule: readonly AttentionScheduleWindow[],
  now: string,
) {
  return evaluateMessage({
    message,
    preferenceVersion,
    intents,
    schedule,
    temporaryFocuses: [],
    cloudAvailable: false,
    now,
  });
}

export async function proposeAppetiteFromTeaching(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { sessionId: string },
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const session = await readTeachingSession(database, input.ownerId, input.sessionId);
  if (!session || session.status !== 'summarized' || !session.summary) {
    throw new Error('teaching_session_not_summarized');
  }
  const active = await readActivePreference(database, input.ownerId);
  const version = (await readLatestPreferenceVersion(database, input.ownerId)) + 1;
  const preferenceId = active?.id ?? createId();
  const concepts = [
    ...session.summary.increase.map((reason) => ({ reason, direction: 'increase' as const })),
    ...session.summary.reduce.map((reason) => ({ reason, direction: 'reduce' as const })),
  ];
  const intents: AttentionIntent[] = concepts.map(({ reason, direction }, index) => {
    const mapped = reasonConcepts[reason] ?? {
      concept: reason,
      intentType: direction === 'increase' ? 'include' as const : 'reduce' as const,
      deliveryMode: direction === 'increase' ? 'inbox' as const : 'digest' as const,
    };
    return {
      id: createId(),
      preferenceId,
      concept: mapped.concept,
      intentType: mapped.intentType,
      weight: mapped.intentType === 'reduce' ? -Math.max(0.55, 0.82 - index * 0.04) : Math.max(0.55, 0.82 - index * 0.04),
      deliveryMode: mapped.deliveryMode,
      confidence: 0.72,
      userConfirmed: false,
      source: 'teaching',
      validFrom: null,
      validUntil: null,
    };
  });
  const schedule = (active?.schedule.length ? active.schedule : defaultSchedule).map((window) => ({
    ...window,
    activeIntentIds: intents.filter((intent) => intent.intentType !== 'reduce').map((intent) => intent.id),
  }));
  const preference: AttentionPreference = {
    id: preferenceId,
    title: 'Signal Appetite',
    naturalLanguageSummary: [
      session.summary.increase.length ? `Focus on ${session.summary.increase.join(', ')}` : '',
      session.summary.reduce.length ? `Reduce ${session.summary.reduce.join(', ')}` : '',
    ].filter(Boolean).join('. ') || 'Keep the current appetite while Pi learns more.',
    scope: 'all_messages',
    status: 'candidate',
    confidence: intents.length > 0 ? 0.72 : 0.45,
    version,
    activeFrom: null,
    activeUntil: null,
    schedule,
    createdAt: now.toISOString(),
    updatedAt: now.toISOString(),
  };
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: preferenceId,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, preferenceId),
    schemaVersion: 1, occurredAt: now.toISOString(), type: 'PreferenceChangeProposed',
    payload: { preference, intents, teachingSessionId: input.sessionId },
  });
  return { preference, intents };
}

export async function simulatePreferenceVersion(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { version: number },
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const preference = await readPreferenceVersion(database, input.ownerId, input.version);
  if (!preference || preference.status !== 'candidate') throw new Error('candidate_preference_not_found');
  const intents = await readPreferenceIntents(database, input.ownerId, preference.id, preference.version);
  const cards = await loadTeachingCandidates(database, input.ownerId, { limit: 500 });
  const messages = cards.map(messageFromCard);
  const previous = new Map((await readMessageFilterDecisions(database, input.ownerId, { limit: 500 }))
    .map((item) => [item.messageId, item]));
  const summary = simulateAppetite(
    messages,
    (message) => baseEvaluation(message, preference.version, intents, preference.schedule, now.toISOString()),
    previous,
  );
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: preference.id,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, preference.id),
    schemaVersion: 1, occurredAt: now.toISOString(), type: 'PreferenceSimulationCompleted',
    payload: { preferenceId: preference.id, candidateVersion: preference.version, summary },
  });
  return summary;
}

async function requireSimulation(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  version: number,
): Promise<AppetiteSimulationSummary> {
  const events = await readSignalAppetiteEvents(database, ownerId, { limit: 1_000 });
  const event = events.findLast((item) => (
    item.type === 'PreferenceSimulationCompleted' && item.payload.candidateVersion === version
  ));
  if (!event || event.type !== 'PreferenceSimulationCompleted') throw new Error('preference_not_simulated');
  return event.payload.summary;
}

export async function applyPreferenceVersion(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { version: number; confirmed: boolean },
) {
  if (!input.confirmed) throw new Error('preference_confirmation_required');
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const preference = await readPreferenceVersion(database, input.ownerId, input.version);
  if (!preference || preference.status !== 'candidate') throw new Error('candidate_preference_not_found');
  await requireSimulation(database, input.ownerId, input.version);
  const intents = await readPreferenceIntents(database, input.ownerId, preference.id, preference.version);
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: preference.id,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, preference.id),
    schemaVersion: 1, occurredAt: now.toISOString(), type: 'PreferenceApplied',
    payload: { preferenceId: preference.id, version: preference.version, appliedAt: now.toISOString() },
  });
  const cards = await loadTeachingCandidates(database, input.ownerId, { limit: 500 });
  const decisions: MessageFilterDecision[] = [];
  for (const card of cards) {
    const filterDecision = baseEvaluation(
      messageFromCard(card), preference.version, intents, preference.schedule, now.toISOString(),
    );
    decisions.push(filterDecision);
    await appendSignalAppetiteEvent(database, {
      eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
      aggregateId: card.messageId,
      aggregateVersion: await nextAggregateVersion(database, input.ownerId, card.messageId),
      schemaVersion: 1, occurredAt: now.toISOString(), type: 'MessageFilterDecisionMade',
      payload: { decision: filterDecision },
    });
  }
  return { preference: await readPreferenceVersion(database, input.ownerId, input.version), decisions };
}

export async function startPreferenceShadow(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { candidateVersion: number; durationHours?: number },
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const active = await readActivePreference(database, input.ownerId);
  const candidate = await readPreferenceVersion(database, input.ownerId, input.candidateVersion);
  if (!active || !candidate || candidate.status !== 'candidate') throw new Error('shadow_versions_not_found');
  const summary = await requireSimulation(database, input.ownerId, input.candidateVersion);
  const durationHours = Math.min(72, Math.max(1, input.durationHours ?? 24));
  const shadow = {
    id: createId(),
    oldVersion: active.version,
    candidateVersion: candidate.version,
    startedAt: now.toISOString(),
    endsAt: new Date(now.getTime() + durationHours * 60 * 60 * 1_000).toISOString(),
    diffSummary: summary,
    status: 'running' as const,
  };
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: shadow.id, aggregateVersion: 1, schemaVersion: 1,
    occurredAt: now.toISOString(), type: 'PreferenceShadowStarted', payload: { shadow },
  });
  return shadow;
}

export async function createTemporaryFocus(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { concept: string; durationHours: number; deliveryMode?: TemporaryFocus['deliveryMode'] },
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const durationHours = Math.min(24 * 30, Math.max(1, input.durationHours));
  const focus: TemporaryFocus = {
    id: createId(),
    concept: input.concept.trim().slice(0, 120),
    deliveryMode: input.deliveryMode ?? 'immediate',
    createdAt: now.toISOString(),
    expiresAt: new Date(now.getTime() + durationHours * 60 * 60 * 1_000).toISOString(),
    expiredAt: null,
  };
  if (!focus.concept) throw new Error('temporary_focus_concept_required');
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: focus.id, aggregateVersion: 1, schemaVersion: 1,
    occurredAt: now.toISOString(), type: 'TemporaryFocusCreated', payload: { focus },
  });
  return focus;
}

export async function undoPreferenceChange(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext,
) {
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const active = await readActivePreference(database, input.ownerId);
  if (!active) throw new Error('active_preference_not_found');
  const row = await database.getFirstAsync<{ version: number }>(
    `SELECT version FROM attention_preferences
     WHERE owner_id = ? AND id = ? AND version < ? AND status IN ('superseded', 'reverted')
     ORDER BY version DESC LIMIT 1`,
    input.ownerId,
    active.id,
    active.version,
  );
  if (!row) throw new Error('previous_preference_not_found');
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: active.id,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, active.id),
    schemaVersion: 1, occurredAt: now.toISOString(), type: 'PreferenceReverted',
    payload: {
      preferenceId: active.id,
      fromVersion: active.version,
      toVersion: row.version,
      revertedAt: now.toISOString(),
    },
  });
  return readActivePreference(database, input.ownerId);
}

function parseScheduleMinute(instruction: string) {
  const normalized = instruction.toLocaleLowerCase();
  const numeric = normalized.match(/\b(\d{1,2})(?::(\d{2}))?\b/);
  const chineseHours: Readonly<Record<string, number>> = {
    一: 1, 二: 2, 三: 3, 四: 4, 五: 5, 六: 6, 七: 7, 八: 8, 九: 9,
    十: 10, 十一: 11, 十二: 12,
  };
  const chinese = normalized.match(/([一二三四五六七八九十]{1,3})点(半)?/);
  let hour = numeric ? Number(numeric[1]) : chinese ? chineseHours[chinese[1] ?? ''] : Number.NaN;
  const minute = numeric?.[2] ? Number(numeric[2]) : chinese?.[2] ? 30 : 0;
  if (!Number.isFinite(hour) || hour > 23 || minute > 59) return null;
  if ((/晚|evening|pm/.test(normalized)) && hour < 12) hour += 12;
  return hour * 60 + minute;
}

export async function proposeScheduleUpdate(
  database: SignalAppetiteStoreDatabase,
  input: ServiceContext & { instruction: string },
) {
  const instruction = input.instruction.trim();
  const minute = parseScheduleMinute(instruction);
  if (!instruction || minute === null) {
    return { state: 'clarification_required' as const, question: 'What time should this start?' };
  }
  const active = await readActivePreference(database, input.ownerId);
  if (!active) throw new Error('active_preference_not_found');
  const now = input.now ?? new Date();
  const createId = input.createId ?? createLocalId;
  const version = (await readLatestPreferenceVersion(database, input.ownerId)) + 1;
  const previousIntents = await readPreferenceIntents(database, input.ownerId, active.id, active.version);
  const schedule = active.schedule.map((window, index) => (
    index === active.schedule.length - 1 ? { ...window, startMinute: minute } : window
  ));
  const preference: AttentionPreference = {
    ...active,
    status: 'candidate',
    version,
    naturalLanguageSummary: instruction,
    activeFrom: null,
    createdAt: now.toISOString(),
    updatedAt: now.toISOString(),
    schedule,
  };
  const intents = previousIntents.map((intent) => ({ ...intent, userConfirmed: false }));
  await appendSignalAppetiteEvent(database, {
    eventId: createId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: active.id,
    aggregateVersion: await nextAggregateVersion(database, input.ownerId, active.id),
    schemaVersion: 1, occurredAt: now.toISOString(), type: 'PreferenceChangeProposed',
    payload: { preference, intents, teachingSessionId: null },
  });
  return { state: 'candidate' as const, preference, intents };
}

export async function comparePreferenceVersions(
  database: SignalAppetiteStoreExecutor,
  input: { ownerId: string; fromVersion: number; toVersion: number },
) {
  const [from, to] = await Promise.all([
    readPreferenceVersion(database, input.ownerId, input.fromVersion),
    readPreferenceVersion(database, input.ownerId, input.toVersion),
  ]);
  if (!from || !to || from.id !== to.id) throw new Error('preference_versions_not_found');
  const [fromIntents, toIntents] = await Promise.all([
    readPreferenceIntents(database, input.ownerId, from.id, from.version),
    readPreferenceIntents(database, input.ownerId, to.id, to.version),
  ]);
  const fromConcepts = new Set(fromIntents.map((intent) => intent.concept));
  const toConcepts = new Set(toIntents.map((intent) => intent.concept));
  return {
    from: { preference: from, intents: fromIntents },
    to: { preference: to, intents: toIntents },
    addedConcepts: [...toConcepts].filter((concept) => !fromConcepts.has(concept)),
    removedConcepts: [...fromConcepts].filter((concept) => !toConcepts.has(concept)),
    scheduleChanged: JSON.stringify(from.schedule) !== JSON.stringify(to.schedule),
  };
}
