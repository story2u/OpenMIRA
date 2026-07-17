import { DatabaseSync } from 'node:sqlite';

import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('expo-crypto', () => ({ randomUUID: () => '00000000-0000-4000-8000-000000000001' }));

import { runRadarMigrations, type MigrationDatabase } from '../../storage/migrations';
import type {
  SignalAppetiteStoreDatabase,
  SignalAppetiteStoreExecutor,
} from '../../attention/signalAppetiteStore';
import {
  executeInteractiveAppetiteTool,
  type InteractiveAppetiteToolCall,
} from './appetiteTools';

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
  return `81234567-89ab-4def-8123-${value.toString(16).padStart(12, '0')}`;
}

describe('interactive Signal Appetite tools', () => {
  let database: TestDatabase;
  let nextId: () => string;

  beforeEach(async () => {
    database = new TestDatabase();
    let id = 100;
    nextId = () => uuid(id++);
    await runRadarMigrations(database as unknown as MigrationDatabase);
    database.raw.prepare(
      `INSERT INTO sync_state (owner_id, stream, cursor, updated_at, phase)
       VALUES (?, 'business', 10, '2026-07-18T10:00:00.000Z', 'ready')`,
    ).run(ownerId);
    for (let index = 0; index < 5; index += 1) {
      const opportunityId = uuid(index + 1);
      const messageId = uuid(index + 20);
      const opportunity = {
        id: opportunityId, platform: 'telegram', contactName: `Contact ${index}`,
        summary: 'Customer needs a reply', matchedKeywords: ['needs_reply'],
        confidenceScore: 0.5, sourceType: 'group', groupName: `Group ${index}`,
        attentionRequired: false,
      };
      const message = {
        id: messageId, opportunityId, senderName: `Sender ${index}`,
        content: `Message ${index}`, isFromContact: true,
        sentAt: `2026-07-18T10:0${index}:00.000Z`, source: null,
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

  async function execute(
    call: InteractiveAppetiteToolCall,
    allowedTools = new Set([call.name as never]),
    approvedApplyCalls = new Set<string>(),
  ) {
    return executeInteractiveAppetiteTool(database, {
      allowedTools,
      approvedApplyCalls,
      call,
      deviceId,
      ownerId,
      randomId: nextId,
      now: new Date('2026-07-18T12:00:00.000Z'),
    });
  }

  it('starts a bounded local teaching deck without exposing message bodies to the model', async () => {
    const result = await execute({
      name: 'start_teaching_session', arguments: { target_count: 5 }, toolCallId: 'call-1',
    });
    expect(result).toMatchObject({ state: 'ready', cardCount: 5 });
    expect(JSON.stringify(result)).not.toContain('Message 0');
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS total FROM attention_preferences WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ total: 0 });
  });

  it('rejects unauthorized tools and model-supplied apply without a host approval', async () => {
    await expect(executeInteractiveAppetiteTool(database, {
      allowedTools: new Set(),
      approvedApplyCalls: new Set(),
      call: { name: 'inspect_signal_appetite', arguments: {}, toolCallId: 'call-1' },
      deviceId, ownerId, randomId: nextId,
    })).rejects.toMatchObject({ code: 'tool_not_authorized' });
    await expect(execute({
      name: 'apply_appetite_change',
      arguments: { preference_version: 1 },
      toolCallId: 'model-cannot-approve-itself',
    })).rejects.toMatchObject({ code: 'preference_confirmation_required' });
  });

  it('validates arguments before touching local data', async () => {
    await expect(execute({
      name: 'create_temporary_focus',
      arguments: { concept: 'launch', duration_hours: 0 },
      toolCallId: 'call-invalid',
    })).rejects.toMatchObject({ code: 'invalid_tool_arguments' });
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS total FROM temporary_focuses WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ total: 0 });
  });
});
