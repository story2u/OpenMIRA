import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import type {
  SyncBootstrap,
  SyncChange,
  SyncChanges,
} from '@story2u/radar-contracts/sync';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { runOwnerSync, type SyncTransport } from './syncEngine';
import {
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '11234567-89ab-cdef-0123-456789abcdef';
const pageSize = 500;
const totalChanges = 10_000;

class FileTestDatabase implements SyncStoreDatabase {
  readonly raw: DatabaseSync;

  constructor(path: string) {
    this.raw = new DatabaseSync(path);
  }

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

  close() {
    this.raw.close();
  }
}

function stableUuid(namespace: string, value: number) {
  return `${namespace}-0000-4000-8000-${value.toString(16).padStart(12, '0')}`;
}

function change(cursor: number): SyncChange {
  const aggregateId = stableUuid('22222222', cursor);
  return {
    eventId: stableUuid('33333333', cursor),
    cursor,
    aggregateType: 'message',
    aggregateId,
    aggregateVersion: 1,
    operation: 'upsert',
    schemaVersion: 1,
    createdAt: '2026-07-17T10:00:00Z',
    payload: {
      id: aggregateId,
      opportunityId,
      senderName: 'Scale fixture',
      content: `Message ${cursor}`,
      isFromContact: true,
      sentAt: '2026-07-17T10:00:00Z',
      source: null,
    },
  };
}

function bootstrap(): SyncBootstrap {
  return {
    watermarkCursor: 0,
    items: [],
    hasMore: false,
    nextPageToken: null,
  };
}

function changesAfter(after: number): SyncChanges {
  const nextCursor = Math.min(after + pageSize, totalChanges);
  const changes = Array.from(
    { length: nextCursor - after },
    (_, index) => change(after + index + 1),
  );
  return {
    changes,
    nextCursor,
    serverCursor: totalChanges,
    hasMore: nextCursor < totalChanges,
    resetRequired: false,
    resetReason: null,
  };
}

function transport(overrides: Partial<SyncTransport> = {}): SyncTransport {
  return {
    acknowledge: vi.fn(async () => undefined),
    bootstrap: vi.fn(async () => bootstrap()),
    changes: vi.fn(async ({ after }) => changesAfter(after)),
    ...overrides,
  };
}

describe('SQLite sync scale and process recovery', () => {
  const directories: string[] = [];

  afterEach(() => {
    for (const directory of directories.splice(0)) {
      rmSync(directory, { force: true, recursive: true });
    }
  });

  it('resumes 10,000 durable changes after a page failure and database reopen', {
    timeout: 60_000,
  }, async () => {
    const directory = mkdtempSync(join(tmpdir(), 'radar-sync-scale-'));
    directories.push(directory);
    const path = join(directory, 'radar.db');
    let database = new FileTestDatabase(path);
    await runRadarMigrations(database as unknown as MigrationDatabase);

    let requestCount = 0;
    const interrupted = transport({
      changes: vi.fn(async ({ after }) => {
        requestCount += 1;
        if (requestCount === 4) throw new TypeError('fixture network interrupted');
        return changesAfter(after);
      }),
    });

    await expect(runOwnerSync(database, interrupted, ownerId)).rejects.toThrow(
      'fixture network interrupted',
    );
    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 1_500,
      phase: 'ready',
    });
    database.close();

    const startedAt = Date.now();
    database = new FileTestDatabase(path);
    await runRadarMigrations(database as unknown as MigrationDatabase);
    const resumed = transport({
      bootstrap: vi.fn(async () => {
        throw new Error('durable ready state must not bootstrap again');
      }),
    });
    const result = await runOwnerSync(database, resumed, ownerId);
    const elapsedMilliseconds = Date.now() - startedAt;

    expect(result).toMatchObject({
      acknowledged: true,
      bootstrapPages: 0,
      changeCount: 8_500,
      changePages: 17,
      cursor: totalChanges,
      resetCount: 0,
    });
    expect(resumed.bootstrap).not.toHaveBeenCalled();
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS count FROM change_inbox WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ count: totalChanges });
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS count FROM message_projection WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ count: totalChanges });
    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: totalChanges,
      phase: 'ready',
    });
    expect(elapsedMilliseconds).toBeLessThan(30_000);
    database.close();
  });
});
