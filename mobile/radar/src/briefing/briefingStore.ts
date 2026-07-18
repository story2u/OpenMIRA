import type { BriefingEvent, BriefingEventType } from '@story2u/radar-core/briefing/events';
import { BRIEFING_EVENT_TYPES } from '@story2u/radar-core/briefing/events';
import type {
  Briefing,
  BriefingItem,
  BriefingScheduleEntry,
} from '@story2u/radar-core/briefing/model';
import { DEFAULT_BRIEFING_SCHEDULE } from '@story2u/radar-core/briefing/model';

export interface BriefingStoreExecutor {
  getAllAsync<Row>(source: string, ...params: Array<string | number | null>): Promise<Row[]>;
  getFirstAsync<Row>(source: string, ...params: Array<string | number | null>): Promise<Row | null>;
  runAsync(source: string, ...params: Array<string | number | null>): Promise<unknown>;
}

export interface BriefingStoreDatabase extends BriefingStoreExecutor {
  withExclusiveTransactionAsync(
    task: (transaction: BriefingStoreExecutor) => Promise<void>,
  ): Promise<void>;
}

export type NewBriefingEvent = BriefingEvent extends infer Event
  ? Event extends BriefingEvent
    ? Omit<Event, 'sequence'>
    : never
  : never;

export class BriefingStoreError extends Error {
  constructor(code: string) {
    super(code);
    this.name = 'BriefingStoreError';
  }
}

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const eventTypes: ReadonlySet<BriefingEventType> = new Set(BRIEFING_EVENT_TYPES);

function requireUuid(value: string, field: string): string {
  if (!UUID_PATTERN.test(value)) throw new BriefingStoreError(`invalid_${field}`);
  return value.toLowerCase();
}

function serializePayload(payload: unknown): string {
  const json = JSON.stringify(payload);
  if (!json || json.length > 262144) throw new BriefingStoreError('invalid_event_payload');
  return json;
}

interface EventRow {
  local_sequence: number;
  owner_id: string;
  event_id: string;
  device_id: string;
  event_type: BriefingEventType;
  aggregate_id: string;
  aggregate_version: number;
  schema_version: number;
  payload_json: string;
  occurred_at: string;
}

function eventFromRow(row: EventRow): BriefingEvent {
  return {
    eventId: row.event_id,
    ownerId: row.owner_id,
    deviceId: row.device_id,
    sequence: row.local_sequence,
    aggregateId: row.aggregate_id,
    aggregateVersion: row.aggregate_version,
    schemaVersion: 1,
    occurredAt: row.occurred_at,
    type: row.event_type,
    payload: JSON.parse(row.payload_json),
  } as BriefingEvent;
}

async function insertEvent(executor: BriefingStoreExecutor, input: NewBriefingEvent) {
  const existing = await executor.getFirstAsync<EventRow>(
    'SELECT * FROM briefing_events WHERE owner_id = ? AND event_id = ?',
    input.ownerId,
    input.eventId,
  );
  if (existing) return eventFromRow(existing);
  await executor.runAsync(
    `INSERT INTO briefing_events (
      owner_id, event_id, device_id, event_type, aggregate_id, aggregate_version,
      schema_version, payload_json, occurred_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    input.ownerId,
    input.eventId,
    input.deviceId,
    input.type,
    input.aggregateId,
    input.aggregateVersion,
    input.schemaVersion,
    serializePayload(input.payload),
    input.occurredAt,
  );
  const row = await executor.getFirstAsync<EventRow>(
    'SELECT * FROM briefing_events WHERE owner_id = ? AND event_id = ?',
    input.ownerId,
    input.eventId,
  );
  if (!row) throw new BriefingStoreError('event_persist_failed');
  return eventFromRow(row);
}

function validateNewEvent(input: NewBriefingEvent): NewBriefingEvent {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const eventId = requireUuid(input.eventId, 'event_id');
  const deviceId = requireUuid(input.deviceId, 'device_id');
  const aggregateId = requireUuid(input.aggregateId, 'aggregate_id');
  if (!eventTypes.has(input.type) || input.schemaVersion !== 1) {
    throw new BriefingStoreError('invalid_briefing_event');
  }
  if (!Number.isInteger(input.aggregateVersion) || input.aggregateVersion < 1) {
    throw new BriefingStoreError('invalid_aggregate_version');
  }
  return { ...input, ownerId, eventId, deviceId, aggregateId } as NewBriefingEvent;
}

export async function appendBriefingEvent(
  database: BriefingStoreDatabase,
  input: NewBriefingEvent,
): Promise<BriefingEvent> {
  const validated = validateNewEvent(input);
  let result: BriefingEvent | null = null;
  await database.withExclusiveTransactionAsync(async (transaction) => {
    result = await insertEvent(transaction, validated);
  });
  if (!result) throw new BriefingStoreError('event_persist_failed');
  return result;
}

export async function readBriefingEvents(
  database: BriefingStoreExecutor,
  ownerId: string,
): Promise<BriefingEvent[]> {
  const rows = await database.getAllAsync<EventRow>(
    'SELECT * FROM briefing_events WHERE owner_id = ? ORDER BY local_sequence',
    requireUuid(ownerId, 'owner_id'),
  );
  return rows.map(eventFromRow);
}

// ---- 投影 ----------------------------------------------------------------

interface BriefingRow {
  owner_id: string;
  id: string;
  briefing_type: Briefing['type'];
  title: string;
  summary: string | null;
  covered_from: string;
  covered_to: string;
  generated_at: string;
  generated_by: Briefing['generatedBy'];
  status: Briefing['status'];
  total_messages: number;
  immediate_count: number;
  inbox_count: number;
  digest_count: number;
  suppressed_count: number;
  included_message_ids_json: string;
  included_opportunity_ids_json: string;
  excluded_handled_ids_json: string;
  category_summaries_json: string;
  evidence_refs_json: string;
}

function briefingFromRow(row: BriefingRow): Briefing {
  return {
    id: row.id,
    type: row.briefing_type,
    title: row.title,
    summary: row.summary,
    coveredFrom: row.covered_from,
    coveredTo: row.covered_to,
    generatedAt: row.generated_at,
    generatedBy: row.generated_by,
    status: row.status,
    totalMessages: row.total_messages,
    immediateCount: row.immediate_count,
    inboxCount: row.inbox_count,
    digestCount: row.digest_count,
    suppressedCount: row.suppressed_count,
    includedMessageIds: JSON.parse(row.included_message_ids_json),
    includedOpportunityIds: JSON.parse(row.included_opportunity_ids_json),
    excludedHandledIds: JSON.parse(row.excluded_handled_ids_json),
    categorySummaries: JSON.parse(row.category_summaries_json),
    evidenceRefs: JSON.parse(row.evidence_refs_json),
  };
}

async function upsertBriefingProjection(
  executor: BriefingStoreExecutor,
  ownerId: string,
  briefing: Briefing,
  items: readonly BriefingItem[],
) {
  await executor.runAsync(
    `INSERT OR REPLACE INTO briefings (
      owner_id, id, briefing_type, title, summary, covered_from, covered_to,
      generated_at, generated_by, status, total_messages, immediate_count,
      inbox_count, digest_count, suppressed_count, included_message_ids_json,
      included_opportunity_ids_json, excluded_handled_ids_json,
      category_summaries_json, evidence_refs_json
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    ownerId,
    briefing.id,
    briefing.type,
    briefing.title,
    briefing.summary,
    briefing.coveredFrom,
    briefing.coveredTo,
    briefing.generatedAt,
    briefing.generatedBy,
    briefing.status,
    briefing.totalMessages,
    briefing.immediateCount,
    briefing.inboxCount,
    briefing.digestCount,
    briefing.suppressedCount,
    JSON.stringify(briefing.includedMessageIds),
    JSON.stringify(briefing.includedOpportunityIds),
    JSON.stringify(briefing.excludedHandledIds),
    JSON.stringify(briefing.categorySummaries),
    JSON.stringify(briefing.evidenceRefs),
  );
  await executor.runAsync(
    'DELETE FROM briefing_items WHERE owner_id = ? AND briefing_id = ?',
    ownerId,
    briefing.id,
  );
  for (const item of items) {
    await executor.runAsync(
      `INSERT INTO briefing_items (
        owner_id, id, briefing_id, item_type, entity_id, priority,
        reason_summary, action_required, handled, order_index
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      ownerId,
      item.id,
      item.briefingId,
      item.itemType,
      item.entityId,
      item.priority,
      item.reasonSummary,
      item.actionRequired ? 1 : 0,
      item.handled ? 1 : 0,
      item.order,
    );
  }
}

/** 生成落库：GenerationStarted + Generated + SnapshotUpdated 事件与投影在同一事务内原子写入。 */
export async function persistBriefingGeneration(
  database: BriefingStoreDatabase,
  input: {
    ownerId: string;
    events: readonly NewBriefingEvent[];
    briefing: Briefing;
    items: readonly BriefingItem[];
  },
): Promise<void> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const validated = input.events.map(validateNewEvent);
  await database.withExclusiveTransactionAsync(async (transaction) => {
    for (const event of validated) await insertEvent(transaction, event);
    await upsertBriefingProjection(transaction, ownerId, input.briefing, input.items);
  });
}

export async function readBriefings(
  database: BriefingStoreExecutor,
  ownerId: string,
  options: { coveredFromInclusive?: string; limit?: number } = {},
): Promise<Briefing[]> {
  const limit = Math.min(100, Math.max(1, options.limit ?? 20));
  const rows = await database.getAllAsync<BriefingRow>(
    `SELECT * FROM briefings WHERE owner_id = ?
     ${options.coveredFromInclusive ? 'AND covered_to >= ?' : ''}
     ORDER BY covered_to DESC LIMIT ?`,
    requireUuid(ownerId, 'owner_id'),
    ...(options.coveredFromInclusive ? [options.coveredFromInclusive] : []),
    limit,
  );
  return rows.map(briefingFromRow);
}

export async function readBriefing(
  database: BriefingStoreExecutor,
  ownerId: string,
  briefingId: string,
): Promise<Briefing | null> {
  const row = await database.getFirstAsync<BriefingRow>(
    'SELECT * FROM briefings WHERE owner_id = ? AND id = ?',
    requireUuid(ownerId, 'owner_id'),
    requireUuid(briefingId, 'briefing_id'),
  );
  return row ? briefingFromRow(row) : null;
}

/** 用户已处理的实体 id 全集：跨全部简报（含已撤销）——"已处理"是用户动作，不随简报撤销失效。 */
export async function readHandledEntityIds(
  database: BriefingStoreExecutor,
  ownerId: string,
): Promise<Set<string>> {
  const rows = await database.getAllAsync<{ entity_id: string }>(
    'SELECT DISTINCT entity_id FROM briefing_items WHERE owner_id = ? AND handled = 1',
    requireUuid(ownerId, 'owner_id'),
  );
  return new Set(rows.map((row) => row.entity_id));
}

export async function readBriefingItems(
  database: BriefingStoreExecutor,
  ownerId: string,
  briefingId: string,
): Promise<BriefingItem[]> {
  const rows = await database.getAllAsync<{
    id: string;
    briefing_id: string;
    item_type: BriefingItem['itemType'];
    entity_id: string;
    priority: BriefingItem['priority'];
    reason_summary: string;
    action_required: number;
    handled: number;
    order_index: number;
  }>(
    `SELECT * FROM briefing_items WHERE owner_id = ? AND briefing_id = ?
     ORDER BY order_index`,
    requireUuid(ownerId, 'owner_id'),
    requireUuid(briefingId, 'briefing_id'),
  );
  return rows.map((row) => ({
    id: row.id,
    briefingId: row.briefing_id,
    itemType: row.item_type,
    entityId: row.entity_id,
    priority: row.priority,
    reasonSummary: row.reason_summary,
    actionRequired: row.action_required === 1,
    handled: row.handled === 1,
    order: row.order_index,
  }));
}

/** 条目处理：事件与投影同事务，保证 fold 与投影一致。 */
export async function persistBriefingItemHandled(
  database: BriefingStoreDatabase,
  input: { event: NewBriefingEvent; ownerId: string; briefingId: string; itemId: string },
): Promise<void> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const validated = validateNewEvent(input.event);
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await insertEvent(transaction, validated);
    await transaction.runAsync(
      'UPDATE briefing_items SET handled = 1 WHERE owner_id = ? AND briefing_id = ? AND id = ?',
      ownerId,
      input.briefingId,
      input.itemId,
    );
  });
}

export async function persistBriefingStatus(
  database: BriefingStoreDatabase,
  input: { event: NewBriefingEvent; ownerId: string; briefingId: string; status: 'dismissed' },
): Promise<void> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const validated = validateNewEvent(input.event);
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await insertEvent(transaction, validated);
    await transaction.runAsync(
      'UPDATE briefings SET status = ? WHERE owner_id = ? AND id = ?',
      input.status,
      ownerId,
      input.briefingId,
    );
  });
}

/** 按分类检索决策引用（证据标签匹配；'other' = 无概念证据）。供 list_category_items 使用。 */
export async function readCategoryDecisionRefs(
  database: BriefingStoreExecutor,
  ownerId: string,
  options: { category: string; decidedFrom: string; limit: number; offset: number },
): Promise<Array<{ messageId: string; decision: string; reasonSummary: string; decidedAt: string }>> {
  const conceptFilter =
    options.category === 'other'
      ? `AND NOT EXISTS (
           SELECT 1 FROM json_each(evidence_json) je
           WHERE json_extract(je.value, '$.kind') IN ('preference', 'message_signal')
         )`
      : `AND EXISTS (
           SELECT 1 FROM json_each(evidence_json) je
           WHERE json_extract(je.value, '$.label') = ?
             AND json_extract(je.value, '$.kind') IN ('preference', 'message_signal')
         )`;
  const rows = await database.getAllAsync<{
    message_id: string;
    decision: string;
    reason_summary: string;
    decided_at: string;
  }>(
    `SELECT message_id, decision, reason_summary, decided_at FROM message_filter_decisions
     WHERE owner_id = ? AND decided_at >= ?
     ${conceptFilter}
     ORDER BY decided_at DESC, message_id LIMIT ? OFFSET ?`,
    requireUuid(ownerId, 'owner_id'),
    options.decidedFrom,
    ...(options.category === 'other' ? [] : [options.category]),
    Math.min(50, Math.max(1, options.limit)),
    Math.max(0, options.offset),
  );
  return rows.map((row) => ({
    messageId: row.message_id,
    decision: row.decision,
    reasonSummary: row.reason_summary,
    decidedAt: row.decided_at,
  }));
}

// ---- 简报时间表 ------------------------------------------------------------

export async function readBriefingSchedule(
  database: BriefingStoreExecutor,
  ownerId: string,
): Promise<BriefingScheduleEntry[]> {
  const rows = await database.getAllAsync<{
    briefing_type: BriefingScheduleEntry['briefingType'];
    minute_of_day: number;
    days_json: string;
    enabled: number;
  }>(
    'SELECT * FROM briefing_schedules WHERE owner_id = ? ORDER BY minute_of_day',
    requireUuid(ownerId, 'owner_id'),
  );
  if (rows.length === 0) return [...DEFAULT_BRIEFING_SCHEDULE];
  return rows.map((row) => ({
    briefingType: row.briefing_type,
    minuteOfDay: row.minute_of_day,
    days: JSON.parse(row.days_json),
    enabled: row.enabled === 1,
  }));
}

export async function persistBriefingSchedule(
  database: BriefingStoreDatabase,
  input: {
    event: NewBriefingEvent;
    ownerId: string;
    entries: readonly BriefingScheduleEntry[];
    updatedAt: string;
  },
): Promise<void> {
  const ownerId = requireUuid(input.ownerId, 'owner_id');
  const validated = validateNewEvent(input.event);
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await insertEvent(transaction, validated);
    await transaction.runAsync('DELETE FROM briefing_schedules WHERE owner_id = ?', ownerId);
    for (const entry of input.entries) {
      await transaction.runAsync(
        `INSERT INTO briefing_schedules (owner_id, briefing_type, minute_of_day, days_json, enabled, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
        ownerId,
        entry.briefingType,
        entry.minuteOfDay,
        JSON.stringify(entry.days),
        entry.enabled ? 1 : 0,
        input.updatedAt,
      );
    }
  });
}
