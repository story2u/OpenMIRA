import type { SignalAppetiteEvent, SignalAppetiteEventType } from '@story2u/radar-core/attention/events';
import type {
  AttentionIntent,
  AttentionPreference,
  MessageFilterDecision,
  PreferenceExample,
  TeachingSession,
} from '@story2u/radar-core/attention/model';

type SqliteValue = string | number | null;

export interface SignalAppetiteStoreExecutor {
  getAllAsync<Row>(source: string, ...params: SqliteValue[]): Promise<Row[]>;
  getFirstAsync<Row>(source: string, ...params: SqliteValue[]): Promise<Row | null>;
  runAsync(source: string, ...params: SqliteValue[]): Promise<unknown>;
}

export interface SignalAppetiteStoreDatabase extends SignalAppetiteStoreExecutor {
  withExclusiveTransactionAsync(
    task: (transaction: SignalAppetiteStoreExecutor) => Promise<void>,
  ): Promise<void>;
}

export type NewSignalAppetiteEvent = SignalAppetiteEvent extends infer Event
  ? Event extends SignalAppetiteEvent
    ? Omit<Event, 'sequence'>
    : never
  : never;

interface EventRow {
  local_sequence: number;
  owner_id: string;
  event_id: string;
  device_id: string;
  event_type: SignalAppetiteEventType;
  aggregate_id: string;
  aggregate_version: number;
  schema_version: 1;
  payload_json: string;
  occurred_at: string;
}

interface TeachingSessionRow {
  id: string;
  started_at: string;
  completed_at: string | null;
  presented_count: number;
  positive_count: number;
  negative_count: number;
  skipped_count: number;
  status: TeachingSession['status'];
  proposed_preference_version: number | null;
  summary_json: string | null;
}

interface PreferenceExampleRow {
  id: string;
  message_id: string;
  label: PreferenceExample['label'];
  selected_reasons_json: string;
  freeform_reason: string | null;
  captured_at: string;
  teaching_session_id: string;
  reverted_at: string | null;
}

interface PreferenceRow {
  id: string;
  version: number;
  title: string;
  natural_language_summary: string;
  scope: AttentionPreference['scope'];
  status: AttentionPreference['status'];
  confidence: number;
  active_from: string | null;
  active_until: string | null;
  schedule_json: string;
  created_at: string;
  updated_at: string;
}

interface IntentRow {
  id: string;
  preference_id: string;
  concept: string;
  intent_type: AttentionIntent['intentType'];
  weight: number;
  delivery_mode: AttentionIntent['deliveryMode'];
  confidence: number;
  user_confirmed: number;
  source: AttentionIntent['source'];
  valid_from: string | null;
  valid_until: string | null;
}

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const eventTypes = new Set<SignalAppetiteEventType>([
  'TeachingSessionStarted',
  'TeachingCardPresented',
  'PreferenceExampleCaptured',
  'PreferenceExampleReverted',
  'TeachingSessionCompleted',
  'PreferenceChangeProposed',
  'PreferenceSimulationCompleted',
  'PreferenceShadowStarted',
  'PreferenceApplied',
  'PreferenceReverted',
  'MessageFilterDecisionMade',
  'MessageDecisionCorrected',
  'IntentMapUpdated',
  'TemporaryFocusCreated',
  'TemporaryFocusExpired',
]);

export class SignalAppetiteStoreError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'SignalAppetiteStoreError';
  }
}

function requireUuid(value: string, field: string) {
  if (!uuidPattern.test(value)) throw new SignalAppetiteStoreError(`invalid_${field}`);
  return value.toLowerCase();
}

function serializePayload(payload: unknown) {
  let serialized: string;
  try {
    serialized = JSON.stringify(payload);
  } catch {
    throw new SignalAppetiteStoreError('invalid_attention_event');
  }
  if (new TextEncoder().encode(serialized).byteLength > 65_536) {
    throw new SignalAppetiteStoreError('attention_event_too_large');
  }
  return serialized;
}

function eventFromRow(row: EventRow): SignalAppetiteEvent {
  if (!eventTypes.has(row.event_type) || row.schema_version !== 1) {
    throw new SignalAppetiteStoreError('attention_event_corrupt');
  }
  let payload: unknown;
  try {
    payload = JSON.parse(row.payload_json);
  } catch {
    throw new SignalAppetiteStoreError('attention_event_corrupt');
  }
  return {
    eventId: row.event_id,
    ownerId: row.owner_id,
    deviceId: row.device_id,
    sequence: row.local_sequence,
    aggregateId: row.aggregate_id,
    aggregateVersion: row.aggregate_version,
    schemaVersion: row.schema_version,
    occurredAt: row.occurred_at,
    type: row.event_type,
    payload,
  } as SignalAppetiteEvent;
}

async function upsertPreference(
  executor: SignalAppetiteStoreExecutor,
  ownerId: string,
  preference: AttentionPreference,
) {
  await executor.runAsync(
    `INSERT INTO attention_preferences (
      owner_id, id, version, title, natural_language_summary, scope, status,
      confidence, active_from, active_until, schedule_json, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT (owner_id, id, version) DO UPDATE SET
      title = excluded.title,
      natural_language_summary = excluded.natural_language_summary,
      scope = excluded.scope,
      confidence = excluded.confidence,
      active_until = excluded.active_until,
      schedule_json = excluded.schedule_json,
      updated_at = excluded.updated_at`,
    ownerId,
    requireUuid(preference.id, 'preference_id'),
    preference.version,
    preference.title.trim(),
    preference.naturalLanguageSummary.trim(),
    preference.scope,
    'candidate',
    preference.confidence,
    preference.activeFrom,
    preference.activeUntil,
    JSON.stringify(preference.schedule),
    preference.createdAt,
    preference.updatedAt,
  );
}

async function replaceIntents(
  executor: SignalAppetiteStoreExecutor,
  ownerId: string,
  preferenceId: string,
  preferenceVersion: number,
  intents: readonly AttentionIntent[],
) {
  await executor.runAsync(
    `DELETE FROM attention_intents
     WHERE owner_id = ? AND preference_id = ? AND preference_version = ?`,
    ownerId,
    preferenceId,
    preferenceVersion,
  );
  for (const intent of intents) {
    if (intent.preferenceId !== preferenceId) {
      throw new SignalAppetiteStoreError('intent_preference_mismatch');
    }
    await executor.runAsync(
      `INSERT INTO attention_intents (
        owner_id, id, preference_id, preference_version, concept, intent_type,
        weight, delivery_mode, confidence, user_confirmed, source, valid_from, valid_until
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      ownerId,
      requireUuid(intent.id, 'intent_id'),
      preferenceId,
      preferenceVersion,
      intent.concept.trim(),
      intent.intentType,
      intent.weight,
      intent.deliveryMode,
      intent.confidence,
      intent.userConfirmed ? 1 : 0,
      intent.source,
      intent.validFrom,
      intent.validUntil,
    );
  }
}

async function insertExample(
  executor: SignalAppetiteStoreExecutor,
  ownerId: string,
  example: PreferenceExample,
) {
  await executor.runAsync(
    `INSERT INTO preference_examples (
      owner_id, id, message_id, label, selected_reasons_json, freeform_reason,
      captured_at, teaching_session_id, reverted_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    ownerId,
    requireUuid(example.id, 'example_id'),
    requireUuid(example.messageId, 'message_id'),
    example.label,
    JSON.stringify(example.selectedReasons),
    example.freeformReason?.trim() || null,
    example.capturedAt,
    requireUuid(example.teachingSessionId, 'teaching_session_id'),
    example.revertedAt,
  );
}

async function upsertDecision(
  executor: SignalAppetiteStoreExecutor,
  ownerId: string,
  decision: MessageFilterDecision,
) {
  await executor.runAsync(
    `INSERT INTO message_filter_decisions (
      owner_id, message_id, preference_version, decision, confidence,
      reason_summary, evidence_json, evaluator, decided_at, expires_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT (owner_id, message_id) DO UPDATE SET
      preference_version = excluded.preference_version,
      decision = excluded.decision,
      confidence = excluded.confidence,
      reason_summary = excluded.reason_summary,
      evidence_json = excluded.evidence_json,
      evaluator = excluded.evaluator,
      decided_at = excluded.decided_at,
      expires_at = excluded.expires_at`,
    ownerId,
    requireUuid(decision.messageId, 'message_id'),
    decision.preferenceVersion,
    decision.decision,
    decision.confidence,
    decision.reasonSummary.trim(),
    JSON.stringify(decision.evidence),
    decision.evaluator,
    decision.decidedAt,
    decision.expiresAt,
  );
}

async function projectEvent(
  executor: SignalAppetiteStoreExecutor,
  event: SignalAppetiteEvent,
) {
  const ownerId = event.ownerId;
  switch (event.type) {
    case 'TeachingSessionStarted':
      await executor.runAsync(
        `INSERT INTO teaching_sessions (
          owner_id, id, started_at, target_count, status
        ) VALUES (?, ?, ?, ?, 'active')`,
        ownerId,
        requireUuid(event.payload.sessionId, 'teaching_session_id'),
        event.occurredAt,
        event.payload.targetCount,
      );
      break;
    case 'TeachingCardPresented':
      await executor.runAsync(
        `UPDATE teaching_sessions SET presented_count = presented_count + 1
         WHERE owner_id = ? AND id = ? AND status = 'active'`,
        ownerId,
        event.payload.sessionId,
      );
      break;
    case 'PreferenceExampleCaptured':
      await insertExample(executor, ownerId, event.payload.example);
      await executor.runAsync(
        `UPDATE teaching_sessions SET
          positive_count = positive_count + ?,
          negative_count = negative_count + ?,
          skipped_count = skipped_count + ?
         WHERE owner_id = ? AND id = ? AND status = 'active'`,
        event.payload.example.label === 'positive' ? 1 : 0,
        event.payload.example.label === 'negative' ? 1 : 0,
        event.payload.example.label === 'skipped' ? 1 : 0,
        ownerId,
        event.payload.example.teachingSessionId,
      );
      break;
    case 'PreferenceExampleReverted': {
      const example = await executor.getFirstAsync<{ label: PreferenceExample['label'] }>(
        `SELECT label FROM preference_examples
         WHERE owner_id = ? AND id = ? AND reverted_at IS NULL`,
        ownerId,
        event.payload.exampleId,
      );
      if (!example) break;
      await executor.runAsync(
        `UPDATE preference_examples SET reverted_at = ? WHERE owner_id = ? AND id = ?`,
        event.payload.revertedAt,
        ownerId,
        event.payload.exampleId,
      );
      await executor.runAsync(
        `UPDATE teaching_sessions SET
          positive_count = MAX(0, positive_count - ?),
          negative_count = MAX(0, negative_count - ?),
          skipped_count = MAX(0, skipped_count - ?)
         WHERE owner_id = ? AND id = (
           SELECT teaching_session_id FROM preference_examples WHERE owner_id = ? AND id = ?
         )`,
        example.label === 'positive' ? 1 : 0,
        example.label === 'negative' ? 1 : 0,
        example.label === 'skipped' ? 1 : 0,
        ownerId,
        ownerId,
        event.payload.exampleId,
      );
      break;
    }
    case 'TeachingSessionCompleted':
      await executor.runAsync(
        `UPDATE teaching_sessions SET completed_at = ?, status = 'summarized', summary_json = ?
         WHERE owner_id = ? AND id = ?`,
        event.payload.completedAt,
        JSON.stringify(event.payload.summary),
        ownerId,
        event.payload.sessionId,
      );
      break;
    case 'PreferenceChangeProposed':
      await upsertPreference(executor, ownerId, event.payload.preference);
      await replaceIntents(
        executor,
        ownerId,
        event.payload.preference.id,
        event.payload.preference.version,
        event.payload.intents,
      );
      if (event.payload.teachingSessionId) {
        await executor.runAsync(
          `UPDATE teaching_sessions SET status = 'completed', proposed_preference_version = ?
           WHERE owner_id = ? AND id = ?`,
          event.payload.preference.version,
          ownerId,
          event.payload.teachingSessionId,
        );
      }
      break;
    case 'PreferenceSimulationCompleted':
      break;
    case 'PreferenceShadowStarted':
      await executor.runAsync(
        `INSERT INTO shadow_evaluations (
          owner_id, id, old_version, candidate_version, started_at, ends_at,
          diff_summary_json, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        ownerId,
        event.payload.shadow.id,
        event.payload.shadow.oldVersion,
        event.payload.shadow.candidateVersion,
        event.payload.shadow.startedAt,
        event.payload.shadow.endsAt,
        JSON.stringify(event.payload.shadow.diffSummary),
        event.payload.shadow.status,
      );
      break;
    case 'PreferenceApplied':
      await executor.runAsync(
        `UPDATE attention_preferences SET status = 'superseded', updated_at = ?
         WHERE owner_id = ? AND status = 'active'`,
        event.payload.appliedAt,
        ownerId,
      );
      await executor.runAsync(
        `UPDATE attention_preferences SET status = 'active', active_from = ?, updated_at = ?
         WHERE owner_id = ? AND id = ? AND version = ? AND status = 'candidate'`,
        event.payload.appliedAt,
        event.payload.appliedAt,
        ownerId,
        event.payload.preferenceId,
        event.payload.version,
      );
      break;
    case 'PreferenceReverted':
      await executor.runAsync(
        `UPDATE attention_preferences SET status = 'reverted', updated_at = ?
         WHERE owner_id = ? AND id = ? AND version = ? AND status = 'active'`,
        event.payload.revertedAt,
        ownerId,
        event.payload.preferenceId,
        event.payload.fromVersion,
      );
      await executor.runAsync(
        `UPDATE attention_preferences SET status = 'active', updated_at = ?
         WHERE owner_id = ? AND id = ? AND version = ?`,
        event.payload.revertedAt,
        ownerId,
        event.payload.preferenceId,
        event.payload.toVersion,
      );
      break;
    case 'MessageFilterDecisionMade':
      await upsertDecision(executor, ownerId, event.payload.decision);
      break;
    case 'MessageDecisionCorrected':
      await upsertDecision(executor, ownerId, event.payload.correctedDecision);
      await insertExample(executor, ownerId, event.payload.example);
      await executor.runAsync(
        `UPDATE teaching_sessions SET
          presented_count = presented_count + 1,
          positive_count = positive_count + ?,
          negative_count = negative_count + ?
         WHERE owner_id = ? AND id = ?`,
        event.payload.example.label === 'positive' ? 1 : 0,
        event.payload.example.label === 'negative' ? 1 : 0,
        ownerId,
        event.payload.example.teachingSessionId,
      );
      break;
    case 'IntentMapUpdated':
      await replaceIntents(
        executor,
        ownerId,
        event.payload.preferenceId,
        event.payload.version,
        event.payload.intents,
      );
      break;
    case 'TemporaryFocusCreated':
      await executor.runAsync(
        `INSERT INTO temporary_focuses (
          owner_id, id, concept, delivery_mode, created_at, expires_at, expired_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
        ownerId,
        event.payload.focus.id,
        event.payload.focus.concept.trim(),
        event.payload.focus.deliveryMode,
        event.payload.focus.createdAt,
        event.payload.focus.expiresAt,
        event.payload.focus.expiredAt,
      );
      break;
    case 'TemporaryFocusExpired':
      await executor.runAsync(
        `UPDATE temporary_focuses SET expired_at = ? WHERE owner_id = ? AND id = ?`,
        event.payload.expiredAt,
        ownerId,
        event.payload.focusId,
      );
      break;
  }
}

export async function appendSignalAppetiteEvent(
  database: SignalAppetiteStoreDatabase,
  input: NewSignalAppetiteEvent,
): Promise<SignalAppetiteEvent> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const eventId = requireUuid(input.eventId, 'event_id');
  const deviceId = requireUuid(input.deviceId, 'device_id');
  const aggregateId = requireUuid(input.aggregateId, 'aggregate_id');
  if (!eventTypes.has(input.type) || input.schemaVersion !== 1) {
    throw new SignalAppetiteStoreError('invalid_attention_event');
  }
  if (!Number.isInteger(input.aggregateVersion) || input.aggregateVersion < 1) {
    throw new SignalAppetiteStoreError('invalid_aggregate_version');
  }
  const payloadJson = serializePayload(input.payload);
  let result: SignalAppetiteEvent | null = null;
  await database.withExclusiveTransactionAsync(async (transaction) => {
    const existing = await transaction.getFirstAsync<EventRow>(
      'SELECT * FROM attention_events WHERE owner_id = ? AND event_id = ?',
      ownerId,
      eventId,
    );
    if (existing) {
      result = eventFromRow(existing);
      return;
    }
    await transaction.runAsync(
      `INSERT INTO attention_events (
        owner_id, event_id, device_id, event_type, aggregate_id, aggregate_version,
        schema_version, payload_json, occurred_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      ownerId,
      eventId,
      deviceId,
      input.type,
      aggregateId,
      input.aggregateVersion,
      input.schemaVersion,
      payloadJson,
      input.occurredAt,
    );
    const stored = await transaction.getFirstAsync<EventRow>(
      'SELECT * FROM attention_events WHERE owner_id = ? AND event_id = ?',
      ownerId,
      eventId,
    );
    if (!stored) throw new SignalAppetiteStoreError('attention_event_not_persisted');
    result = eventFromRow(stored);
    await projectEvent(transaction, result);
  });
  if (!result) throw new SignalAppetiteStoreError('attention_event_not_persisted');
  return result;
}

export async function readSignalAppetiteEvents(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  options: { afterSequence?: number; limit?: number } = {},
) {
  const afterSequence = options.afterSequence ?? 0;
  const limit = options.limit ?? 500;
  if (!Number.isInteger(afterSequence) || afterSequence < 0) {
    throw new SignalAppetiteStoreError('invalid_attention_event_cursor');
  }
  if (!Number.isInteger(limit) || limit < 1 || limit > 1_000) {
    throw new SignalAppetiteStoreError('invalid_attention_event_limit');
  }
  const rows = await database.getAllAsync<EventRow>(
    `SELECT * FROM attention_events
     WHERE owner_id = ? AND local_sequence > ? ORDER BY local_sequence LIMIT ?`,
    requireUuid(ownerId, 'owner_id'),
    afterSequence,
    limit,
  );
  return rows.map(eventFromRow);
}

export async function readTeachingSession(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  sessionId: string,
): Promise<TeachingSession | null> {
  const row = await database.getFirstAsync<TeachingSessionRow>(
    'SELECT * FROM teaching_sessions WHERE owner_id = ? AND id = ?',
    requireUuid(ownerId, 'owner_id'),
    requireUuid(sessionId, 'teaching_session_id'),
  );
  if (!row) return null;
  return {
    id: row.id,
    startedAt: row.started_at,
    completedAt: row.completed_at,
    presentedCount: row.presented_count,
    positiveCount: row.positive_count,
    negativeCount: row.negative_count,
    skippedCount: row.skipped_count,
    status: row.status,
    proposedPreferenceVersion: row.proposed_preference_version,
    summary: row.summary_json ? JSON.parse(row.summary_json) : null,
  };
}

export async function readPreferenceExamples(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  sessionId: string,
) {
  const rows = await database.getAllAsync<PreferenceExampleRow>(
    `SELECT * FROM preference_examples
     WHERE owner_id = ? AND teaching_session_id = ? ORDER BY captured_at, id`,
    requireUuid(ownerId, 'owner_id'),
    requireUuid(sessionId, 'teaching_session_id'),
  );
  return rows.map((row): PreferenceExample => ({
    id: row.id,
    messageId: row.message_id,
    label: row.label,
    selectedReasons: JSON.parse(row.selected_reasons_json),
    freeformReason: row.freeform_reason,
    capturedAt: row.captured_at,
    teachingSessionId: row.teaching_session_id,
    revertedAt: row.reverted_at,
  }));
}

function preferenceFromRow(row: PreferenceRow): AttentionPreference {
  return {
    id: row.id,
    version: row.version,
    title: row.title,
    naturalLanguageSummary: row.natural_language_summary,
    scope: row.scope,
    status: row.status,
    confidence: row.confidence,
    activeFrom: row.active_from,
    activeUntil: row.active_until,
    schedule: JSON.parse(row.schedule_json),
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export async function readPreferenceVersion(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  version: number,
): Promise<AttentionPreference | null> {
  const row = await database.getFirstAsync<PreferenceRow>(
    `SELECT * FROM attention_preferences WHERE owner_id = ? AND version = ?`,
    requireUuid(ownerId, 'owner_id'),
    version,
  );
  return row ? preferenceFromRow(row) : null;
}

export async function readActivePreference(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
): Promise<AttentionPreference | null> {
  const row = await database.getFirstAsync<PreferenceRow>(
    `SELECT * FROM attention_preferences WHERE owner_id = ? AND status = 'active'`,
    requireUuid(ownerId, 'owner_id'),
  );
  return row ? preferenceFromRow(row) : null;
}

export async function readPreferenceIntents(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  preferenceId: string,
  preferenceVersion: number,
): Promise<AttentionIntent[]> {
  const rows = await database.getAllAsync<IntentRow>(
    `SELECT * FROM attention_intents
     WHERE owner_id = ? AND preference_id = ? AND preference_version = ?
     ORDER BY abs(weight) DESC, concept, id`,
    requireUuid(ownerId, 'owner_id'),
    requireUuid(preferenceId, 'preference_id'),
    preferenceVersion,
  );
  return rows.map((row) => ({
    id: row.id,
    preferenceId: row.preference_id,
    concept: row.concept,
    intentType: row.intent_type,
    weight: row.weight,
    deliveryMode: row.delivery_mode,
    confidence: row.confidence,
    userConfirmed: row.user_confirmed === 1,
    source: row.source,
    validFrom: row.valid_from,
    validUntil: row.valid_until,
  }));
}

export async function readLatestPreferenceVersion(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
) {
  const row = await database.getFirstAsync<{ maximum: number | null }>(
    'SELECT MAX(version) AS maximum FROM attention_preferences WHERE owner_id = ?',
    requireUuid(ownerId, 'owner_id'),
  );
  return row?.maximum ?? 0;
}

export async function readMessageFilterDecisions(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  options: { decision?: MessageFilterDecision['decision']; limit?: number } = {},
): Promise<MessageFilterDecision[]> {
  const limit = Math.min(500, Math.max(1, options.limit ?? 100));
  const rows = await database.getAllAsync<{
    message_id: string;
    preference_version: number;
    decision: MessageFilterDecision['decision'];
    confidence: number;
    reason_summary: string;
    evidence_json: string;
    evaluator: MessageFilterDecision['evaluator'];
    decided_at: string;
    expires_at: string | null;
  }>(
    `SELECT * FROM message_filter_decisions WHERE owner_id = ?
     ${options.decision ? 'AND decision = ?' : ''}
     ORDER BY decided_at DESC, message_id LIMIT ?`,
    requireUuid(ownerId, 'owner_id'),
    ...(options.decision ? [options.decision] : []),
    limit,
  );
  return rows.map((row) => ({
    messageId: row.message_id,
    preferenceVersion: row.preference_version,
    decision: row.decision,
    confidence: row.confidence,
    reasonSummary: row.reason_summary,
    evidence: JSON.parse(row.evidence_json),
    evaluator: row.evaluator,
    decidedAt: row.decided_at,
    expiresAt: row.expires_at,
  }));
}
