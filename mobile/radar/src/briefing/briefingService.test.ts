import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import { afterEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { clearLocalUserDataInDatabase } from '../storage/userData';
import {
  dismissBriefing,
  generateBriefing,
  generateDueBriefings,
  getAttentionSnapshot,
  getBriefing,
  getBriefingSchedule,
  getQuietSummary,
  listDueBriefingTypes,
  markBriefingItemHandled,
  updateBriefingSchedule,
} from './briefingService';
import { readBriefingEvents, type BriefingStoreDatabase, type BriefingStoreExecutor } from './briefingStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '11234567-89ab-cdef-0123-456789abcdef';
const deviceId = '21234567-89ab-cdef-0123-456789abcdef';

class TestDatabase implements BriefingStoreDatabase {
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

  async withExclusiveTransactionAsync(task: (transaction: BriefingStoreExecutor) => Promise<void>) {
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

let cleanups: Array<() => void> = [];

afterEach(() => {
  cleanups.forEach((cleanup) => cleanup());
  cleanups = [];
});

async function createDatabase(): Promise<TestDatabase> {
  const dir = mkdtempSync(join(tmpdir(), 'briefing-'));
  const database = new TestDatabase(join(dir, 'radar.db'));
  await runRadarMigrations(database as unknown as MigrationDatabase);
  cleanups.push(() => {
    database.close();
    rmSync(dir, { recursive: true, force: true });
  });
  return database;
}

let idCounter = 0;
function nextId() {
  idCounter += 1;
  return `71234567-89ab-4def-8123-${idCounter.toString().padStart(12, '0')}`;
}

async function insertDecision(
  database: TestDatabase,
  input: {
    ownerId?: string;
    messageId: string;
    decision?: 'immediate' | 'inbox' | 'digest' | 'suppress';
    confidence?: number;
    decidedAt: string;
    reason?: string;
    concept?: string;
    evaluator?: string;
  },
) {
  await database.runAsync(
    `INSERT INTO message_filter_decisions (
      owner_id, message_id, preference_version, decision, confidence,
      reason_summary, evidence_json, evaluator, decided_at, expires_at
    ) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, NULL)`,
    input.ownerId ?? ownerId,
    input.messageId,
    input.decision ?? 'inbox',
    input.confidence ?? 0.9,
    input.reason ?? 'matched preference',
    JSON.stringify([{ kind: 'preference', label: input.concept ?? 'purchase_intent' }]),
    input.evaluator ?? 'deterministic',
    input.decidedAt,
  );
}

const context = (nowIso: string) => ({
  ownerId,
  deviceId,
  now: () => new Date(nowIso),
  createId: nextId,
});

const localDate = (hour: number, minute: number) => new Date(2026, 6, 20, hour, minute, 0, 0);

const localContext = (hour: number, minute: number) => ({
  ownerId,
  deviceId,
  now: () => localDate(hour, minute),
  createId: nextId,
});

describe('generateBriefing', () => {
  it('早间简报覆盖窗口内决策；午间简报接续窗口且不重复早间内容', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T07:00:00.000Z' });
    const morningOnly = nextId();
    await insertDecision(database, { messageId: morningOnly, decidedAt: '2026-07-18T08:00:00.000Z', decision: 'immediate' });

    const morning = await generateBriefing(database, context('2026-07-18T08:30:00.000Z'), { type: 'morning' });
    expect(morning.briefing.totalMessages).toBe(2);
    expect(morning.briefing.immediateCount).toBe(1);

    // 午间新增一条；早间的两条不得再出现
    const middayMessage = nextId();
    await insertDecision(database, { messageId: middayMessage, decidedAt: '2026-07-18T11:00:00.000Z' });
    const midday = await generateBriefing(database, context('2026-07-18T12:00:00.000Z'), { type: 'midday' });
    expect(midday.briefing.coveredFrom).toBe(morning.briefing.coveredTo);
    expect(midday.briefing.includedMessageIds).toEqual([middayMessage]);
    expect(midday.briefing.includedMessageIds).not.toContain(morningOnly);
  });

  it('晚间摘要排除用户已处理的条目', async () => {
    const database = await createDatabase();
    const handled = nextId();
    await insertDecision(database, { messageId: handled, decidedAt: '2026-07-18T09:00:00.000Z', decision: 'immediate' });
    const morning = await generateBriefing(database, context('2026-07-18T09:30:00.000Z'), { type: 'morning' });
    const item = morning.items.find((entry) => entry.entityId === handled);
    expect(item).toBeDefined();
    await markBriefingItemHandled(database, context('2026-07-18T10:00:00.000Z'), {
      briefingId: morning.briefing.id,
      itemId: item!.id,
      entityId: handled,
    });

    // 同一条消息晚间再次进入窗口（例如晚间从头覆盖当天）也必须被排除：
    // 通过 dismiss 早间简报使其覆盖窗口失效，强制晚间回看全天。
    await dismissBriefing(database, context('2026-07-18T18:00:00.000Z'), morning.briefing.id);
    const evening = await generateBriefing(database, context('2026-07-18T18:30:00.000Z'), { type: 'evening' });
    expect(evening.briefing.includedMessageIds).not.toContain(handled);
    expect(evening.briefing.excludedHandledIds).toContain(handled);
  });

  it('L2 摘要成功时 generatedBy=cloud；失败时保留本地结果且不影响已有简报', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z' });
    const withSummary = await generateBriefing(database, context('2026-07-18T08:30:00.000Z'), {
      type: 'morning',
      summarize: async () => '早上有一条明确采购需求。',
    });
    expect(withSummary.briefing.summary).toBe('早上有一条明确采购需求。');
    expect(withSummary.briefing.generatedBy).toBe('cloud');

    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T11:00:00.000Z' });
    const degraded = await generateBriefing(database, context('2026-07-18T12:00:00.000Z'), {
      type: 'midday',
      summarize: async () => {
        throw new Error('gateway unavailable');
      },
    });
    expect(degraded.briefing.summary).toBeNull();
    expect(degraded.briefing.generatedBy).toBe('local');

    // 云端失败绝不清空已有简报
    const kept = await getBriefing(database, context('2026-07-18T12:01:00.000Z'), withSummary.briefing.id);
    expect(kept?.briefing.summary).toBe('早上有一条明确采购需求。');
  });

  it('事件与投影同事务：生成后事件日志包含 Started/Generated/SnapshotUpdated', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z' });
    await generateBriefing(database, context('2026-07-18T08:30:00.000Z'), { type: 'morning' });
    const events = await readBriefingEvents(database, ownerId);
    expect(events.map((event) => event.type)).toEqual([
      'BriefingGenerationStarted',
      'BriefingGenerated',
      'AttentionSnapshotUpdated',
    ]);
  });

  it('owner 隔离：其他用户的决策不进入简报，清理账号数据后简报清空', async () => {
    const database = await createDatabase();
    await insertDecision(database, { ownerId: otherOwnerId, messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z' });
    const briefing = await generateBriefing(database, context('2026-07-18T08:30:00.000Z'), { type: 'morning' });
    expect(briefing.briefing.totalMessages).toBe(0);

    await clearLocalUserDataInDatabase(database as never, ownerId);
    const events = await readBriefingEvents(database, ownerId);
    expect(events).toHaveLength(0);
  });
});

describe('snapshot 与安静区', () => {
  it('快照统计当天全部决策；安静区只聚合 suppress', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T01:00:00.000Z', decision: 'immediate' });
    await insertDecision(database, {
      messageId: nextId(),
      decidedAt: '2026-07-18T02:00:00.000Z',
      decision: 'suppress',
      concept: 'advertising',
      reason: 'advertising content',
      evaluator: 'on_device_model',
    });
    await insertDecision(database, {
      messageId: nextId(),
      decidedAt: '2026-07-18T03:00:00.000Z',
      decision: 'suppress',
      concept: 'advertising',
      reason: 'advertising content',
    });
    const ctx = context('2026-07-18T12:00:00.000Z');
    const snapshot = await getAttentionSnapshot(database, ctx);
    expect(snapshot).toMatchObject({
      totalProcessed: 3,
      immediateCount: 1,
      suppressedCount: 2,
      deepAnalyzed: 0,
    });
    const quiet = await getQuietSummary(database, ctx);
    expect(quiet).toHaveLength(1);
    expect(quiet[0]).toMatchObject({ category: 'advertising', count: 2, topReason: 'advertising content' });
  });
});

describe('简报时间表', () => {
  it('默认返回 08:30/12:00/18:30；更新后持久化并写事件', async () => {
    const database = await createDatabase();
    const ctx = context('2026-07-18T12:00:00.000Z');
    const defaults = await getBriefingSchedule(database, ctx);
    expect(defaults.map((entry) => entry.minuteOfDay)).toEqual([510, 720, 1110]);

    await updateBriefingSchedule(database, ctx, [
      { briefingType: 'morning', minuteOfDay: 9 * 60, days: [1, 2, 3, 4, 5], enabled: true },
      { briefingType: 'evening', minuteOfDay: 19 * 60, days: [1, 2, 3, 4, 5, 6, 0], enabled: true },
    ]);
    const updated = await getBriefingSchedule(database, ctx);
    expect(updated).toHaveLength(2);
    expect(updated[0]).toMatchObject({ briefingType: 'morning', minuteOfDay: 540 });
    const events = await readBriefingEvents(database, ownerId);
    expect(events.at(-1)?.type).toBe('BriefingScheduleUpdated');
  });

  it('到点触发简报并按同一日同一时段去重，L2 summarizer 从调度入口透传', async () => {
    const database = await createDatabase();
    const messageId = nextId();
    await insertDecision(database, {
      messageId,
      decidedAt: localDate(8, 10).toISOString(),
      decision: 'immediate',
    });

    expect(await listDueBriefingTypes(database, localContext(8, 29))).toEqual([]);
    expect(await listDueBriefingTypes(database, localContext(8, 31))).toEqual(['morning']);

    const generated = await generateDueBriefings(database, localContext(8, 31), {
      summarize: async ({ briefing }) => `整理了 ${briefing.totalMessages} 条`,
    });
    expect(generated).toHaveLength(1);
    expect(generated[0].briefing.type).toBe('morning');
    expect(generated[0].briefing.summary).toBe('整理了 1 条');
    expect(generated[0].briefing.includedMessageIds).toEqual([messageId]);

    const duplicate = await generateDueBriefings(database, localContext(8, 45));
    expect(duplicate).toEqual([]);
  });

  it('停用时段或非生效日期不会触发简报', async () => {
    const database = await createDatabase();
    const ctx = localContext(12, 10);
    await updateBriefingSchedule(database, ctx, [
      { briefingType: 'morning', minuteOfDay: 8 * 60, days: [2], enabled: true },
      { briefingType: 'midday', minuteOfDay: 12 * 60, days: [1], enabled: false },
    ]);

    expect(await listDueBriefingTypes(database, ctx)).toEqual([]);
  });
});
