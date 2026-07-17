import { DatabaseSync } from 'node:sqlite';

import type {
  AnalysisRun,
  AnalysisRunClaim,
  AnalysisRunLinks,
} from '@story2u/radar-contracts/analysis-runs';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));
vi.mock('expo-secure-store', () => ({
  AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY: 'device-only',
  deleteItemAsync: vi.fn(),
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));
vi.mock('expo-sqlite', () => ({ openDatabaseAsync: vi.fn() }));

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import {
  AnalysisRunCoordinator,
  type DeviceAnalysisRunApi,
} from './analysisRunCoordinator';
import {
  deleteLocalAnalysisRun,
  readRecoverableAnalysisRuns,
  saveClaimedAnalysisRun,
  updateLocalAnalysisRun,
  type AnalysisRunStoreExecutor,
} from './analysisRunStore';
import type { AnalysisRunTokenStore } from './runTokenStorage';

const ownerId = '01234567-89ab-cdef-8123-456789abcdef';
const runId = '12345678-1234-4234-8234-123456789abc';
const messageId = '22345678-1234-4234-8234-123456789abc';
const deviceId = '32345678-1234-4234-8234-123456789abc';
const runToken = 'run-token-with-more-than-sixteen-characters';
const now = Date.parse('2026-07-17T10:00:00Z');

class TestDatabase implements AnalysisRunStoreExecutor {
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

  async withExclusiveTransactionAsync(
    task: (transaction: AnalysisRunStoreExecutor) => Promise<void>,
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
}

class MemoryTokenStore implements AnalysisRunTokenStore {
  readonly values = new Map<string, string>();

  async clear(id: string) {
    this.values.delete(id);
  }

  async read(id: string) {
    return this.values.get(id) ?? null;
  }

  async write(id: string, token: string) {
    this.values.set(id, token);
  }
}

function claim(overrides: Partial<AnalysisRunClaim> = {}): AnalysisRunClaim {
  return {
    id: runId,
    messageId,
    deviceId,
    status: 'claimed',
    executedBy: 'device',
    mode: 'primary',
    runtimeVersion: 'pi-0.80.6',
    schemaVersion: 1,
    modelAlias: 'radar-analysis-v1',
    policyVersion: 'agent-policy-v1',
    sourceMessageVersion: 4,
    lockVersion: 1,
    leaseExpiresAt: '2026-07-17T10:02:00Z',
    claimedAt: '2026-07-17T10:00:00Z',
    heartbeatAt: null,
    completedAt: null,
    failedAt: null,
    expiredAt: null,
    failureCode: null,
    shadowMatch: null,
    shadowDifferenceCount: null,
    runToken,
    input: {
      messageId,
      sourceMessageVersion: 4,
      channel: 'telegram',
      senderDisplayName: 'Buyer',
      sourceType: 'private',
      groupName: null,
      text: 'Please send a quote.',
      links: ['https://example.test'],
    },
    ...overrides,
  };
}

function runResponse(lockVersion: number, overrides: Partial<AnalysisRun> = {}): AnalysisRun {
  const source = claim();
  return {
    id: source.id,
    messageId: source.messageId,
    deviceId: source.deviceId,
    status: 'running',
    executedBy: source.executedBy,
    mode: source.mode,
    runtimeVersion: source.runtimeVersion,
    schemaVersion: source.schemaVersion,
    modelAlias: source.modelAlias,
    policyVersion: source.policyVersion,
    sourceMessageVersion: source.sourceMessageVersion,
    lockVersion,
    leaseExpiresAt: '2026-07-17T10:03:00Z',
    claimedAt: source.claimedAt,
    heartbeatAt: '2026-07-17T10:01:00Z',
    completedAt: null,
    failedAt: null,
    expiredAt: null,
    failureCode: null,
    shadowMatch: null,
    shadowDifferenceCount: null,
    ...overrides,
  };
}

function links(): AnalysisRunLinks {
  return {
    runId,
    sourceMessageVersion: 4,
    fetchedAt: '2026-07-17T10:00:10Z',
    evidence: [],
  };
}

const result = {
  is_opportunity: false,
  confidence: 0.2,
  title: 'No opportunity',
  summary: 'No commercial request was found.',
  priority: 'normal' as const,
  trust_score: 80,
  attention_required: false,
  link_status: 'unverified' as const,
  link_summary: null,
  risk_flags: [],
  contacts: {
    email: null,
    phone: null,
    telegram_handle: null,
    wecom_id: null,
    extraction_source: null,
  },
  actions: [],
};

function api(overrides: Partial<DeviceAnalysisRunApi> = {}): DeviceAnalysisRunApi {
  let lockVersion = 1;
  return {
    claim: vi.fn(async () => claim()),
    claimNext: vi.fn(async () => null),
    claimShadow: vi.fn(async () => null),
    heartbeat: vi.fn(async () => runResponse(++lockVersion)),
    inspectLinks: vi.fn(async () => links()),
    complete: vi.fn(async (_id, _token, version) => runResponse(version + 1, {
      status: 'completed',
      completedAt: '2026-07-17T10:01:30Z',
    })),
    fail: vi.fn(async (_id, _token, version, code) => runResponse(version + 1, {
      status: 'failed',
      failedAt: '2026-07-17T10:01:30Z',
      failureCode: code,
    })),
    expire: vi.fn(async () => runResponse(2, {
      status: 'expired',
      expiredAt: '2026-07-17T10:03:00Z',
    })),
    ...overrides,
  };
}

describe('device analysis run persistence and recovery', () => {
  let database: TestDatabase;
  let tokens: MemoryTokenStore;

  beforeEach(async () => {
    database = new TestDatabase();
    tokens = new MemoryTokenStore();
    await runRadarMigrations(database as unknown as MigrationDatabase);
  });

  it('persists only bounded recovery input and never the run token', async () => {
    const stored = await saveClaimedAnalysisRun(database, ownerId, claim());
    const [restored] = await readRecoverableAnalysisRuns(database, ownerId);

    expect(restored).toEqual(stored);
    expect(database.raw.prepare(
      'SELECT input_json FROM analysis_run_state WHERE owner_id = ? AND run_id = ?',
    ).get(ownerId, runId)).not.toMatchObject({ input_json: expect.stringContaining(runToken) });
    const updated = await updateLocalAnalysisRun(database, restored, {
      phase: 'running',
      attemptCount: 1,
      lockVersion: 2,
    });
    expect((await readRecoverableAnalysisRuns(database, ownerId))[0]).toMatchObject({
      phase: 'running',
      attemptCount: 1,
      lockVersion: 2,
    });
    await deleteLocalAnalysisRun(database, ownerId, updated.runId);
    await expect(readRecoverableAnalysisRuns(database, ownerId)).resolves.toEqual([]);
  });

  it('heartbeats during the model stream, completes with the latest lock and clears recovery state', async () => {
    const transport = api();
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      heartbeatIntervalMs: 2,
      now: () => now,
      execute: async () => {
        await new Promise((resolve) => setTimeout(resolve, 12));
        return result;
      },
    });

    await expect(coordinator.claimAndExecute(ownerId, messageId)).resolves.toBe('completed');
    expect(vi.mocked(transport.heartbeat).mock.calls.length).toBeGreaterThan(1);
    const completeCall = vi.mocked(transport.complete).mock.calls[0];
    const latestHeartbeat = vi.mocked(transport.heartbeat).mock.results.at(-1);
    const latestRun = await latestHeartbeat?.value;
    expect(completeCall[2]).toBe(latestRun?.lockVersion);
    await expect(readRecoverableAnalysisRuns(database, ownerId)).resolves.toEqual([]);
    expect(tokens.values.size).toBe(0);
  });

  it('keeps an interrupted foreground run for AppState/network recovery instead of releasing it', async () => {
    const controller = new AbortController();
    const transport = api();
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      heartbeatIntervalMs: 1_000,
      now: () => now,
      execute: async (_claim, _links, signal) => {
        controller.abort();
        await new Promise((resolve) => setTimeout(resolve, 0));
        expect(signal.aborted).toBe(true);
        throw new Error('cancelled');
      },
    });

    await expect(
      coordinator.claimAndExecute(ownerId, messageId, controller.signal),
    ).resolves.toBe('deferred');
    expect(transport.fail).not.toHaveBeenCalled();
    expect(tokens.values.get(runId)).toBe(runToken);
    expect((await readRecoverableAnalysisRuns(database, ownerId))[0]).toMatchObject({
      attemptCount: 1,
      lastErrorCode: 'analysis_interrupted',
      phase: 'running',
    });
  });

  it('expires stale runs without needing the run token', async () => {
    await saveClaimedAnalysisRun(database, ownerId, claim({
      leaseExpiresAt: '2026-07-17T09:59:00Z',
    }));
    const transport = api();
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      now: () => now,
      execute: vi.fn(),
    });

    await expect(coordinator.recover(ownerId, false)).resolves.toEqual(['expired']);
    expect(transport.expire).toHaveBeenCalledWith(runId, undefined);
    await expect(readRecoverableAnalysisRuns(database, ownerId)).resolves.toEqual([]);
  });

  it('explicitly fails a run after the bounded retry budget', async () => {
    const stored = await saveClaimedAnalysisRun(database, ownerId, claim());
    await updateLocalAnalysisRun(database, stored, { attemptCount: 3 });
    await tokens.write(runId, runToken);
    const transport = api();
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      now: () => now,
      execute: vi.fn(),
    });

    await expect(coordinator.recover(ownerId, true)).resolves.toEqual(['failed']);
    expect(transport.fail).toHaveBeenCalledWith(
      runId,
      runToken,
      1,
      'agent_retry_exhausted',
      undefined,
    );
    await expect(readRecoverableAnalysisRuns(database, ownerId)).resolves.toEqual([]);
    expect(tokens.values.size).toBe(0);
  });

  it('claims and executes one server-selected shadow candidate after local recovery', async () => {
    const shadowClaim = claim({ mode: 'shadow' });
    const transport = api({ claimShadow: vi.fn(async () => shadowClaim) });
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      heartbeatIntervalMs: 1_000,
      now: () => now,
      execute: vi.fn(async () => result),
    });

    await expect(coordinator.recover(ownerId, true)).resolves.toEqual(['completed']);
    expect(transport.claimShadow).toHaveBeenCalledTimes(1);
    expect(transport.complete).toHaveBeenCalledTimes(1);
    await expect(readRecoverableAnalysisRuns(database, ownerId)).resolves.toEqual([]);
    expect(tokens.values.size).toBe(0);
  });

  it('prioritizes one server-selected primary candidate over shadow work', async () => {
    const primaryClaim = claim({ mode: 'primary' });
    const transport = api({
      claimNext: vi.fn(async () => primaryClaim),
      claimShadow: vi.fn(async () => claim({ mode: 'shadow' })),
    });
    const coordinator = new AnalysisRunCoordinator({
      api: transport,
      database,
      tokenStore: tokens,
      heartbeatIntervalMs: 1_000,
      now: () => now,
      execute: vi.fn(async () => result),
    });

    await expect(coordinator.recover(ownerId, true)).resolves.toEqual(['completed']);
    expect(transport.claimNext).toHaveBeenCalledTimes(1);
    expect(transport.claimShadow).not.toHaveBeenCalled();
    expect(transport.complete).toHaveBeenCalledTimes(1);
  });
});
