import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

import { afterEach, describe, expect, it } from 'vitest';

import { INTERACTIVE_BRIEFING_TOOLS, type InteractiveBriefingToolName } from '@story2u/radar-agent/interactive';

import { runRadarMigrations, type MigrationDatabase } from '../../storage/migrations';
import type { BriefingStoreDatabase, BriefingStoreExecutor } from '../../briefing/briefingStore';
import { readBriefingSchedule } from '../../briefing/briefingStore';
import { executeInteractiveBriefingTool, InteractiveBriefingToolError } from './briefingTools';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const deviceId = '11234567-89ab-cdef-0123-456789abcdef';

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
  const dir = mkdtempSync(join(tmpdir(), 'briefing-tools-'));
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
  return `81234567-89ab-4def-8123-${idCounter.toString().padStart(12, '0')}`;
}

const allTools = new Set<InteractiveBriefingToolName>(
  INTERACTIVE_BRIEFING_TOOLS.map((tool) => tool.name as InteractiveBriefingToolName),
);

async function insertDecision(
  database: TestDatabase,
  input: {
    messageId: string;
    decision?: string;
    confidence?: number;
    decidedAt: string;
    concept?: string;
    reason?: string;
  },
) {
  await database.runAsync(
    `INSERT INTO message_filter_decisions (
      owner_id, message_id, preference_version, decision, confidence,
      reason_summary, evidence_json, evaluator, decided_at, expires_at
    ) VALUES (?, ?, 1, ?, ?, ?, ?, 'deterministic', ?, NULL)`,
    ownerId,
    input.messageId,
    input.decision ?? 'inbox',
    input.confidence ?? 0.9,
    input.reason ?? 'matched preference',
    JSON.stringify([{ kind: 'preference', label: input.concept ?? 'purchase_intent' }]),
    input.decidedAt,
  );
}

function run(database: TestDatabase, name: string, args: unknown, allowed = allTools) {
  return executeInteractiveBriefingTool(database, {
    allowedTools: allowed,
    call: { name, arguments: args, toolCallId: 'call-1' },
    deviceId,
    ownerId,
    randomId: nextId,
    now: new Date('2026-07-18T12:00:00.000Z'),
  });
}

describe('executeInteractiveBriefingTool', () => {
  it('summarize_time_window 生成结构化简报（含条目与分类）', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z', decision: 'immediate' });
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T09:00:00.000Z', decision: 'suppress', concept: 'advertising' });
    const result = await run(database, 'summarize_time_window', { briefing_type: 'morning' });
    expect(result.state).toBe('generated');
    expect(result.totalMessages).toBe(2);
    expect(result.immediateCount).toBe(1);
    expect(result.suppressedCount).toBe(1);
    expect((result.items as unknown[]).length).toBe(1);
  });

  it('get_attention_snapshot 返回当天统计；list_priority_items 读取最新简报条目', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z', decision: 'immediate' });
    await run(database, 'summarize_time_window', { briefing_type: 'morning' });

    const snapshot = await run(database, 'get_attention_snapshot', {});
    expect(snapshot.totalProcessed).toBe(1);
    expect(snapshot.immediateCount).toBe(1);

    const items = await run(database, 'list_priority_items', { priority: 'action_required' });
    expect(items.state).toBe('ok');
    expect((items.items as Array<{ priority: string }>)[0]?.priority).toBe('action_required');

    const empty = await run(database, 'list_priority_items', { priority: 'needs_judgment' });
    expect((empty.items as unknown[]).length).toBe(0);
  });

  it('list_category_items 按证据标签检索；get_quiet_summary 聚合安静区', async () => {
    const database = await createDatabase();
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T08:00:00.000Z', decision: 'suppress', concept: 'advertising', reason: 'ad content' });
    await insertDecision(database, { messageId: nextId(), decidedAt: '2026-07-18T09:00:00.000Z', concept: 'purchase_intent' });

    const ads = await run(database, 'list_category_items', { category: 'advertising' });
    expect((ads.items as unknown[]).length).toBe(1);
    const other = await run(database, 'list_category_items', { category: 'other' });
    expect((other.items as unknown[]).length).toBe(0);

    const quiet = await run(database, 'get_quiet_summary', {});
    expect((quiet.categories as Array<{ category: string; count: number }>)[0]).toMatchObject({
      category: 'advertising',
      count: 1,
    });
  });

  it('update_brief_schedule 持久化新时间表', async () => {
    const database = await createDatabase();
    const result = await run(database, 'update_brief_schedule', {
      entries: [
        { briefing_type: 'morning', minute_of_day: 540, days: [1, 2, 3, 4, 5], enabled: true },
      ],
    });
    expect(result.state).toBe('updated');
    const schedule = await readBriefingSchedule(database, ownerId);
    expect(schedule).toHaveLength(1);
    expect(schedule[0]).toMatchObject({ briefingType: 'morning', minuteOfDay: 540 });
  });

  it('未授权工具与非法参数被拒绝', async () => {
    const database = await createDatabase();
    await expect(run(database, 'get_attention_snapshot', {}, new Set())).rejects.toThrow(
      new InteractiveBriefingToolError('tool_not_authorized'),
    );
    await expect(run(database, 'summarize_time_window', { briefing_type: 'hourly' })).rejects.toThrow(
      new InteractiveBriefingToolError('invalid_tool_arguments'),
    );
    await expect(run(database, 'not_a_tool', {})).rejects.toThrow(
      new InteractiveBriefingToolError('unknown_tool'),
    );
  });
});
