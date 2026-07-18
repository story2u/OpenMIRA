import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import { afterEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { clearLocalUserDataInDatabase } from '../storage/userData';
import {
  hasSeenTeachingOnboarding,
  setTeachingOnboardingSeen,
} from './signalAppetiteUiState';
import {
  appendSignalAppetiteEvent,
  readPreferenceExamples,
  readSignalAppetiteEvents,
  readTeachingSession,
  type NewSignalAppetiteEvent,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from './signalAppetiteStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '11234567-89ab-cdef-0123-456789abcdef';
const deviceId = '21234567-89ab-cdef-0123-456789abcdef';
const sessionId = '31234567-89ab-cdef-0123-456789abcdef';
const messageId = '41234567-89ab-cdef-0123-456789abcdef';

class TestDatabase implements SignalAppetiteStoreDatabase {
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

  async withExclusiveTransactionAsync(
    task: (transaction: SignalAppetiteStoreExecutor) => Promise<void>,
  ) {
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

function eventId(sequence: number) {
  return `51234567-89ab-4def-8123-${sequence.toString().padStart(12, '0')}`;
}

function event(
  type: NewSignalAppetiteEvent['type'],
  sequence: number,
  payload: unknown,
  overrides: Partial<NewSignalAppetiteEvent> = {},
) {
  return {
    eventId: eventId(sequence),
    ownerId,
    deviceId,
    aggregateId: sessionId,
    aggregateVersion: sequence,
    schemaVersion: 1,
    occurredAt: `2026-07-18T12:00:${sequence.toString().padStart(2, '0')}.000Z`,
    type,
    payload,
    ...overrides,
  } as NewSignalAppetiteEvent;
}

describe('Signal Appetite SQLite event store', () => {
  const directories: string[] = [];

  afterEach(() => {
    for (const directory of directories.splice(0)) {
      rmSync(directory, { force: true, recursive: true });
    }
  });

  async function open() {
    const directory = mkdtempSync(join(tmpdir(), 'radar-attention-'));
    directories.push(directory);
    const path = join(directory, 'radar.db');
    const database = new TestDatabase(path);
    await runRadarMigrations(database as unknown as MigrationDatabase);
    return { database, path };
  }

  it('persists teaching examples idempotently and folds a reversible session projection', async () => {
    const { database, path } = await open();
    const started = event('TeachingSessionStarted', 1, { sessionId, targetCount: 8 });
    const presented = event('TeachingCardPresented', 2, {
      sessionId,
      messageId,
      selectionScore: 0.87,
    });
    const exampleId = '61234567-89ab-cdef-0123-456789abcdef';
    const captured = event('PreferenceExampleCaptured', 3, {
      example: {
        id: exampleId,
        messageId,
        label: 'positive',
        selectedReasons: ['needs_reply'],
        freeformReason: null,
        capturedAt: '2026-07-18T12:00:03.000Z',
        teachingSessionId: sessionId,
        revertedAt: null,
      },
    });
    await appendSignalAppetiteEvent(database, started);
    await appendSignalAppetiteEvent(database, presented);
    await appendSignalAppetiteEvent(database, captured);
    await appendSignalAppetiteEvent(database, captured);

    expect(await readTeachingSession(database, ownerId, sessionId)).toMatchObject({
      status: 'active',
      presentedCount: 1,
      positiveCount: 1,
    });
    expect(await readPreferenceExamples(database, ownerId, sessionId)).toEqual([
      expect.objectContaining({ id: exampleId, label: 'positive', revertedAt: null }),
    ]);
    expect(await readSignalAppetiteEvents(database, ownerId)).toHaveLength(3);
    expect(database.raw.prepare(
      "SELECT COUNT(*) AS count FROM attention_preferences WHERE owner_id = ? AND status = 'active'",
    ).get(ownerId)).toMatchObject({ count: 0 });

    await appendSignalAppetiteEvent(database, event('PreferenceExampleReverted', 4, {
      exampleId,
      revertedAt: '2026-07-18T12:00:04.000Z',
    }));
    expect(await readTeachingSession(database, ownerId, sessionId)).toMatchObject({ positiveCount: 0 });
    expect((await readPreferenceExamples(database, ownerId, sessionId))[0]?.revertedAt).not.toBeNull();

    database.close();
    const reopened = new TestDatabase(path);
    await runRadarMigrations(reopened as unknown as MigrationDatabase);
    expect(await readSignalAppetiteEvents(reopened, ownerId)).toHaveLength(4);
    reopened.close();
  });

  it('isolates owners and removes the event log and projections on account cleanup', async () => {
    const { database } = await open();
    await appendSignalAppetiteEvent(database, event('TeachingSessionStarted', 1, {
      sessionId,
      targetCount: 5,
    }));

    expect(await readSignalAppetiteEvents(database, otherOwnerId)).toEqual([]);
    expect(await readTeachingSession(database, otherOwnerId, sessionId)).toBeNull();
    await setTeachingOnboardingSeen(database, ownerId, true);
    expect(await hasSeenTeachingOnboarding(database, ownerId)).toBe(true);
    expect(await hasSeenTeachingOnboarding(database, otherOwnerId)).toBe(false);
    await clearLocalUserDataInDatabase(database, ownerId);
    expect(await readSignalAppetiteEvents(database, ownerId)).toEqual([]);
    expect(await readTeachingSession(database, ownerId, sessionId)).toBeNull();
    expect(await hasSeenTeachingOnboarding(database, ownerId)).toBe(false);
    database.close();
  });

  it('rolls back an invalid projection together with its event', async () => {
    const { database } = await open();
    await expect(appendSignalAppetiteEvent(database, event('TeachingSessionStarted', 1, {
      sessionId,
      targetCount: 99,
    }))).rejects.toThrow();
    expect(await readSignalAppetiteEvents(database, ownerId)).toEqual([]);
    database.close();
  });
});
