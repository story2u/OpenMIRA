import { RadarApiError } from '@story2u/radar-api/client';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';

import {
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const supportedStatuses = new Set<InternalOpportunityStatus>([
  'pending_human',
  'ai_auto_reply',
  'replied',
  'following',
  'ignored',
  'closed',
]);
const maxActiveCommands = 100;
const maxCommandsPerDrain = 50;
const staleRunningMilliseconds = 5 * 60 * 1_000;
const maxAttempts = 5;

export class InternalCommandQueueError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'InternalCommandQueueError';
  }
}

export interface InternalStatusCommand {
  id: string;
  opportunityId: string;
  expectedVersion: number;
  idempotencyKey: string;
  status: InternalOpportunityStatus;
  attemptCount: number;
  expiresAt: string;
}

export interface CommandOutboxSummary {
  pendingCount: number;
  conflictCount: number;
  failedCount: number;
  attentionCommands: VisibleInternalCommand[];
}

export interface VisibleInternalCommand {
  id: string;
  opportunityId: string;
  errorCode: string;
  status: InternalOpportunityStatus | null;
}

export interface InternalCommandTransport {
  updateOpportunityStatus(
    command: InternalStatusCommand,
    signal?: AbortSignal,
  ): Promise<unknown>;
}

function requireUuid(value: string, code: string) {
  if (!uuidPattern.test(value)) throw new InternalCommandQueueError(code);
}

function requireIsoFuture(value: string, now: Date) {
  const milliseconds = Date.parse(value);
  if (!Number.isFinite(milliseconds) || milliseconds <= now.getTime()) {
    throw new InternalCommandQueueError('invalid_expiry');
  }
}

export async function enqueueOpportunityStatusCommand(
  database: SyncStoreDatabase,
  input: {
    ownerId: string;
    opportunityId: string;
    status: InternalOpportunityStatus;
    commandId: string;
    idempotencyKey: string;
    expiresAt: string;
  },
  now = new Date(),
): Promise<InternalStatusCommand> {
  requireUuid(input.ownerId, 'invalid_owner');
  requireUuid(input.opportunityId, 'invalid_opportunity');
  requireUuid(input.commandId, 'invalid_command');
  if (!supportedStatuses.has(input.status)) throw new InternalCommandQueueError('invalid_status');
  if (input.idempotencyKey.length < 8 || input.idempotencyKey.length > 128) {
    throw new InternalCommandQueueError('invalid_idempotency_key');
  }
  requireIsoFuture(input.expiresAt, now);

  let command: InternalStatusCommand | null = null;
  await database.withExclusiveTransactionAsync(async (transaction) => {
    const state = await readLocalSyncState(transaction, input.ownerId);
    if (!state || state.phase !== 'ready') {
      throw new InternalCommandQueueError('sync_not_ready');
    }
    await transaction.runAsync(
      `DELETE FROM command_outbox
       WHERE owner_id = ? AND aggregate_type = 'opportunity'
         AND aggregate_id = ? AND status = 'failed' AND next_attempt_at IS NULL`,
      input.ownerId,
      input.opportunityId,
    );
    const existing = await transaction.getFirstAsync<StoredCommandRow>(
      `SELECT id, aggregate_id, expected_version, idempotency_key,
        payload_json, attempt_count, expires_at
       FROM command_outbox
       WHERE owner_id = ? AND aggregate_type = 'opportunity'
         AND aggregate_id = ?
         AND (status IN ('pending', 'running')
           OR (status = 'failed' AND next_attempt_at IS NOT NULL))
       ORDER BY created_at, id LIMIT 1`,
      input.ownerId,
      input.opportunityId,
    );
    if (existing) {
      const existingCommand = decodeStoredCommand(existing);
      if (existingCommand.status === input.status) {
        command = existingCommand;
        return;
      }
      throw new InternalCommandQueueError('command_already_queued');
    }
    const active = await transaction.getFirstAsync<{ total: number }>(
      `SELECT COUNT(*) AS total FROM command_outbox
       WHERE owner_id = ? AND (status IN ('pending', 'running')
         OR (status = 'failed' AND next_attempt_at IS NOT NULL))`,
      input.ownerId,
    );
    if ((active?.total ?? 0) >= maxActiveCommands) {
      throw new InternalCommandQueueError('queue_full');
    }
    const projection = await transaction.getFirstAsync<{
      aggregate_version: number;
      archived_at: string | null;
      deleted_at: string | null;
    }>(
      `SELECT aggregate_version, archived_at, deleted_at
       FROM opportunity_projection WHERE owner_id = ? AND id = ?`,
      input.ownerId,
      input.opportunityId,
    );
    if (!projection || projection.deleted_at || projection.archived_at) {
      throw new InternalCommandQueueError('opportunity_not_queueable');
    }
    const timestamp = now.toISOString();
    await transaction.runAsync(
      `INSERT INTO command_outbox (
        owner_id, id, command_type, aggregate_type, aggregate_id,
        expected_version, idempotency_key, payload_json, status,
        attempt_count, next_attempt_at, last_error_code, created_at,
        updated_at, expires_at
      ) VALUES (?, ?, 'opportunity_status', 'opportunity', ?, ?, ?, ?,
        'pending', 0, NULL, NULL, ?, ?, ?)`,
      input.ownerId,
      input.commandId,
      input.opportunityId,
      projection.aggregate_version,
      input.idempotencyKey,
      JSON.stringify({ status: input.status }),
      timestamp,
      timestamp,
      input.expiresAt,
    );
    command = {
      id: input.commandId,
      opportunityId: input.opportunityId,
      expectedVersion: projection.aggregate_version,
      idempotencyKey: input.idempotencyKey,
      status: input.status,
      attemptCount: 0,
      expiresAt: input.expiresAt,
    };
  });
  if (!command) throw new InternalCommandQueueError('enqueue_failed');
  return command;
}

interface StoredCommandRow {
  id: string;
  aggregate_id: string;
  expected_version: number;
  idempotency_key: string;
  payload_json: string;
  attempt_count: number;
  expires_at: string;
}

function decodeStoredCommand(row: StoredCommandRow): InternalStatusCommand {
  let status: unknown;
  try {
    status = (JSON.parse(row.payload_json) as { status?: unknown }).status;
  } catch {
    throw new InternalCommandQueueError('corrupt_command');
  }
  if (typeof status !== 'string' || !supportedStatuses.has(status as InternalOpportunityStatus)) {
    throw new InternalCommandQueueError('corrupt_command');
  }
  return {
    id: row.id,
    opportunityId: row.aggregate_id,
    expectedVersion: row.expected_version,
    idempotencyKey: row.idempotency_key,
    status: status as InternalOpportunityStatus,
    attemptCount: row.attempt_count,
    expiresAt: row.expires_at,
  };
}

async function prepareOutbox(database: SyncStoreDatabase, ownerId: string, now: Date) {
  await database.withExclusiveTransactionAsync(async (transaction) => {
    const timestamp = now.toISOString();
    const staleBefore = new Date(now.getTime() - staleRunningMilliseconds).toISOString();
    await transaction.runAsync(
      `UPDATE command_outbox SET status = 'failed', next_attempt_at = ?,
        last_error_code = 'interrupted', updated_at = ?
       WHERE owner_id = ? AND status = 'running' AND updated_at <= ?`,
      timestamp,
      timestamp,
      ownerId,
      staleBefore,
    );
    await transaction.runAsync(
      `UPDATE command_outbox SET status = 'failed', next_attempt_at = NULL,
        last_error_code = 'expired', updated_at = ?
       WHERE owner_id = ? AND status NOT IN ('succeeded', 'failed')
         AND expires_at IS NOT NULL AND expires_at <= ?`,
      timestamp,
      ownerId,
      timestamp,
    );
    await transaction.runAsync(
      `UPDATE command_outbox SET next_attempt_at = NULL,
        last_error_code = 'expired', updated_at = ?
       WHERE owner_id = ? AND status = 'failed' AND next_attempt_at IS NOT NULL
         AND expires_at IS NOT NULL AND expires_at <= ?`,
      timestamp,
      ownerId,
      timestamp,
    );
  });
}

async function claimNextCommand(
  database: SyncStoreDatabase,
  ownerId: string,
  now: Date,
): Promise<InternalStatusCommand | null> {
  let command: InternalStatusCommand | null = null;
  await database.withExclusiveTransactionAsync(async (transaction) => {
    const timestamp = now.toISOString();
    const row = await transaction.getFirstAsync<StoredCommandRow>(
      `SELECT id, aggregate_id, expected_version, idempotency_key,
        payload_json, attempt_count, expires_at
       FROM command_outbox
       WHERE owner_id = ? AND command_type = 'opportunity_status'
         AND (status = 'pending'
           OR (status = 'failed' AND next_attempt_at IS NOT NULL
             AND next_attempt_at <= ?))
         AND (expires_at IS NULL OR expires_at > ?)
       ORDER BY created_at, id LIMIT 1`,
      ownerId,
      timestamp,
      timestamp,
    );
    if (!row) return;
    await transaction.runAsync(
      `UPDATE command_outbox SET status = 'running', attempt_count = attempt_count + 1,
        next_attempt_at = NULL, last_error_code = NULL, updated_at = ?
       WHERE owner_id = ? AND id = ?`,
      timestamp,
      ownerId,
      row.id,
    );
    command = decodeStoredCommand({ ...row, attempt_count: row.attempt_count + 1 });
  });
  return command;
}

async function markCommand(
  database: SyncStoreDatabase,
  ownerId: string,
  commandId: string,
  status: 'succeeded' | 'failed',
  errorCode: string | null,
  nextAttemptAt: string | null,
  now: Date,
) {
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync(
      `UPDATE command_outbox SET status = ?, last_error_code = ?,
        next_attempt_at = ?, updated_at = ?
       WHERE owner_id = ? AND id = ? AND status = 'running'`,
      status,
      errorCode,
      nextAttemptAt,
      now.toISOString(),
      ownerId,
      commandId,
    );
  });
}

async function localProjectionVersion(
  database: SyncStoreExecutor,
  ownerId: string,
  opportunityId: string,
) {
  return database.getFirstAsync<{ aggregate_version: number }>(
    `SELECT aggregate_version FROM opportunity_projection
     WHERE owner_id = ? AND id = ? AND deleted_at IS NULL`,
    ownerId,
    opportunityId,
  );
}

export async function readCommandOutboxSummary(
  database: SyncStoreExecutor,
  ownerId: string,
): Promise<CommandOutboxSummary> {
  requireUuid(ownerId, 'invalid_owner');
  const [row, attentionRows] = await Promise.all([
    database.getFirstAsync<{
    pending_count: number;
    conflict_count: number;
    failed_count: number;
    }>(
      `SELECT
       SUM(CASE WHEN status IN ('pending', 'running')
         OR (status = 'failed' AND next_attempt_at IS NOT NULL) THEN 1 ELSE 0 END)
         AS pending_count,
       SUM(CASE WHEN status = 'failed' AND last_error_code = 'version_conflict'
         THEN 1 ELSE 0 END) AS conflict_count,
       SUM(CASE WHEN status = 'failed' AND next_attempt_at IS NULL
         AND last_error_code != 'version_conflict' THEN 1 ELSE 0 END) AS failed_count
     FROM command_outbox WHERE owner_id = ?`,
      ownerId,
    ),
    database.getAllAsync<{
      id: string;
      aggregate_id: string;
      last_error_code: string;
      payload_json: string;
    }>(
      `SELECT id, aggregate_id, last_error_code, payload_json
       FROM command_outbox
       WHERE owner_id = ? AND status = 'failed' AND next_attempt_at IS NULL
       ORDER BY updated_at DESC, id LIMIT 20`,
      ownerId,
    ),
  ]);
  return {
    pendingCount: row?.pending_count ?? 0,
    conflictCount: row?.conflict_count ?? 0,
    failedCount: row?.failed_count ?? 0,
    attentionCommands: attentionRows.map((item) => {
      let status: InternalOpportunityStatus | null = null;
      try {
        const candidate = (JSON.parse(item.payload_json) as { status?: unknown }).status;
        if (typeof candidate === 'string' && supportedStatuses.has(
          candidate as InternalOpportunityStatus,
        )) {
          status = candidate as InternalOpportunityStatus;
        }
      } catch {
        // A corrupt terminal command stays visible and can still be dismissed safely.
      }
      return {
        id: item.id,
        opportunityId: item.aggregate_id,
        errorCode: item.last_error_code,
        status,
      };
    }),
  };
}

export async function dismissTerminalCommand(
  database: SyncStoreDatabase,
  ownerId: string,
  commandId: string,
) {
  requireUuid(ownerId, 'invalid_owner');
  requireUuid(commandId, 'invalid_command');
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync(
      `DELETE FROM command_outbox
       WHERE owner_id = ? AND id = ? AND status = 'failed' AND next_attempt_at IS NULL`,
      ownerId,
      commandId,
    );
  });
  return readCommandOutboxSummary(database, ownerId);
}

export interface DrainCommandResult extends CommandOutboxSummary {
  succeededCount: number;
}

export async function drainInternalCommandOutbox(
  database: SyncStoreDatabase,
  transport: InternalCommandTransport,
  ownerId: string,
  signal?: AbortSignal,
  clock: () => Date = () => new Date(),
): Promise<DrainCommandResult> {
  requireUuid(ownerId, 'invalid_owner');
  await prepareOutbox(database, ownerId, clock());
  let succeededCount = 0;
  for (let index = 0; index < maxCommandsPerDrain; index += 1) {
    if (signal?.aborted) throw new DOMException('Command drain aborted', 'AbortError');
    const now = clock();
    const command = await claimNextCommand(database, ownerId, now);
    if (!command) break;
    const projection = await localProjectionVersion(database, ownerId, command.opportunityId);
    if (command.attemptCount === 1 && projection?.aggregate_version !== command.expectedVersion) {
      await markCommand(
        database,
        ownerId,
        command.id,
        'failed',
        'version_conflict',
        null,
        now,
      );
      continue;
    }
    try {
      await transport.updateOpportunityStatus(command, signal);
      await markCommand(database, ownerId, command.id, 'succeeded', null, null, now);
      succeededCount += 1;
    } catch (error) {
      if (signal?.aborted || (error instanceof Error && error.name === 'AbortError')) {
        await markCommand(
          database,
          ownerId,
          command.id,
          'failed',
          'interrupted',
          now.toISOString(),
          now,
        );
        throw error;
      }
      if (error instanceof RadarApiError && error.status === 409) {
        await markCommand(
          database,
          ownerId,
          command.id,
          'failed',
          'version_conflict',
          null,
          now,
        );
        continue;
      }
      if (error instanceof RadarApiError && error.status >= 400 && error.status < 500) {
        await markCommand(
          database,
          ownerId,
          command.id,
          'failed',
          error.status === 401 ? 'authentication_required' : 'rejected',
          null,
          now,
        );
        break;
      }
      const exhausted = command.attemptCount >= maxAttempts;
      const delaySeconds = Math.min(300, 2 ** command.attemptCount);
      await markCommand(
        database,
        ownerId,
        command.id,
        'failed',
        exhausted ? 'retry_exhausted' : 'transient',
        exhausted ? null : new Date(now.getTime() + delaySeconds * 1_000).toISOString(),
        now,
      );
      break;
    }
  }
  return { ...(await readCommandOutboxSummary(database, ownerId)), succeededCount };
}
