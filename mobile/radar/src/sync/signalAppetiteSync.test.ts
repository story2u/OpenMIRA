import { DatabaseSync } from 'node:sqlite';

import { describe, expect, it } from 'vitest';

import { appendSignalAppetiteEvent } from '../attention/signalAppetiteStore';
import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { runSignalAppetiteSync } from './signalAppetiteSync';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const localDeviceId = '11234567-89ab-cdef-0123-456789abcdef';
const remoteDeviceId = '21234567-89ab-cdef-0123-456789abcdef';
const localSessionId = '31234567-89ab-cdef-0123-456789abcdef';
const remoteSessionId = '41234567-89ab-cdef-0123-456789abcdef';

class TestDatabase {
  readonly raw = new DatabaseSync(':memory:');

  async execAsync(source: string) { this.raw.exec(source); }
  async runAsync(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).run(...params);
  }
  async getAllAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).all(...params) as Row[];
  }
  async getFirstAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return (this.raw.prepare(source).get(...params) as Row | undefined) ?? null;
  }
  async withExclusiveTransactionAsync(task: (transaction: TestDatabase) => Promise<void>) {
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

function syncedEvent(
  eventId: string,
  deviceId: string,
  aggregateId: string,
  cursor: number,
) {
  return {
    eventId,
    eventType: 'TeachingSessionStarted' as const,
    aggregateId,
    aggregateVersion: 1,
    schemaVersion: 1 as const,
    occurredAt: `2026-07-18T12:00:0${cursor}Z`,
    payload: { sessionId: aggregateId, targetCount: 8 },
    ownerId,
    deviceId,
    cursor,
    serverReceivedAt: `2026-07-18T12:00:1${cursor}Z`,
  };
}

describe('Signal Appetite multi-device sync', () => {
  it('pushes pending events, pulls remote events, and advances a separate cursor', async () => {
    const database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    const localId = '51234567-89ab-cdef-0123-456789abcdef';
    await appendSignalAppetiteEvent(database, {
      eventId: localId,
      ownerId,
      deviceId: localDeviceId,
      aggregateId: localSessionId,
      aggregateVersion: 1,
      schemaVersion: 1,
      occurredAt: '2026-07-18T12:00:01Z',
      type: 'TeachingSessionStarted',
      payload: { sessionId: localSessionId, targetCount: 8 },
    });
    const local = syncedEvent(localId, localDeviceId, localSessionId, 1);
    const remote = syncedEvent(
      '61234567-89ab-cdef-0123-456789abcdef',
      remoteDeviceId,
      remoteSessionId,
      2,
    );
    let appended = 0;

    const result = await runSignalAppetiteSync(database, {
      append: async (input) => {
        appended += input.events.length;
        return { events: [local], serverCursor: 1 };
      },
      list: async () => ({
        events: [local, remote], nextCursor: 2, serverCursor: 2, hasMore: false,
      }),
    }, ownerId);

    expect(result).toEqual({ pushed: 1, pulled: 2, cursor: 2 });
    expect(appended).toBe(1);
    expect(database.raw.prepare(
      "SELECT COUNT(*) AS count FROM attention_events WHERE owner_id = ? AND sync_status = 'synced'",
    ).get(ownerId)).toEqual({ count: 2 });
    expect(database.raw.prepare(
      "SELECT COUNT(*) AS count FROM teaching_sessions WHERE owner_id = ?",
    ).get(ownerId)).toEqual({ count: 2 });
    expect(database.raw.prepare(
      "SELECT cursor FROM sync_state WHERE owner_id = ? AND stream = 'signal_appetite'",
    ).get(ownerId)).toEqual({ cursor: 2 });
  });

  it('rejects an event from another owner before projection', async () => {
    const database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    const remote = {
      ...syncedEvent(
        '71234567-89ab-cdef-0123-456789abcdef',
        remoteDeviceId,
        remoteSessionId,
        1,
      ),
      ownerId: '81234567-89ab-cdef-0123-456789abcdef',
    };
    await expect(runSignalAppetiteSync(database, {
      append: async () => ({ events: [], serverCursor: 0 }),
      list: async () => ({
        events: [remote], nextCursor: 1, serverCursor: 1, hasMore: false,
      }),
    }, ownerId)).rejects.toThrow('owner mismatch');
    expect(database.raw.prepare('SELECT COUNT(*) AS count FROM attention_events').get())
      .toEqual({ count: 0 });
  });
});
