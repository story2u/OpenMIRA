import { DatabaseSync } from 'node:sqlite';

import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('expo-crypto', () => ({ randomUUID: () => '00000000-0000-4000-8000-000000000001' }));

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import {
  readPreferenceExamples,
  readTeachingSession,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from './signalAppetiteStore';
import {
  captureTeachingExample,
  completeTeachingSession,
  loadTeachingCandidates,
  startTeachingSession,
  undoTeachingExamples,
} from './teachingService';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const deviceId = '11234567-89ab-cdef-0123-456789abcdef';

class TestDatabase implements SignalAppetiteStoreDatabase {
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
  async withExclusiveTransactionAsync(task: (transaction: SignalAppetiteStoreExecutor) => Promise<void>) {
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

function uuid(value: number) {
  return `21234567-89ab-4def-8123-${value.toString(16).padStart(12, '0')}`;
}

function ids(start = 100) {
  let value = start;
  return () => uuid(value++);
}

describe('teaching service', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    database.raw.prepare(
      `INSERT INTO sync_state (owner_id, stream, cursor, updated_at, phase)
       VALUES (?, 'business', 10, '2026-07-18T10:00:00.000Z', 'ready')`,
    ).run(ownerId);
    for (let index = 0; index < 8; index += 1) {
      const opportunityId = uuid(index + 1);
      const messageId = uuid(index + 20);
      const categoryText = index % 3 === 0
        ? 'Remote AI job with salary'
        : index % 3 === 1
          ? 'Training promotion'
          : 'Customer asks for a quote';
      const opportunity = {
        id: opportunityId,
        platform: 'telegram',
        contactName: `Contact ${index}`,
        summary: categoryText,
        matchedKeywords: index % 3 === 0 ? ['ai_jobs', 'remote'] : [`topic_${index}`],
        confidenceScore: 0.48 + index * 0.01,
        sourceType: 'group',
        groupName: `Group ${index}`,
        attentionRequired: index === 2,
      };
      const message = {
        id: messageId,
        opportunityId,
        senderName: `Sender ${index}`,
        content: `${categoryText} https://example.com/${index}`,
        isFromContact: true,
        sentAt: `2026-07-18T10:0${index}:00.000Z`,
        source: null,
      };
      database.raw.prepare(
        `INSERT INTO opportunity_projection (
          owner_id, id, aggregate_version, payload_json, updated_at, source_type
        ) VALUES (?, ?, 1, ?, '2026-07-18T10:00:00.000Z', 'group')`,
      ).run(ownerId, opportunityId, JSON.stringify(opportunity));
      database.raw.prepare(
        `INSERT INTO message_projection (
          owner_id, id, opportunity_id, aggregate_version, sent_at, payload_json, updated_at
        ) VALUES (?, ?, ?, 1, ?, ?, '2026-07-18T10:00:00.000Z')`,
      ).run(ownerId, messageId, opportunityId, message.sentAt, JSON.stringify(message));
    }
  });

  it('builds privacy-bounded cards from synchronized messages and selects an active-learning deck', async () => {
    const candidates = await loadTeachingCandidates(database, ownerId);
    expect(candidates).toHaveLength(8);
    expect(candidates[0]).toMatchObject({
      platform: 'telegram',
      conversationKind: 'group',
      hasLink: true,
      piUncertain: true,
    });
    expect(candidates[0]).not.toHaveProperty('payload_json');

    const result = await startTeachingSession(database, {
      ownerId,
      deviceId,
      targetCount: 8,
      now: new Date('2026-07-18T11:00:00.000Z'),
      createId: ids(),
    });
    expect(result.cards).toHaveLength(8);
    expect(new Set(result.cards.map((card) => card.sourceKey)).size).toBe(8);
    expect(await readTeachingSession(database, ownerId, result.sessionId)).toMatchObject({
      presentedCount: 8,
      positiveCount: 0,
      negativeCount: 0,
    });
  });

  it('captures positive, negative and skipped examples, supports continuous undo, then summarizes', async () => {
    const createId = ids(300);
    const session = await startTeachingSession(database, {
      ownerId,
      deviceId,
      targetCount: 5,
      createId,
    });
    const [first, second, third] = session.cards;
    if (!first || !second || !third) throw new Error('missing cards');
    await captureTeachingExample(database, {
      ownerId, deviceId, sessionId: session.sessionId, messageId: first.messageId,
      label: 'positive', reasons: ['needs_reply'], createId,
    });
    await captureTeachingExample(database, {
      ownerId, deviceId, sessionId: session.sessionId, messageId: second.messageId,
      label: 'negative', reasons: ['training'], createId,
    });
    await captureTeachingExample(database, {
      ownerId, deviceId, sessionId: session.sessionId, messageId: third.messageId,
      label: 'skipped', createId,
    });
    expect(await readTeachingSession(database, ownerId, session.sessionId)).toMatchObject({
      positiveCount: 1, negativeCount: 1, skippedCount: 1,
    });

    expect(await undoTeachingExamples(database, {
      ownerId, deviceId, sessionId: session.sessionId, count: 2, createId,
    })).toHaveLength(2);
    expect(await readTeachingSession(database, ownerId, session.sessionId)).toMatchObject({
      positiveCount: 1, negativeCount: 0, skippedCount: 0,
    });
    const summary = await completeTeachingSession(database, {
      ownerId, deviceId, sessionId: session.sessionId, createId,
    });
    expect(summary).toMatchObject({
      increase: ['needs_reply'], reduce: [], positiveCount: 1, negativeCount: 0,
    });
    expect((await readPreferenceExamples(database, ownerId, session.sessionId)))
      .toHaveLength(3);
    expect(await readTeachingSession(database, ownerId, session.sessionId)).toMatchObject({
      status: 'summarized',
    });
  });

  it('refuses to teach before the synchronized local projection is ready', async () => {
    database.raw.prepare(
      `UPDATE sync_state SET phase = 'error' WHERE owner_id = ? AND stream = 'business'`,
    ).run(ownerId);
    await expect(loadTeachingCandidates(database, ownerId)).rejects.toThrow('not_ready');
  });
});
