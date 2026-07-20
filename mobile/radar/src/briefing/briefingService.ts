import {
  composeAttentionSnapshot,
  composeBriefing,
  summarizeQuietItems,
} from '@story2u/radar-core/briefing/compose';
import type { NewBriefingEvent } from './briefingStore';
import type {
  AttentionSnapshot,
  Briefing,
  BriefingItem,
  BriefingScheduleEntry,
  BriefingType,
  QuietItemSummary,
  ScheduledBriefingType,
} from '@story2u/radar-core/briefing/model';

import { readMessageFilterDecisions } from '../attention/signalAppetiteStore';
import {
  appendBriefingEvent,
  persistBriefingGeneration,
  persistBriefingItemHandled,
  persistBriefingSchedule,
  persistBriefingStatus,
  readBriefing,
  readBriefingItems,
  readBriefings,
  readBriefingSchedule,
  readHandledEntityIds,
  type BriefingStoreDatabase,
} from './briefingStore';

export interface BriefingServiceContext {
  ownerId: string;
  deviceId: string;
  now?: () => Date;
  createId?: () => string;
}

/**
 * 云端语言整理钩子：输入结构化简报，返回 1-3 句中文摘要。
 * 失败/超时由服务捕获——本地简报照常落库，summary 保持 null 走兜底渲染。
 * 绝不把该函数的输出当作事实来源：计数与条目一律来自结构化决策。
 */
export type BriefingSummarizer = (input: {
  briefing: Briefing;
  items: readonly BriefingItem[];
}) => Promise<string | null>;

function defaultCreateId(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function resolve(context: BriefingServiceContext) {
  return {
    ownerId: context.ownerId,
    deviceId: context.deviceId,
    now: context.now ?? (() => new Date()),
    createId: context.createId ?? defaultCreateId,
  };
}

const BRIEFING_TITLE_KEYS: Readonly<Record<BriefingType, string>> = {
  morning: 'briefing.title.morning',
  midday: 'briefing.title.midday',
  evening: 'briefing.title.evening',
  ad_hoc: 'briefing.title.adHoc',
  urgent: 'briefing.title.urgent',
};

const FALLBACK_WINDOW_HOURS = 24;

export interface GenerateBriefingResult {
  briefing: Briefing;
  items: readonly BriefingItem[];
  snapshot: AttentionSnapshot;
}

function scheduledInstantFor(now: Date, minuteOfDay: number) {
  const scheduled = new Date(now);
  scheduled.setHours(Math.floor(minuteOfDay / 60), minuteOfDay % 60, 0, 0);
  return scheduled;
}

function wasGeneratedForSchedule(
  briefings: readonly Briefing[],
  type: ScheduledBriefingType,
  scheduledAt: Date,
  now: Date,
) {
  const scheduledIso = scheduledAt.toISOString();
  const nowIso = now.toISOString();
  return briefings.some((briefing) => (
    briefing.type === type &&
    briefing.generatedAt >= scheduledIso &&
    briefing.generatedAt <= nowIso
  ));
}

/**
 * 生成增量简报：
 * 1. coveredFrom 接续上一份未撤销简报的 coveredTo（无则回看 24h）；
 * 2. 已被此前简报覆盖或已处理的消息绝不重复出现；
 * 3. summarize（L2）失败只降级 summary，不影响结构化结果，也绝不清空已有简报。
 */
export async function generateBriefing(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  options: { type: BriefingType; summarize?: BriefingSummarizer },
): Promise<GenerateBriefingResult> {
  const { ownerId, deviceId, now, createId } = resolve(context);
  const generatedAt = now().toISOString();

  const existing = await readBriefings(database, ownerId, { limit: 50 });
  const active = existing.filter((briefing) => briefing.status !== 'dismissed');
  const lastCoveredTo = active.map((briefing) => briefing.coveredTo).sort().at(-1) ?? null;
  const coveredFrom =
    lastCoveredTo ??
    new Date(now().getTime() - FALLBACK_WINDOW_HOURS * 60 * 60 * 1000).toISOString();

  const decisions = await readMessageFilterDecisions(database, ownerId, {
    limit: 500,
    decidedFrom: coveredFrom,
    decidedTo: generatedAt,
  });

  const previouslyIncludedIds = new Set<string>();
  for (const briefing of active) {
    for (const id of briefing.includedMessageIds) previouslyIncludedIds.add(id);
  }
  // "已处理"跨全部简报（含已撤销）收集：用户动作不随简报撤销而失效。
  const handledIds = await readHandledEntityIds(database, ownerId);

  const briefingId = createId();
  const composed = composeBriefing({
    id: briefingId,
    type: options.type,
    title: BRIEFING_TITLE_KEYS[options.type],
    coveredFrom,
    coveredTo: generatedAt,
    generatedAt,
    decisions,
    previouslyIncludedIds,
    handledIds,
    createItemId: () => createId(),
  });

  let briefing = composed.briefing;
  if (options.summarize) {
    try {
      const summary = await options.summarize({ briefing, items: composed.items });
      if (summary) briefing = { ...briefing, summary, generatedBy: 'cloud' };
    } catch {
      // L2 不可用：保持本地简报，summary=null 由 UI 按结构化数据渲染兜底文案。
    }
  }

  const dayStart = new Date(now());
  dayStart.setHours(0, 0, 0, 0);
  const todayDecisions = await readMessageFilterDecisions(database, ownerId, {
    limit: 500,
    decidedFrom: dayStart.toISOString(),
    decidedTo: generatedAt,
  });
  const snapshot = composeAttentionSnapshot({
    id: createId(),
    generatedAt,
    decisions: todayDecisions,
  });

  const baseEvent = { ownerId, deviceId, schemaVersion: 1 as const, occurredAt: generatedAt };
  const events: NewBriefingEvent[] = [
    {
      ...baseEvent,
      eventId: createId(),
      aggregateId: briefingId,
      aggregateVersion: 1,
      type: 'BriefingGenerationStarted',
      payload: { briefingId, briefingType: options.type },
    },
    {
      ...baseEvent,
      eventId: createId(),
      aggregateId: briefingId,
      aggregateVersion: 2,
      type: 'BriefingGenerated',
      payload: { briefing, items: composed.items },
    },
    {
      ...baseEvent,
      eventId: createId(),
      aggregateId: snapshot.id,
      aggregateVersion: 1,
      type: 'AttentionSnapshotUpdated',
      payload: { snapshot },
    },
  ];
  await persistBriefingGeneration(database, {
    ownerId,
    events,
    briefing,
    items: composed.items,
  });
  return { briefing, items: composed.items, snapshot };
}

export async function listDueBriefingTypes(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
): Promise<ScheduledBriefingType[]> {
  const { ownerId, now } = resolve(context);
  const current = now();
  const minute = current.getHours() * 60 + current.getMinutes();
  const day = current.getDay();
  const schedule = await readBriefingSchedule(database, ownerId);
  const recent = await readBriefings(database, ownerId, { limit: 50 });
  return schedule
    .filter((entry) => (
      entry.enabled &&
      entry.days.includes(day) &&
      entry.minuteOfDay <= minute &&
      !wasGeneratedForSchedule(
        recent,
        entry.briefingType,
        scheduledInstantFor(current, entry.minuteOfDay),
        current,
      )
    ))
    .map((entry) => entry.briefingType);
}

export async function generateDueBriefings(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  options: { summarize?: BriefingSummarizer } = {},
): Promise<GenerateBriefingResult[]> {
  const due = await listDueBriefingTypes(database, context);
  const results: GenerateBriefingResult[] = [];
  for (const type of due) {
    results.push(await generateBriefing(database, context, {
      type,
      summarize: options.summarize,
    }));
  }
  return results;
}

export async function getAttentionSnapshot(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
): Promise<AttentionSnapshot> {
  const { ownerId, now, createId } = resolve(context);
  const generatedAt = now().toISOString();
  const dayStart = new Date(now());
  dayStart.setHours(0, 0, 0, 0);
  const decisions = await readMessageFilterDecisions(database, ownerId, {
    limit: 500,
    decidedFrom: dayStart.toISOString(),
    decidedTo: generatedAt,
  });
  return composeAttentionSnapshot({ id: createId(), generatedAt, decisions });
}

export async function getQuietSummary(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  options: { sinceIso?: string } = {},
): Promise<QuietItemSummary[]> {
  const { ownerId, now } = resolve(context);
  const dayStart = new Date(now());
  dayStart.setHours(0, 0, 0, 0);
  const decisions = await readMessageFilterDecisions(database, ownerId, {
    limit: 500,
    decision: 'suppress',
    decidedFrom: options.sinceIso ?? dayStart.toISOString(),
  });
  return summarizeQuietItems({ decisions });
}

export async function markBriefingOpened(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  briefingId: string,
): Promise<void> {
  const { ownerId, deviceId, now, createId } = resolve(context);
  await appendBriefingEvent(database, {
    ownerId,
    deviceId,
    eventId: createId(),
    aggregateId: briefingId,
    aggregateVersion: 3,
    schemaVersion: 1,
    occurredAt: now().toISOString(),
    type: 'BriefingOpened',
    payload: { briefingId },
  });
}

export async function markBriefingItemHandled(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  input: { briefingId: string; itemId: string; entityId: string },
): Promise<void> {
  const { ownerId, deviceId, now, createId } = resolve(context);
  await persistBriefingItemHandled(database, {
    ownerId,
    briefingId: input.briefingId,
    itemId: input.itemId,
    event: {
      ownerId,
      deviceId,
      eventId: createId(),
      aggregateId: input.briefingId,
      aggregateVersion: 4,
      schemaVersion: 1,
      occurredAt: now().toISOString(),
      type: 'BriefingItemHandled',
      payload: { briefingId: input.briefingId, itemId: input.itemId, entityId: input.entityId },
    },
  });
}

export async function dismissBriefing(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  briefingId: string,
): Promise<void> {
  const { ownerId, deviceId, now, createId } = resolve(context);
  await persistBriefingStatus(database, {
    ownerId,
    briefingId,
    status: 'dismissed',
    event: {
      ownerId,
      deviceId,
      eventId: createId(),
      aggregateId: briefingId,
      aggregateVersion: 5,
      schemaVersion: 1,
      occurredAt: now().toISOString(),
      type: 'BriefingDismissed',
      payload: { briefingId },
    },
  });
}

export async function getBriefing(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  briefingId: string,
): Promise<{ briefing: Briefing; items: BriefingItem[] } | null> {
  const { ownerId } = resolve(context);
  const briefing = await readBriefing(database, ownerId, briefingId);
  if (!briefing) return null;
  const items = await readBriefingItems(database, ownerId, briefingId);
  return { briefing, items };
}

export async function listBriefings(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  options: { coveredFromInclusive?: string; limit?: number } = {},
): Promise<Briefing[]> {
  const { ownerId } = resolve(context);
  return readBriefings(database, ownerId, options);
}

export async function getBriefingSchedule(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
): Promise<BriefingScheduleEntry[]> {
  const { ownerId } = resolve(context);
  return readBriefingSchedule(database, ownerId);
}

export async function updateBriefingSchedule(
  database: BriefingStoreDatabase,
  context: BriefingServiceContext,
  entries: readonly BriefingScheduleEntry[],
): Promise<void> {
  const { ownerId, deviceId, now, createId } = resolve(context);
  const updatedAt = now().toISOString();
  await persistBriefingSchedule(database, {
    ownerId,
    entries,
    updatedAt,
    event: {
      ownerId,
      deviceId,
      eventId: createId(),
      aggregateId: createId(),
      aggregateVersion: 1,
      schemaVersion: 1,
      occurredAt: updatedAt,
      type: 'BriefingScheduleUpdated',
      payload: { entries },
    },
  });
}
