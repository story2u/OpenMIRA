import type {
  SyncBootstrap,
  SyncChange,
  SyncSnapshotItem,
} from '@story2u/radar-contracts/sync';

export interface SyncStoreExecutor {
  getAllAsync<Row>(source: string, ...params: Array<string | number | null>): Promise<Row[]>;
  getFirstAsync<Row>(source: string, ...params: Array<string | number | null>): Promise<Row | null>;
  runAsync(source: string, ...params: Array<string | number | null>): Promise<unknown>;
}

export interface SyncStoreDatabase extends SyncStoreExecutor {
  withExclusiveTransactionAsync(
    task: (transaction: SyncStoreExecutor) => Promise<void>,
  ): Promise<void>;
}

export type LocalSyncPhase = 'ready' | 'bootstrapping' | 'error';

export interface LocalSyncState {
  cursor: number;
  phase: LocalSyncPhase;
  lastErrorCode: string | null;
}

export interface LocalBootstrapState {
  watermarkCursor: number;
  nextPageToken: string | null;
}

export class LocalSyncStateError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'LocalSyncStateError';
  }
}

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const streamName = 'main';
const supportedSettingTypes = new Set([
  'user_detection_preference',
  'user_work_schedule',
  'user_notification_preference',
]);

function requireOwnerId(ownerId: string) {
  if (!uuidPattern.test(ownerId)) throw new LocalSyncStateError('invalid_owner');
}

function canonicalValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalValue);
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, item]) => [key, canonicalValue(item)]),
    );
  }
  return value;
}

function canonicalJson(value: unknown) {
  return JSON.stringify(canonicalValue(value));
}

function recordPayload(item: SyncSnapshotItem | SyncChange) {
  if (item.payload === null || typeof item.payload !== 'object') {
    throw new LocalSyncStateError('invalid_payload');
  }
  return item.payload as unknown as Record<string, unknown>;
}

function isDelete(item: SyncSnapshotItem | SyncChange): item is SyncChange & {
  operation: 'delete';
  payload: null;
} {
  return 'operation' in item && item.operation === 'delete';
}

function requirePayloadIdentity(
  item: SyncSnapshotItem | SyncChange,
  ownerId: string,
) {
  if (supportedSettingTypes.has(item.aggregateType)) {
    if (item.aggregateId !== ownerId) throw new LocalSyncStateError('owner_mismatch');
    return;
  }
  if (item.aggregateType === 'reply_template') {
    throw new LocalSyncStateError('unsupported_aggregate');
  }
  if (isDelete(item)) return;
  const payload = recordPayload(item);
  if (payload.id !== item.aggregateId) throw new LocalSyncStateError('identity_mismatch');
}

async function upsertOpportunity(
  executor: SyncStoreExecutor,
  ownerId: string,
  item: SyncSnapshotItem | SyncChange,
  now: string,
) {
  if (isDelete(item)) {
    await executor.runAsync(
      `INSERT INTO opportunity_projection (
        owner_id, id, aggregate_version, payload_json, updated_at, deleted_at
      ) VALUES (?, ?, ?, 'null', ?, ?)
      ON CONFLICT(owner_id, id) DO UPDATE SET
        aggregate_version = excluded.aggregate_version,
        payload_json = excluded.payload_json,
        updated_at = excluded.updated_at,
        deleted_at = excluded.deleted_at
      WHERE excluded.aggregate_version > opportunity_projection.aggregate_version`,
      ownerId,
      item.aggregateId,
      item.aggregateVersion,
      now,
      now,
    );
    return;
  }
  const payload = recordPayload(item);
  await executor.runAsync(
    `INSERT INTO opportunity_projection (
      owner_id, id, aggregate_version, payload_json, updated_at, deleted_at,
      platform, frontend_status, source_type, created_at, last_message_at,
      trust_score, sop_stage, confidence_score, attention_required, archived_at
    ) VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(owner_id, id) DO UPDATE SET
      aggregate_version = excluded.aggregate_version,
      payload_json = excluded.payload_json,
      updated_at = excluded.updated_at,
      deleted_at = NULL,
      platform = excluded.platform,
      frontend_status = excluded.frontend_status,
      source_type = excluded.source_type,
      created_at = excluded.created_at,
      last_message_at = excluded.last_message_at,
      trust_score = excluded.trust_score,
      sop_stage = excluded.sop_stage,
      confidence_score = excluded.confidence_score,
      attention_required = excluded.attention_required,
      archived_at = excluded.archived_at
    WHERE excluded.aggregate_version > opportunity_projection.aggregate_version`,
    ownerId,
    item.aggregateId,
    item.aggregateVersion,
    canonicalJson(payload),
    String(payload.updatedAt ?? now),
    String(payload.platform ?? ''),
    String(payload.status ?? ''),
    String(payload.sourceType ?? ''),
    String(payload.createdAt ?? now),
    String(payload.updatedAt ?? now),
    Number(payload.trustScore ?? 0),
    String(payload.sopStage ?? ''),
    Number(payload.confidenceScore ?? 0),
    payload.attentionRequired === true ? 1 : 0,
    typeof payload.archivedAt === 'string' ? payload.archivedAt : null,
  );
}

async function upsertMessage(
  executor: SyncStoreExecutor,
  ownerId: string,
  item: SyncSnapshotItem | SyncChange,
  now: string,
) {
  if (isDelete(item)) {
    await executor.runAsync(
      `INSERT INTO message_projection (
        owner_id, id, opportunity_id, aggregate_version, sent_at,
        payload_json, updated_at, deleted_at
      ) VALUES (?, ?, NULL, ?, ?, 'null', ?, ?)
      ON CONFLICT(owner_id, id) DO UPDATE SET
        aggregate_version = excluded.aggregate_version,
        payload_json = excluded.payload_json,
        updated_at = excluded.updated_at,
        deleted_at = excluded.deleted_at
      WHERE excluded.aggregate_version > message_projection.aggregate_version`,
      ownerId,
      item.aggregateId,
      item.aggregateVersion,
      now,
      now,
      now,
    );
    return;
  }
  const payload = recordPayload(item);
  await executor.runAsync(
    `INSERT INTO message_projection (
      owner_id, id, opportunity_id, aggregate_version, sent_at,
      payload_json, updated_at, deleted_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
    ON CONFLICT(owner_id, id) DO UPDATE SET
      opportunity_id = excluded.opportunity_id,
      aggregate_version = excluded.aggregate_version,
      sent_at = excluded.sent_at,
      payload_json = excluded.payload_json,
      updated_at = excluded.updated_at,
      deleted_at = NULL
    WHERE excluded.aggregate_version > message_projection.aggregate_version`,
    ownerId,
    item.aggregateId,
    typeof payload.opportunityId === 'string' ? payload.opportunityId : null,
    item.aggregateVersion,
    String(payload.sentAt ?? now),
    canonicalJson(payload),
    now,
  );
}

async function upsertSetting(
  executor: SyncStoreExecutor,
  ownerId: string,
  item: SyncSnapshotItem | SyncChange,
  now: string,
) {
  const deletedAt = isDelete(item) ? now : null;
  const payloadJson = isDelete(item) ? 'null' : canonicalJson(recordPayload(item));
  await executor.runAsync(
    `INSERT INTO setting_projection (
      owner_id, setting_type, aggregate_version, payload_json, updated_at, deleted_at
    ) VALUES (?, ?, ?, ?, ?, ?)
    ON CONFLICT(owner_id, setting_type) DO UPDATE SET
      aggregate_version = excluded.aggregate_version,
      payload_json = excluded.payload_json,
      updated_at = excluded.updated_at,
      deleted_at = excluded.deleted_at
    WHERE excluded.aggregate_version > setting_projection.aggregate_version`,
    ownerId,
    item.aggregateType,
    item.aggregateVersion,
    payloadJson,
    now,
    deletedAt,
  );
}

async function applyProjection(
  executor: SyncStoreExecutor,
  ownerId: string,
  item: SyncSnapshotItem | SyncChange,
  now: string,
) {
  requirePayloadIdentity(item, ownerId);
  if (item.aggregateType === 'opportunity') {
    await upsertOpportunity(executor, ownerId, item, now);
  } else if (item.aggregateType === 'message') {
    await upsertMessage(executor, ownerId, item, now);
  } else if (supportedSettingTypes.has(item.aggregateType)) {
    await upsertSetting(executor, ownerId, item, now);
  } else {
    throw new LocalSyncStateError('unsupported_aggregate');
  }
}

export async function readLocalSyncState(
  database: SyncStoreExecutor,
  ownerId: string,
): Promise<LocalSyncState | null> {
  requireOwnerId(ownerId);
  const row = await database.getFirstAsync<{
    cursor: number;
    phase: LocalSyncPhase;
    last_error_code: string | null;
  }>(
    `SELECT cursor, phase, last_error_code
     FROM sync_state WHERE owner_id = ? AND stream = ?`,
    ownerId,
    streamName,
  );
  return row
    ? { cursor: row.cursor, phase: row.phase, lastErrorCode: row.last_error_code }
    : null;
}

export async function readLocalBootstrapState(
  database: SyncStoreExecutor,
  ownerId: string,
): Promise<LocalBootstrapState | null> {
  requireOwnerId(ownerId);
  const row = await database.getFirstAsync<{
    watermark_cursor: number;
    next_page_token: string | null;
  }>(
    `SELECT watermark_cursor, next_page_token
     FROM sync_bootstrap_state WHERE owner_id = ?`,
    ownerId,
  );
  return row
    ? { watermarkCursor: row.watermark_cursor, nextPageToken: row.next_page_token }
    : null;
}

export async function applyBootstrapPage(
  database: SyncStoreDatabase,
  ownerId: string,
  page: SyncBootstrap,
  options: { restart: boolean },
) {
  requireOwnerId(ownerId);
  const now = new Date().toISOString();
  await database.withExclusiveTransactionAsync(async (transaction) => {
    if (options.restart) {
      await transaction.runAsync('DELETE FROM message_projection WHERE owner_id = ?', ownerId);
      await transaction.runAsync('DELETE FROM setting_projection WHERE owner_id = ?', ownerId);
      await transaction.runAsync('DELETE FROM opportunity_projection WHERE owner_id = ?', ownerId);
      await transaction.runAsync('DELETE FROM change_inbox WHERE owner_id = ?', ownerId);
      await transaction.runAsync('DELETE FROM sync_bootstrap_state WHERE owner_id = ?', ownerId);
      await transaction.runAsync('DELETE FROM sync_state WHERE owner_id = ?', ownerId);
      await transaction.runAsync(
        `INSERT INTO sync_state (
          owner_id, stream, cursor, updated_at, phase, last_error_code
        ) VALUES (?, ?, 0, ?, 'bootstrapping', NULL)`,
        ownerId,
        streamName,
        now,
      );
    } else {
      const state = await readLocalSyncState(transaction, ownerId);
      const bootstrap = await readLocalBootstrapState(transaction, ownerId);
      if (
        state?.phase !== 'bootstrapping' ||
        !bootstrap ||
        bootstrap.watermarkCursor !== page.watermarkCursor
      ) {
        throw new LocalSyncStateError('bootstrap_state_conflict');
      }
    }

    for (const item of page.items) {
      await applyProjection(transaction, ownerId, item, now);
    }

    if (page.hasMore) {
      if (!page.nextPageToken) throw new LocalSyncStateError('bootstrap_token_missing');
      await transaction.runAsync(
        `INSERT INTO sync_bootstrap_state (
          owner_id, watermark_cursor, next_page_token, updated_at
        ) VALUES (?, ?, ?, ?)
        ON CONFLICT(owner_id) DO UPDATE SET
          watermark_cursor = excluded.watermark_cursor,
          next_page_token = excluded.next_page_token,
          updated_at = excluded.updated_at`,
        ownerId,
        page.watermarkCursor,
        page.nextPageToken,
        now,
      );
      return;
    }

    await transaction.runAsync(
      `UPDATE sync_state SET cursor = ?, phase = 'ready',
        last_error_code = NULL, updated_at = ?
       WHERE owner_id = ? AND stream = ?`,
      page.watermarkCursor,
      now,
      ownerId,
      streamName,
    );
    await transaction.runAsync('DELETE FROM sync_bootstrap_state WHERE owner_id = ?', ownerId);
  });
}

interface StoredInboxChange {
  cursor: number;
  aggregate_type: string;
  aggregate_id: string;
  aggregate_version: number;
  operation: string;
  schema_version: number;
  payload_json: string;
}

function sameStoredChange(existing: StoredInboxChange, change: SyncChange, payloadJson: string) {
  return existing.cursor === change.cursor &&
    existing.aggregate_type === change.aggregateType &&
    existing.aggregate_id === change.aggregateId &&
    existing.aggregate_version === change.aggregateVersion &&
    existing.operation === change.operation &&
    existing.schema_version === change.schemaVersion &&
    existing.payload_json === payloadJson;
}

export async function applyChangePage(
  database: SyncStoreDatabase,
  ownerId: string,
  requestedAfter: number,
  page: { changes: SyncChange[]; nextCursor: number; resetRequired: boolean },
) {
  requireOwnerId(ownerId);
  if (page.resetRequired) throw new LocalSyncStateError('server_reset_required');
  const now = new Date().toISOString();
  await database.withExclusiveTransactionAsync(async (transaction) => {
    const state = await readLocalSyncState(transaction, ownerId);
    if (!state || state.phase !== 'ready') throw new LocalSyncStateError('sync_not_ready');
    if (state.cursor !== requestedAfter) {
      if (state.cursor >= page.nextCursor) return;
      throw new LocalSyncStateError('cursor_conflict');
    }

    let previousCursor = requestedAfter;
    for (const change of page.changes) {
      if (change.cursor <= previousCursor || change.cursor > page.nextCursor) {
        throw new LocalSyncStateError('change_order_invalid');
      }
      previousCursor = change.cursor;
      const payloadJson = canonicalJson(change.payload);
      const existing = await transaction.getFirstAsync<StoredInboxChange>(
        `SELECT cursor, aggregate_type, aggregate_id, aggregate_version,
          operation, schema_version, payload_json
         FROM change_inbox WHERE owner_id = ? AND event_id = ?`,
        ownerId,
        change.eventId,
      );
      if (existing) {
        if (!sameStoredChange(existing, change, payloadJson)) {
          throw new LocalSyncStateError('event_identity_conflict');
        }
      } else {
        await transaction.runAsync(
          `INSERT INTO change_inbox (
            owner_id, event_id, cursor, aggregate_type, aggregate_id,
            aggregate_version, operation, schema_version, payload_json,
            received_at, applied_at
          ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
          ownerId,
          change.eventId,
          change.cursor,
          change.aggregateType,
          change.aggregateId,
          change.aggregateVersion,
          change.operation,
          change.schemaVersion,
          payloadJson,
          now,
        );
      }
      await applyProjection(transaction, ownerId, change, now);
      await transaction.runAsync(
        `UPDATE change_inbox SET applied_at = ?
         WHERE owner_id = ? AND event_id = ?`,
        now,
        ownerId,
        change.eventId,
      );
    }
    if (page.changes.length > 0 && previousCursor !== page.nextCursor) {
      throw new LocalSyncStateError('next_cursor_invalid');
    }
    await transaction.runAsync(
      `UPDATE sync_state SET cursor = ?, phase = 'ready',
        last_error_code = NULL, updated_at = ?
       WHERE owner_id = ? AND stream = ?`,
      page.nextCursor,
      now,
      ownerId,
      streamName,
    );
  });
}

export async function markLocalSyncError(
  database: SyncStoreDatabase,
  ownerId: string,
  errorCode: string,
) {
  requireOwnerId(ownerId);
  if (!/^[a-z0-9][a-z0-9._-]{0,63}$/.test(errorCode)) {
    throw new LocalSyncStateError('invalid_error_code');
  }
  const now = new Date().toISOString();
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync(
      `UPDATE sync_state SET phase = 'error', last_error_code = ?, updated_at = ?
       WHERE owner_id = ? AND stream = ?`,
      errorCode,
      now,
      ownerId,
      streamName,
    );
  });
}
