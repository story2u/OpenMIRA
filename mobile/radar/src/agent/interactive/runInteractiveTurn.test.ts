import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import type { InteractiveAgentTurn, InteractiveAgentTurnClaim } from '@story2u/radar-contracts/interactive-agent';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('expo-crypto', () => ({ randomUUID: vi.fn() }));
vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));
vi.mock('expo-secure-store', () => ({
  AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY: 'device-only',
  deleteItemAsync: vi.fn(),
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

import { runRadarMigrations, type MigrationDatabase } from '../../storage/migrations';
import type { SyncStoreExecutor } from '../../sync/syncStore';
import {
  createAgentSession,
  readAgentEntries,
  type AgentSessionStoreDatabase,
  type AgentSessionStoreExecutor,
} from './sessionStore';
import {
  runInteractiveTurn,
  type InteractiveTurnDependencies,
} from './runInteractiveTurn';

const ownerId = '01234567-89ab-4def-8123-456789abcdef';
const sessionId = '11234567-89ab-4def-8123-456789abcdef';
const turnId = '21234567-89ab-4def-8123-456789abcdef';
const turnToken = 'interactive-turn-token-long-enough';

class TestDatabase implements AgentSessionStoreDatabase, SyncStoreExecutor {
  readonly raw: DatabaseSync;

  constructor(path: string) {
    this.raw = new DatabaseSync(path);
  }

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
  close() { this.raw.close(); }
}

function turn(status: InteractiveAgentTurn['status'], lockVersion: number): InteractiveAgentTurn {
  return {
    id: turnId,
    localSessionId: sessionId,
    deviceId: '31234567-89ab-4def-8123-456789abcdef',
    status,
    runtimeVersion: 'pi-0.80.6',
    schemaVersion: 1,
    modelAlias: 'radar-interactive-v1',
    policyVersion: 'interactive-read-only-v1',
    lockVersion,
    requestCount: status === 'completed' ? 1 : 0,
    leaseExpiresAt: '2026-07-17T10:05:00Z',
    claimedAt: '2026-07-17T10:00:00Z',
    heartbeatAt: '2026-07-17T10:00:01Z',
    completedAt: status === 'completed' ? '2026-07-17T10:00:02Z' : null,
    failedAt: status === 'failed' ? '2026-07-17T10:00:02Z' : null,
    expiredAt: null,
    failureCode: status === 'failed' ? 'interactive_agent_failed' : null,
  };
}

function turnClaim(): InteractiveAgentTurnClaim {
  return { ...turn('claimed', 1), turnToken };
}

describe('interactive turn coordinator', () => {
  const directories: string[] = [];

  afterEach(() => {
    for (const directory of directories.splice(0)) {
      rmSync(directory, { force: true, recursive: true });
    }
  });

  async function open() {
    const directory = mkdtempSync(join(tmpdir(), 'radar-interactive-turn-'));
    directories.push(directory);
    const database = new TestDatabase(join(directory, 'radar.db'));
    await runRadarMigrations(database as unknown as MigrationDatabase);
    await createAgentSession(database, { ownerId, sessionId, title: 'Local turn' });
    return database;
  }

  it('heartbeats first, batches the local result, then confirms server completion', async () => {
    const database = await open();
    const order: string[] = [];
    const complete = vi.fn(async (_baseUrl, _turnId, _token, lockVersion) => {
      order.push('complete');
      expect(lockVersion).toBe(2);
      expect((await readAgentEntries(database, ownerId, sessionId)).map(
        (entry) => entry.content.type,
      )).toEqual(['user', 'assistant']);
      return turn('completed', 3);
    });
    const dependencies: InteractiveTurnDependencies = {
      claim: vi.fn(async () => {
        order.push('claim');
        return turnClaim();
      }),
      heartbeat: vi.fn(async () => {
        order.push('heartbeat');
        return turn('running', 2);
      }),
      runHost: vi.fn(async ({ entries, onStreamText }) => {
        order.push('host');
        expect(entries.at(-1)?.content).toEqual({ type: 'user', text: 'Find quotes' });
        onStreamText?.('Local answer');
        return {
          entries: [{ type: 'assistant' as const, text: 'Local answer' }],
          finalText: 'Local answer',
        };
      }),
      complete,
      fail: vi.fn(async () => turn('failed', 3)),
      randomId: () => '41234567-89ab-4def-8123-456789abcdef',
    };

    await expect(runInteractiveTurn({
      baseUrl: 'https://api.example.test',
      database,
      dependencies,
      ownerId,
      sessionId,
      text: 'Find quotes',
    })).resolves.toMatchObject({ finalText: 'Local answer' });
    expect(order).toEqual(['claim', 'heartbeat', 'host', 'complete']);
    expect(dependencies.fail).not.toHaveBeenCalled();
    database.close();
  });

  it('records a stable local error and releases the server reservation on host failure', async () => {
    const database = await open();
    const fail = vi.fn(async () => turn('failed', 3));
    const dependencies: InteractiveTurnDependencies = {
      claim: vi.fn(async () => turnClaim()),
      heartbeat: vi.fn(async () => turn('running', 2)),
      runHost: vi.fn(async () => { throw new Error('provider-secret-must-not-persist'); }),
      complete: vi.fn(async () => turn('completed', 3)),
      fail,
      randomId: () => '51234567-89ab-4def-8123-456789abcdef',
    };

    await expect(runInteractiveTurn({
      baseUrl: 'https://api.example.test',
      database,
      dependencies,
      ownerId,
      sessionId,
      text: 'Find quotes',
    })).rejects.toThrow('interactive_agent_failed');
    expect(fail).toHaveBeenCalledWith(
      'https://api.example.test',
      turnId,
      turnToken,
      2,
      'interactive_agent_failed',
    );
    const persisted = await readAgentEntries(database, ownerId, sessionId);
    expect(persisted.map((entry) => entry.content)).toEqual([
      { type: 'user', text: 'Find quotes' },
      { type: 'error', code: 'interactive_agent_failed' },
    ]);
    expect(JSON.stringify(persisted)).not.toContain('provider-secret');
    database.close();
  });
});
