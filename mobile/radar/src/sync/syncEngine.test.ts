import { DatabaseSync } from 'node:sqlite';

import { RadarApiError } from '@story2u/radar-api/client';
import type {
  SyncBootstrap,
  SyncChanges,
} from '@story2u/radar-contracts/sync';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { runOwnerSync, type SyncTransport } from './syncEngine';
import {
  applyBootstrapPage,
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';

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

function notificationPayload(enabled = true) {
  return {
    newOpportunityEnabled: enabled,
    aiRepliedEnabled: true,
    dailyDigestEnabled: false,
    urgentOnly: false,
  };
}

function bootstrapPage(
  watermarkCursor: number,
  options: { hasMore?: boolean; nextPageToken?: string | null; enabled?: boolean } = {},
): SyncBootstrap {
  return {
    watermarkCursor,
    items: [{
      aggregateType: 'user_notification_preference',
      aggregateId: ownerId,
      aggregateVersion: 1,
      schemaVersion: 1,
      payload: notificationPayload(options.enabled),
    }],
    hasMore: options.hasMore ?? false,
    nextPageToken: options.nextPageToken ?? null,
  };
}

function emptyChanges(after: number): SyncChanges {
  return {
    changes: [],
    nextCursor: after,
    serverCursor: after,
    hasMore: false,
    resetRequired: false,
    resetReason: null,
  };
}

function transport(overrides: Partial<SyncTransport> = {}): SyncTransport {
  return {
    acknowledge: vi.fn(async () => undefined),
    bootstrap: vi.fn(async () => bootstrapPage(10)),
    changes: vi.fn(async ({ after }) => emptyChanges(after)),
    ...overrides,
  };
}

describe('owner sync engine', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
  });

  it('bootstraps, applies all change pages and acknowledges the durable cursor', async () => {
    const api = transport({
      changes: vi.fn(async ({ after }) => {
        if (after !== 10) return emptyChanges(after);
        return {
          changes: [{
            eventId: '11234567-89ab-cdef-0123-456789abcdef',
            cursor: 11,
            aggregateType: 'user_notification_preference',
            aggregateId: ownerId,
            aggregateVersion: 2,
            operation: 'upsert',
            schemaVersion: 1,
            createdAt: '2026-07-17T10:00:00Z',
            payload: notificationPayload(false),
          }],
          nextCursor: 11,
          serverCursor: 11,
          hasMore: false,
          resetRequired: false,
          resetReason: null,
        } as SyncChanges;
      }),
    });

    const result = await runOwnerSync(database, api, ownerId);

    expect(result).toMatchObject({ cursor: 11, bootstrapPages: 1, changeCount: 1 });
    expect(api.acknowledge).toHaveBeenCalledWith({ cursor: 11 }, undefined);
    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 11,
      phase: 'ready',
    });
  });

  it('resumes a persisted bootstrap continuation after process restart', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrapPage(10, { hasMore: true, nextPageToken: 'resume.token.value' }),
      { restart: true },
    );
    const bootstrap = vi.fn(async ({ pageToken }) => {
      expect(pageToken).toBe('resume.token.value');
      return bootstrapPage(10, { enabled: false });
    });
    const api = transport({ bootstrap });

    const result = await runOwnerSync(database, api, ownerId);

    expect(result.bootstrapPages).toBe(1);
    expect(bootstrap).toHaveBeenCalledTimes(1);
    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 10,
      phase: 'ready',
    });
  });

  it('restarts only an expired signed continuation and keeps network failures resumable', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrapPage(10, { hasMore: true, nextPageToken: 'expired.token.value' }),
      { restart: true },
    );
    const bootstrap = vi.fn(async ({ pageToken }) => {
      if (pageToken) throw new RadarApiError('invalid page', 422, null);
      return bootstrapPage(12);
    });
    const api = transport({ bootstrap });

    const result = await runOwnerSync(database, api, ownerId);

    expect(result.cursor).toBe(12);
    expect(bootstrap).toHaveBeenNthCalledWith(
      1,
      { limit: 500, pageToken: 'expired.token.value' },
      undefined,
    );
    expect(bootstrap).toHaveBeenNthCalledWith(2, { limit: 500 }, undefined);
  });

  it('honors one server reset and does not fail durable sync when ack is offline', async () => {
    await applyBootstrapPage(database, ownerId, bootstrapPage(5), { restart: true });
    const changes = vi.fn(async ({ after }) => {
      if (after === 5) {
        return {
          changes: [],
          nextCursor: 5,
          serverCursor: 10,
          hasMore: false,
          resetRequired: true,
          resetReason: 'cursor_expired',
        } as SyncChanges;
      }
      return emptyChanges(after);
    });
    const api = transport({
      acknowledge: vi.fn(async () => {
        throw new TypeError('offline');
      }),
      bootstrap: vi.fn(async () => bootstrapPage(10)),
      changes,
    });

    const result = await runOwnerSync(database, api, ownerId);

    expect(result).toMatchObject({ cursor: 10, resetCount: 1, acknowledged: false });
    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 10,
      phase: 'ready',
    });
  });
});
