import { describe, expect, it } from 'vitest';

import { composeBriefing } from './compose';
import type { BriefingEvent } from './events';
import { foldBriefings, handledEntityIds, includedMessageIdsSince } from './fold';
import type { DecisionLike } from './model';

let sequence = 0;
function event<Type extends BriefingEvent['type']>(
  type: Type,
  payload: Extract<BriefingEvent, { type: Type }>['payload'],
  overrides: Partial<Pick<BriefingEvent, 'eventId' | 'sequence'>> = {},
): BriefingEvent {
  sequence += 1;
  return {
    eventId: overrides.eventId ?? `evt-${sequence}`,
    ownerId: 'owner-1',
    deviceId: 'device-1',
    sequence: overrides.sequence ?? sequence,
    aggregateId: 'agg-1',
    aggregateVersion: 1,
    schemaVersion: 1,
    occurredAt: '2026-07-18T12:00:00.000Z',
    type,
    payload,
  } as BriefingEvent;
}

function generated(id: string, messageIds: string[], coveredTo = '2026-07-18T12:00:00.000Z') {
  const decisions: DecisionLike[] = messageIds.map((messageId) => ({
    messageId,
    decision: 'inbox',
    confidence: 0.9,
    reasonSummary: 'r',
    evaluator: 'deterministic',
    decidedAt: '2026-07-18T08:00:00.000Z',
    evidence: [],
  }));
  const { briefing, items } = composeBriefing({
    id,
    type: 'midday',
    title: 't',
    coveredFrom: '2026-07-18T00:00:00.000Z',
    coveredTo,
    generatedAt: coveredTo,
    decisions,
    previouslyIncludedIds: new Set(),
    handledIds: new Set(),
    createItemId: (index) => `${id}-item-${index}`,
  });
  return { briefing, items };
}

describe('foldBriefings', () => {
  it('同 eventId 幂等，乱序输入收敛到同一状态', () => {
    const { briefing, items } = generated('b-1', ['m-1', 'm-2']);
    const events = [
      event('BriefingGenerated', { briefing, items }, { eventId: 'e-gen', sequence: 1 }),
      event('BriefingItemHandled', { briefingId: 'b-1', itemId: 'b-1-item-0', entityId: 'm-1' }, { eventId: 'e-handle', sequence: 2 }),
      event('BriefingItemHandled', { briefingId: 'b-1', itemId: 'b-1-item-0', entityId: 'm-1' }, { eventId: 'e-handle', sequence: 2 }),
    ];
    const forward = foldBriefings(events);
    const shuffled = foldBriefings([events[2], events[0], events[1]]);
    expect(forward.itemsByBriefing.get('b-1')?.[0]?.handled).toBe(true);
    expect(forward.appliedEventIds.size).toBe(2);
    expect([...shuffled.appliedEventIds]).toEqual([...forward.appliedEventIds]);
    expect(handledEntityIds(forward)).toEqual(new Set(['m-1']));
  });

  it('快照取 generatedAt 最新一份；schedule 以最后事件为准；dismissed 改状态不删数据', () => {
    const { briefing, items } = generated('b-1', ['m-1']);
    const snapshot = (generatedAt: string, total: number) => ({
      id: `s-${total}`,
      generatedAt,
      totalProcessed: total,
      localProcessed: total,
      deepAnalyzed: 0,
      immediateCount: 0,
      inboxCount: total,
      digestCount: 0,
      suppressedCount: 0,
      needsUserInputCount: 0,
      categoryCounts: [],
    });
    const state = foldBriefings([
      event('BriefingGenerated', { briefing, items }),
      event('AttentionSnapshotUpdated', { snapshot: snapshot('2026-07-18T12:00:00.000Z', 10) }),
      event('AttentionSnapshotUpdated', { snapshot: snapshot('2026-07-18T09:00:00.000Z', 5) }),
      event('BriefingDismissed', { briefingId: 'b-1' }),
      event('BriefingScheduleUpdated', {
        entries: [{ briefingType: 'morning', minuteOfDay: 9 * 60, days: [1, 2, 3, 4, 5], enabled: true }],
      }),
    ]);
    expect(state.latestSnapshot?.totalProcessed).toBe(10);
    expect(state.briefings.get('b-1')?.status).toBe('dismissed');
    expect(state.schedule).toHaveLength(1);
    expect(state.schedule[0]?.minuteOfDay).toBe(540);
  });

  it('includedMessageIdsSince 汇总窗口内未撤销简报的覆盖 id，供增量去重', () => {
    const first = generated('b-1', ['m-1'], '2026-07-18T08:30:00.000Z');
    const second = generated('b-2', ['m-2'], '2026-07-18T12:00:00.000Z');
    const state = foldBriefings([
      event('BriefingGenerated', first),
      event('BriefingGenerated', second),
    ]);
    const ids = includedMessageIdsSince(state, '2026-07-18T00:00:00.000Z');
    expect(ids).toEqual(new Set(['m-1', 'm-2']));
    // 早于窗口起点的简报不参与
    expect(includedMessageIdsSince(state, '2026-07-18T10:00:00.000Z')).toEqual(new Set(['m-2']));
  });
});
