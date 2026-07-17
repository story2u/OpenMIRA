import { DatabaseSync } from 'node:sqlite';

import { RadarApiError } from '@story2u/radar-api/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import {
  drainInternalCommandOutbox,
  enqueueOpportunityStatusCommand,
  InternalCommandQueueError,
  readCommandOutboxSummary,
  type InternalCommandTransport,
} from './commandOutbox';
import type { SyncStoreDatabase, SyncStoreExecutor } from './syncStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '11234567-89ab-cdef-0123-456789abcdef';
const commandId = '21234567-89ab-cdef-0123-456789abcdef';
const now = new Date('2026-07-17T10:00:00.000Z');

class TestDatabase implements SyncStoreDatabase {
  readonly raw = new DatabaseSync(':memory:');

  async execAsync(source: string) {
    this.raw.exec(source);
  }

  async runAsync(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).run(...params);
  }

  async getAllAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).all(...params) as Row[];
  }

  async getFirstAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return (this.raw.prepare(source).get(...params) as Row | undefined) ?? null;
  }

  async withExclusiveTransactionAsync(task: (transaction: SyncStoreExecutor) => Promise<void>) {
    this.raw.exec('BEGIN EXCLUSIVE');
    try {
      await task(this);
      this.raw.exec('COMMIT');
    } catch (error) {
      this.raw.exec('ROLLBACK');
      throw error;
    }
  }
}

function queueInput(overrides: Partial<Parameters<typeof enqueueOpportunityStatusCommand>[1]> = {}) {
  return {
    ownerId,
    opportunityId,
    status: 'following' as const,
    commandId,
    idempotencyKey: 'status-command-001',
    expiresAt: '2026-07-24T10:00:00.000Z',
    ...overrides,
  };
}

describe('internal command outbox', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    database.raw.prepare(
      `INSERT INTO sync_state (
        owner_id, stream, cursor, updated_at, phase, last_error_code
      ) VALUES (?, 'main', 7, ?, 'ready', NULL)`,
    ).run(ownerId, now.toISOString());
    database.raw.prepare(
      `INSERT INTO opportunity_projection (
        owner_id, id, aggregate_version, payload_json, updated_at,
        deleted_at, archived_at
      ) VALUES (?, ?, 3, '{}', ?, NULL, NULL)`,
    ).run(ownerId, opportunityId, now.toISOString());
  });

  it('queues one bounded internal status command with local base version and expiry', async () => {
    const command = await enqueueOpportunityStatusCommand(database, queueInput(), now);

    expect(command).toMatchObject({
      id: commandId,
      expectedVersion: 3,
      status: 'following',
      attemptCount: 0,
    });
    await expect(readCommandOutboxSummary(database, ownerId)).resolves.toEqual({
      pendingCount: 1,
      conflictCount: 0,
      failedCount: 0,
      attentionCommands: [],
    });
    await expect(enqueueOpportunityStatusCommand(
      database,
      queueInput({ commandId: '31234567-89ab-cdef-0123-456789abcdef' }),
      now,
    )).resolves.toMatchObject({ id: commandId, status: 'following' });
    await expect(enqueueOpportunityStatusCommand(
      database,
      queueInput({
        commandId: '31234567-89ab-cdef-0123-456789abcdef',
        status: 'closed',
      }),
      now,
    )).rejects.toMatchObject({ code: 'command_already_queued' });
  });

  it('replays successfully once and marks the durable command succeeded', async () => {
    await enqueueOpportunityStatusCommand(database, queueInput(), now);
    const updateOpportunityStatus = vi.fn(async () => undefined);

    const result = await drainInternalCommandOutbox(
      database,
      { updateOpportunityStatus },
      ownerId,
      undefined,
      () => now,
    );

    expect(updateOpportunityStatus).toHaveBeenCalledWith(
      expect.objectContaining({
        idempotencyKey: 'status-command-001',
        expectedVersion: 3,
      }),
      undefined,
    );
    expect(result).toEqual({
      succeededCount: 1,
      pendingCount: 0,
      conflictCount: 0,
      failedCount: 0,
      attentionCommands: [],
    });
  });

  it('makes a pre-send local version conflict visible without contacting the server', async () => {
    await enqueueOpportunityStatusCommand(database, queueInput(), now);
    database.raw.prepare(
      'UPDATE opportunity_projection SET aggregate_version = 4 WHERE owner_id = ? AND id = ?',
    ).run(ownerId, opportunityId);
    const updateOpportunityStatus = vi.fn(async () => undefined);

    const result = await drainInternalCommandOutbox(
      database,
      { updateOpportunityStatus },
      ownerId,
      undefined,
      () => now,
    );

    expect(updateOpportunityStatus).not.toHaveBeenCalled();
    expect(result.conflictCount).toBe(1);
    expect(result.pendingCount).toBe(0);

    await enqueueOpportunityStatusCommand(
      database,
      queueInput({
        commandId: '31234567-89ab-cdef-0123-456789abcdef',
        idempotencyKey: 'status-command-002',
      }),
      now,
    );
    await expect(readCommandOutboxSummary(database, ownerId)).resolves.toMatchObject({
      conflictCount: 0,
      pendingCount: 1,
    });
  });

  it('keeps an uncertain replay idempotent and maps server 409 to a visible conflict', async () => {
    await enqueueOpportunityStatusCommand(database, queueInput(), now);
    const transient: InternalCommandTransport = {
      updateOpportunityStatus: vi.fn(async () => {
        throw new TypeError('network unavailable');
      }),
    };
    const first = await drainInternalCommandOutbox(
      database,
      transient,
      ownerId,
      undefined,
      () => now,
    );
    expect(first.pendingCount).toBe(1);

    database.raw.prepare(
      'UPDATE opportunity_projection SET aggregate_version = 4 WHERE owner_id = ? AND id = ?',
    ).run(ownerId, opportunityId);
    const retryAt = new Date(now.getTime() + 3_000);
    const conflict: InternalCommandTransport = {
      updateOpportunityStatus: vi.fn(async () => {
        throw new RadarApiError('opportunity version conflict', 409, null);
      }),
    };
    const second = await drainInternalCommandOutbox(
      database,
      conflict,
      ownerId,
      undefined,
      () => retryAt,
    );

    expect(conflict.updateOpportunityStatus).toHaveBeenCalledOnce();
    expect(second.conflictCount).toBe(1);
  });

  it('fails closed when sync is not ready or the command is already expired', async () => {
    database.raw.prepare(
      "UPDATE sync_state SET phase = 'error' WHERE owner_id = ?",
    ).run(ownerId);
    await expect(enqueueOpportunityStatusCommand(database, queueInput(), now)).rejects.toBeInstanceOf(
      InternalCommandQueueError,
    );
    database.raw.prepare(
      "UPDATE sync_state SET phase = 'ready' WHERE owner_id = ?",
    ).run(ownerId);
    await expect(enqueueOpportunityStatusCommand(
      database,
      queueInput({ expiresAt: '2026-07-17T09:59:59.000Z' }),
      now,
    )).rejects.toMatchObject({ code: 'invalid_expiry' });
  });
});
