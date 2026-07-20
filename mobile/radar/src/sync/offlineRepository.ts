import type { MessagePage } from '@story2u/radar-contracts/messages';
import type {
  Dashboard,
  DashboardQuery,
  OpportunityDetail,
} from '@story2u/radar-contracts/opportunities';
import type { SettingsBundle } from '@story2u/radar-contracts/settings';
import { decodeSyncMessagePayload } from '@story2u/radar-api/sync';
import {
  dashboardRequestPath,
  decodeOpportunityDetailResponse,
} from '@story2u/radar-api/opportunities';
import {
  decodeDetectionSettings,
  decodeNotificationSettings,
  decodeWorkSchedule,
} from '@story2u/radar-api/settings';

import {
  markLocalSyncError,
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

export class OfflineProjectionUnavailableError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'OfflineProjectionUnavailableError';
  }
}

export class LocalProjectionCorruptError extends Error {
  constructor(readonly aggregateType: string) {
    super(`corrupt_${aggregateType}`);
    this.name = 'LocalProjectionCorruptError';
  }
}

export async function readOfflineProjectionWithRecovery<T>(
  database: SyncStoreDatabase,
  ownerId: string,
  read: () => Promise<T>,
) {
  try {
    return await read();
  } catch (error) {
    if (error instanceof LocalProjectionCorruptError) {
      await markLocalSyncError(database, ownerId, 'projection_corrupt');
    }
    throw error;
  }
}

interface ProjectionRow {
  payload_json: string;
}

async function requireReady(database: SyncStoreExecutor, ownerId: string) {
  const state = await readLocalSyncState(database, ownerId);
  if (!state || state.phase !== 'ready') {
    throw new OfflineProjectionUnavailableError(state?.phase ?? 'not_bootstrapped');
  }
  return state;
}

function decodeOpportunity(row: ProjectionRow): OpportunityDetail {
  try {
    const payload = JSON.parse(row.payload_json);
    if (payload && typeof payload === 'object' && !Array.isArray(payload) && !('opportunityType' in payload)) {
      payload.opportunityType = 'business';
    }
    return decodeOpportunityDetailResponse(payload);
  } catch {
    throw new LocalProjectionCorruptError('opportunity');
  }
}

function placeholders(values: readonly unknown[]) {
  return values.map(() => '?').join(', ');
}

const trustRanges: Readonly<Record<string, readonly [number, number]>> = {
  trusted: [80, 100],
  unverified: [60, 79],
  suspicious: [40, 59],
  risky: [0, 39],
} as const;

function dashboardWhere(ownerId: string, query: DashboardQuery) {
  const clauses = [
    'op.owner_id = ?',
    'op.deleted_at IS NULL',
    'op.archived_at IS NULL',
  ];
  const params: Array<string | number | null> = [ownerId];
  if (query.status) {
    clauses.push('op.frontend_status = ?');
    params.push(query.status);
  }
  if (query.platform) {
    clauses.push('op.platform = ?');
    params.push(query.platform);
  }
  if (query.source_type) {
    clauses.push('op.source_type = ?');
    params.push(query.source_type);
  }
  if (query.created_from) {
    clauses.push('op.created_at >= ?');
    params.push(query.created_from);
  }
  if (query.created_to) {
    clauses.push('op.created_at <= ?');
    params.push(query.created_to);
  }
  if (query.trust_levels?.length) {
    const ranges = query.trust_levels.map((level) => trustRanges[level]);
    clauses.push(`(${ranges.map(() => '(op.trust_score BETWEEN ? AND ?)').join(' OR ')})`);
    ranges.forEach(([minimum, maximum]) => params.push(minimum, maximum));
  }
  if (query.sop_stages?.length) {
    clauses.push(`op.sop_stage IN (${placeholders(query.sop_stages)})`);
    params.push(...query.sop_stages);
  }
  if (query.keywords?.length) {
    clauses.push(
      `EXISTS (
        SELECT 1 FROM json_each(json_extract(op.payload_json, '$.matchedKeywords')) AS keyword
        WHERE CAST(keyword.value AS TEXT) IN (${placeholders(query.keywords)})
      )`,
    );
    params.push(...query.keywords);
  }
  return { clause: clauses.join(' AND '), params };
}

const dashboardSort: Readonly<Record<string, string>> = {
  newest: 'op.created_at DESC, op.id DESC',
  oldest: 'op.created_at ASC, op.id DESC',
  confidence: 'op.confidence_score DESC, op.id DESC',
  trust: 'op.trust_score DESC, op.id DESC',
} as const;

export async function readOfflineDashboard(
  database: SyncStoreExecutor,
  ownerId: string,
  query: DashboardQuery = {},
): Promise<Dashboard> {
  await requireReady(database, ownerId);
  dashboardRequestPath(query);
  const limit = query.limit ?? 20;
  const offset = query.offset ?? 0;
  const where = dashboardWhere(ownerId, query);
  const orderBy = dashboardSort[query.sort ?? 'newest'] ?? dashboardSort.newest;
  const [rows, countRow, pendingRow, attentionRows, keywordRows] = await Promise.all([
    database.getAllAsync<ProjectionRow>(
      `SELECT op.payload_json FROM opportunity_projection AS op
       WHERE ${where.clause}
       ORDER BY ${orderBy} LIMIT ? OFFSET ?`,
      ...where.params,
      limit,
      offset,
    ),
    database.getFirstAsync<{ total: number }>(
      `SELECT COUNT(*) AS total FROM opportunity_projection AS op
       WHERE ${where.clause}`,
      ...where.params,
    ),
    database.getFirstAsync<{ total: number }>(
      `SELECT COUNT(*) AS total FROM opportunity_projection AS op
       WHERE op.owner_id = ? AND op.deleted_at IS NULL AND op.archived_at IS NULL
         AND op.frontend_status = 'pending'`,
      ownerId,
    ),
    database.getAllAsync<ProjectionRow>(
      `SELECT op.payload_json FROM opportunity_projection AS op
       WHERE op.owner_id = ? AND op.deleted_at IS NULL AND op.archived_at IS NULL
         AND op.frontend_status = 'pending' AND op.attention_required = 1
       ORDER BY op.created_at DESC, op.id DESC LIMIT 20`,
      ownerId,
    ),
    database.getAllAsync<{ keyword: string }>(
      `SELECT DISTINCT CAST(keyword.value AS TEXT) AS keyword
       FROM opportunity_projection AS op,
         json_each(json_extract(op.payload_json, '$.matchedKeywords')) AS keyword
       WHERE op.owner_id = ? AND op.deleted_at IS NULL AND op.archived_at IS NULL
       ORDER BY keyword LIMIT 256`,
      ownerId,
    ),
  ]);
  return {
    items: rows.map(decodeOpportunity),
    total: countRow?.total ?? 0,
    limit,
    offset,
    pendingCount: pendingRow?.total ?? 0,
    attentionItems: attentionRows.map(decodeOpportunity),
    keywordOptions: keywordRows.map((row) => row.keyword),
  };
}

export async function readOfflineOpportunityDetail(
  database: SyncStoreExecutor,
  ownerId: string,
  opportunityId: string,
) {
  await requireReady(database, ownerId);
  if (!/^[0-9a-fA-F-]{36}$/.test(opportunityId)) {
    throw new OfflineProjectionUnavailableError('invalid_opportunity');
  }
  const row = await database.getFirstAsync<ProjectionRow>(
    `SELECT payload_json FROM opportunity_projection
     WHERE owner_id = ? AND id = ? AND deleted_at IS NULL`,
    ownerId,
    opportunityId,
  );
  if (!row) return null;
  const detail = decodeOpportunity(row);
  if (detail.id !== opportunityId) throw new LocalProjectionCorruptError('opportunity');
  return detail;
}

export async function searchOfflineOpportunities(
  database: SyncStoreExecutor,
  ownerId: string,
  query: string,
  limit = 10,
): Promise<OpportunityDetail[]> {
  await requireReady(database, ownerId);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (!normalizedQuery || normalizedQuery.length > 100) {
    throw new OfflineProjectionUnavailableError('invalid_opportunity_search');
  }
  if (!Number.isInteger(limit) || limit < 1 || limit > 20) {
    throw new OfflineProjectionUnavailableError('invalid_opportunity_search_limit');
  }
  const rows = await database.getAllAsync<ProjectionRow>(
    `SELECT payload_json FROM opportunity_projection
     WHERE owner_id = ? AND deleted_at IS NULL AND archived_at IS NULL
       AND (
         instr(lower(coalesce(json_extract(payload_json, '$.contactName'), '')), ?) > 0
         OR instr(lower(coalesce(json_extract(payload_json, '$.summary'), '')), ?) > 0
         OR instr(lower(coalesce(json_extract(payload_json, '$.lastMessagePreview'), '')), ?) > 0
         OR instr(lower(coalesce(json_extract(payload_json, '$.matchedKeywords'), '')), ?) > 0
       )
     ORDER BY updated_at DESC, id DESC LIMIT ?`,
    ownerId,
    normalizedQuery,
    normalizedQuery,
    normalizedQuery,
    normalizedQuery,
    limit,
  );
  return rows.map(decodeOpportunity);
}

export async function readOfflineMessagePage(
  database: SyncStoreExecutor,
  ownerId: string,
  opportunityId: string,
  options: { limit?: number; offset?: number } = {},
): Promise<MessagePage> {
  await requireReady(database, ownerId);
  const limit = options.limit ?? 20;
  const offset = options.offset ?? 0;
  if (!Number.isInteger(limit) || limit < 1 || limit > 200) {
    throw new OfflineProjectionUnavailableError('invalid_message_limit');
  }
  if (!Number.isInteger(offset) || offset < 0) {
    throw new OfflineProjectionUnavailableError('invalid_message_offset');
  }
  const [rows, countRow] = await Promise.all([
    database.getAllAsync<ProjectionRow>(
      `SELECT payload_json FROM message_projection
       WHERE owner_id = ? AND opportunity_id = ? AND deleted_at IS NULL
       ORDER BY sent_at ASC, id ASC LIMIT ? OFFSET ?`,
      ownerId,
      opportunityId,
      limit,
      offset,
    ),
    database.getFirstAsync<{ total: number }>(
      `SELECT COUNT(*) AS total FROM message_projection
       WHERE owner_id = ? AND opportunity_id = ? AND deleted_at IS NULL`,
      ownerId,
      opportunityId,
    ),
  ]);
  try {
    return {
      items: rows.map((row) => {
        const { opportunityId: payloadOpportunityId, ...chatMessage } =
          decodeSyncMessagePayload(JSON.parse(row.payload_json));
        if (payloadOpportunityId !== opportunityId) {
          throw new LocalProjectionCorruptError('message');
        }
        return chatMessage;
      }),
      total: countRow?.total ?? 0,
      limit,
      offset,
    };
  } catch (error) {
    if (error instanceof LocalProjectionCorruptError) throw error;
    throw new LocalProjectionCorruptError('message');
  }
}

export async function readOfflineSettings(
  database: SyncStoreExecutor,
  ownerId: string,
): Promise<SettingsBundle> {
  await requireReady(database, ownerId);
  const rows = await database.getAllAsync<{ setting_type: string; payload_json: string }>(
    `SELECT setting_type, payload_json FROM setting_projection
     WHERE owner_id = ? AND deleted_at IS NULL`,
    ownerId,
  );
  const values = new Map(rows.map((row) => [row.setting_type, row.payload_json]));
  try {
    const detection = decodeDetectionSettings(JSON.parse(
      values.get('user_detection_preference') ?? 'null',
    ));
    const workSchedule = decodeWorkSchedule(JSON.parse(
      values.get('user_work_schedule') ?? 'null',
    ));
    const notifications = decodeNotificationSettings(JSON.parse(
      values.get('user_notification_preference') ?? 'null',
    ));
    return {
      detection,
      workSchedule,
      notifications,
      capabilities: { pushAvailable: false, wecomUserBindingAvailable: false },
    };
  } catch {
    throw new LocalProjectionCorruptError('settings');
  }
}
