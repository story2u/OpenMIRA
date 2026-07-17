import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import { afterEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../../storage/migrations';
import {
  AgentSessionStoreError,
  appendAgentEntry,
  createAgentSession,
  deleteAgentSession,
  listAgentSessions,
  MAXIMUM_AGENT_ENTRIES_PER_SESSION,
  MAXIMUM_AGENT_SESSIONS_PER_OWNER,
  readAgentEntries,
  type AgentSessionStoreDatabase,
  type AgentSessionStoreExecutor,
} from './sessionStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '11234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '21234567-89ab-cdef-0123-456789abcdef';
const sessionId = '31234567-89ab-cdef-0123-456789abcdef';
const createdAt = new Date('2026-07-01T10:00:00.000Z');

class TestDatabase implements AgentSessionStoreDatabase {
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
    task: (transaction: AgentSessionStoreExecutor) => Promise<void>,
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

function stableUuid(value: number) {
  return `41234567-89ab-4def-8123-${value.toString(16).padStart(12, '0')}`;
}

describe('interactive Agent local session store', () => {
  const directories: string[] = [];

  afterEach(() => {
    for (const directory of directories.splice(0)) {
      rmSync(directory, { force: true, recursive: true });
    }
  });

  async function open() {
    const directory = mkdtempSync(join(tmpdir(), 'radar-agent-session-'));
    directories.push(directory);
    const path = join(directory, 'radar.db');
    const database = new TestDatabase(path);
    await runRadarMigrations(database as unknown as MigrationDatabase);
    return { database, path };
  }

  it('persists typed ordered entries across close and reopen', async () => {
    const { database, path } = await open();
    const session = await createAgentSession(database, {
      ownerId,
      sessionId,
      opportunityId,
      title: '  Deployment question  ',
    }, createdAt);
    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: { type: 'user', text: 'What does the customer need?' },
    }, createdAt);
    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: {
        type: 'tool_call',
        toolCallId: 'call-1',
        toolName: 'get_opportunity',
        arguments: { opportunity_id: opportunityId },
      },
    }, new Date('2026-07-01T10:00:01.000Z'));
    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: {
        type: 'tool_result',
        toolCallId: 'call-1',
        toolName: 'get_opportunity',
        result: { id: opportunityId, summary: 'Needs deployment' },
      },
    }, new Date('2026-07-01T10:00:02.000Z'));

    expect(session).toMatchObject({ title: 'Deployment question', schemaVersion: 1 });
    database.close();

    const reopened = new TestDatabase(path);
    await runRadarMigrations(reopened as unknown as MigrationDatabase);
    const sessions = await listAgentSessions(reopened, ownerId, createdAt);
    const entries = await readAgentEntries(reopened, ownerId, sessionId);

    expect(sessions).toHaveLength(1);
    expect(entries.map((entry) => entry.seq)).toEqual([1, 2, 3]);
    expect(entries[1].content).toMatchObject({
      type: 'tool_call',
      toolName: 'get_opportunity',
    });
    reopened.close();
  });

  it('purges expired sessions with entries and isolates owners on delete', async () => {
    const { database } = await open();
    await createAgentSession(database, { ownerId, sessionId, title: 'Old session' }, createdAt);
    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: { type: 'assistant', text: 'Local only' },
    }, createdAt);

    expect(await readAgentEntries(database, otherOwnerId, sessionId)).toEqual([]);
    await deleteAgentSession(database, otherOwnerId, sessionId);
    expect(await readAgentEntries(database, ownerId, sessionId)).toHaveLength(1);

    const afterRetention = new Date('2026-08-01T10:00:01.000Z');
    expect(await listAgentSessions(database, ownerId, afterRetention)).toEqual([]);
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS total FROM agent_entries WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ total: 0 });
    database.close();
  });

  it('enforces owner session and per-session entry capacity', async () => {
    const { database } = await open();
    for (let index = 0; index < MAXIMUM_AGENT_SESSIONS_PER_OWNER; index += 1) {
      await createAgentSession(database, {
        ownerId,
        sessionId: stableUuid(index),
        title: `Session ${index}`,
      }, createdAt);
    }
    await expect(createAgentSession(database, {
      ownerId,
      sessionId,
      title: 'One too many',
    }, createdAt)).rejects.toMatchObject({ code: 'agent_session_limit_reached' });

    const entrySessionId = stableUuid(0);
    for (let index = 0; index < MAXIMUM_AGENT_ENTRIES_PER_SESSION; index += 1) {
      await appendAgentEntry(database, {
        ownerId,
        sessionId: entrySessionId,
        content: { type: 'error', code: `test_${index}` },
      }, createdAt);
    }
    await expect(appendAgentEntry(database, {
      ownerId,
      sessionId: entrySessionId,
      content: { type: 'error', code: 'one_too_many' },
    }, createdAt)).rejects.toMatchObject({ code: 'agent_entry_limit_reached' });
    database.close();
  });

  it('persists reviewed tools but rejects approval material, oversized and corrupt entries', async () => {
    const { database } = await open();
    await createAgentSession(database, { ownerId, sessionId, title: 'Strict session' }, createdAt);

    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: {
        type: 'tool_result',
        toolCallId: 'call-local-draft',
        toolName: 'draft_reply',
        result: { opportunity_id: opportunityId, draft: 'Local only', sent: false },
      },
    }, createdAt);
    await expect(readAgentEntries(database, ownerId, sessionId)).resolves.toEqual([
      expect.objectContaining({
        content: expect.objectContaining({ toolName: 'draft_reply' }),
      }),
    ]);

    await expect(appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: {
        type: 'tool_call',
        toolCallId: 'call-1',
        toolName: 'send_reply',
        arguments: {
          opportunity_id: opportunityId,
          text: 'Safe reply',
          approvalToken: 'must-never-enter-sqlite',
        },
      } as never,
    }, createdAt)).rejects.toMatchObject({ code: 'invalid_agent_entry' });
    await expect(appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: { type: 'assistant', text: 'x'.repeat(32_001) },
    }, createdAt)).rejects.toMatchObject({ code: 'invalid_agent_entry' });
    await expect(appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: {
        type: 'tool_result',
        toolCallId: 'call-oversized',
        toolName: 'get_messages',
        result: { text: '数'.repeat(30_000) },
      },
    }, createdAt)).rejects.toMatchObject({ code: 'agent_entry_too_large' });
    expect(() => new AgentSessionStoreError('example')).not.toThrow();

    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: { type: 'assistant', text: 'valid' },
    }, createdAt);
    database.raw.prepare(
      `UPDATE agent_entries SET content_json = '{"type":"assistant"}'
       WHERE owner_id = ? AND session_id = ? AND seq = 2`,
    ).run(ownerId, sessionId);
    await expect(readAgentEntries(database, ownerId, sessionId)).rejects.toMatchObject({
      code: 'agent_entry_corrupt',
    });
    database.close();
  });
});
