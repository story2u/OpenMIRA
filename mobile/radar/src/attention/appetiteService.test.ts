import { DatabaseSync } from 'node:sqlite';

import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('expo-crypto', () => ({ randomUUID: () => '00000000-0000-4000-8000-000000000001' }));

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import {
  applyPreferenceVersion,
  createTemporaryFocus,
  proposeAppetiteFromTeaching,
  simulatePreferenceVersion,
  startPreferenceShadow,
  undoPreferenceChange,
} from './appetiteService';
import {
  readActivePreference,
  readMessageFilterDecisions,
  readPreferenceIntents,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from './signalAppetiteStore';
import {
  captureTeachingExample,
  completeTeachingSession,
  startTeachingSession,
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
  return `71234567-89ab-4def-8123-${value.toString(16).padStart(12, '0')}`;
}

function ids(start = 100) {
  let value = start;
  return () => uuid(value++);
}

describe('appetite version, simulation and shadow service', () => {
  let database: TestDatabase;
  let createId: () => string;

  beforeEach(async () => {
    database = new TestDatabase();
    createId = ids();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    database.raw.prepare(
      `INSERT INTO sync_state (owner_id, stream, cursor, updated_at, phase)
       VALUES (?, 'business', 10, '2026-07-18T10:00:00.000Z', 'ready')`,
    ).run(ownerId);
    for (let index = 0; index < 6; index += 1) {
      const opportunityId = uuid(index + 1);
      const messageId = uuid(index + 20);
      const opportunity = {
        id: opportunityId,
        platform: 'telegram',
        contactName: `Contact ${index}`,
        summary: index % 2 ? 'Training promotion' : 'Customer needs a reply',
        matchedKeywords: index % 2 ? ['training'] : ['needs_reply'],
        confidenceScore: 0.5,
        sourceType: 'group',
        groupName: `Group ${index}`,
        attentionRequired: index % 2 === 0,
      };
      const message = {
        id: messageId,
        opportunityId,
        senderName: `Sender ${index}`,
        content: opportunity.summary,
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

  async function teachAndPropose(positiveReason: string, negativeReason: string) {
    const session = await startTeachingSession(database, {
      ownerId, deviceId, targetCount: 5, createId,
    });
    const [positive, negative] = session.cards;
    if (!positive || !negative) throw new Error('missing teaching cards');
    await captureTeachingExample(database, {
      ownerId, deviceId, sessionId: session.sessionId, messageId: positive.messageId,
      label: 'positive', reasons: [positiveReason], createId,
    });
    await captureTeachingExample(database, {
      ownerId, deviceId, sessionId: session.sessionId, messageId: negative.messageId,
      label: 'negative', reasons: [negativeReason], createId,
    });
    await completeTeachingSession(database, {
      ownerId, deviceId, sessionId: session.sessionId, createId,
    });
    return proposeAppetiteFromTeaching(database, {
      ownerId, deviceId, sessionId: session.sessionId, createId,
    });
  }

  it('does not change the active version before simulation and explicit confirmation', async () => {
    const proposal = await teachAndPropose('needs_reply', 'training');
    expect(await readActivePreference(database, ownerId)).toBeNull();
    expect(await readPreferenceIntents(
      database, ownerId, proposal.preference.id, proposal.preference.version,
    )).toHaveLength(2);

    const simulation = await simulatePreferenceVersion(database, {
      ownerId, deviceId, version: proposal.preference.version, createId,
    });
    expect(simulation.originalCount).toBe(6);
    await expect(applyPreferenceVersion(database, {
      ownerId, deviceId, version: proposal.preference.version, confirmed: false, createId,
    })).rejects.toThrow('confirmation_required');
    expect(await readActivePreference(database, ownerId)).toBeNull();

    await applyPreferenceVersion(database, {
      ownerId, deviceId, version: proposal.preference.version, confirmed: true, createId,
    });
    expect(await readActivePreference(database, ownerId)).toMatchObject({ version: 1, status: 'active' });
    expect(await readMessageFilterDecisions(database, ownerId)).toHaveLength(6);
  });

  it('runs a candidate in shadow mode, supports temporary focus, and rolls back versions', async () => {
    const first = await teachAndPropose('needs_reply', 'training');
    await simulatePreferenceVersion(database, { ownerId, deviceId, version: 1, createId });
    await applyPreferenceVersion(database, {
      ownerId, deviceId, version: first.preference.version, confirmed: true, createId,
    });
    const second = await teachAndPropose('suitable_job', 'advertising');
    await simulatePreferenceVersion(database, {
      ownerId, deviceId, version: second.preference.version, createId,
    });
    const shadow = await startPreferenceShadow(database, {
      ownerId, deviceId, candidateVersion: second.preference.version,
      durationHours: 24, now: new Date('2026-07-18T12:00:00.000Z'), createId,
    });
    expect(shadow).toMatchObject({ oldVersion: 1, candidateVersion: 2, status: 'running' });
    expect(Date.parse(shadow.endsAt) - Date.parse(shadow.startedAt)).toBe(24 * 60 * 60 * 1_000);
    const focus = await createTemporaryFocus(database, {
      ownerId, deviceId, concept: 'launch_week', durationHours: 48, createId,
    });
    expect(focus).toMatchObject({ concept: 'launch_week', deliveryMode: 'immediate' });

    await applyPreferenceVersion(database, {
      ownerId, deviceId, version: second.preference.version, confirmed: true, createId,
    });
    expect(await readActivePreference(database, ownerId)).toMatchObject({ version: 2 });
    expect(await undoPreferenceChange(database, { ownerId, deviceId, createId })).toMatchObject({ version: 1 });
  });
});
