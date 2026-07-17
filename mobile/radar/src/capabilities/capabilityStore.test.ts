import { DatabaseSync } from 'node:sqlite';

import { beforeEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import type { SyncStoreDatabase, SyncStoreExecutor } from '../sync/syncStore';
import {
  readStoredSyncCapability,
  writeStoredSyncCapability,
} from './capabilityStore';

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

describe('client capability store', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
  });

  it('persists the last explicit server decision per owner, including rollback to false', async () => {
    expect(await readStoredSyncCapability(database, ownerId)).toBe(false);

    await writeStoredSyncCapability(database, ownerId, true);
    expect(await readStoredSyncCapability(database, ownerId)).toBe(true);

    await writeStoredSyncCapability(database, ownerId, false);
    expect(await readStoredSyncCapability(database, ownerId)).toBe(false);
  });

  it('rejects invalid owner identifiers before querying SQLite', async () => {
    await expect(readStoredSyncCapability(database, 'unsafe-owner')).rejects.toThrow(
      'invalid capability owner',
    );
  });
});
