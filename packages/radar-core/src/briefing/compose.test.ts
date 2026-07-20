import { describe, expect, it } from 'vitest';

import {
  categoryOfDecision,
  composeAttentionSnapshot,
  composeBriefing,
  summarizeQuietItems,
} from './compose';
import type { DecisionLike } from './model';

function decision(overrides: Partial<DecisionLike> & { messageId: string }): DecisionLike {
  return {
    decision: 'inbox',
    confidence: 0.9,
    reasonSummary: 'matched preference',
    evaluator: 'deterministic',
    decidedAt: '2026-07-18T10:00:00.000Z',
    evidence: [{ kind: 'preference', label: 'purchase_intent' }],
    ...overrides,
  };
}

const WINDOW = { coveredFrom: '2026-07-18T00:00:00.000Z', coveredTo: '2026-07-18T12:00:00.000Z' };

function compose(
  decisions: DecisionLike[],
  overrides: Partial<Parameters<typeof composeBriefing>[0]> = {},
) {
  return composeBriefing({
    id: 'briefing-1',
    type: 'midday',
    title: 'briefing.midday',
    ...WINDOW,
    generatedAt: '2026-07-18T12:00:00.000Z',
    decisions,
    previouslyIncludedIds: new Set(),
    handledIds: new Set(),
    createItemId: (index) => `item-${index}`,
    ...overrides,
  });
}

describe('composeBriefing', () => {
  it('午间简报不重复早间已覆盖的消息', () => {
    const decisions = [
      decision({ messageId: 'm-morning', decidedAt: '2026-07-18T08:00:00.000Z' }),
      decision({ messageId: 'm-new', decidedAt: '2026-07-18T11:00:00.000Z' }),
    ];
    const { briefing } = compose(decisions, { previouslyIncludedIds: new Set(['m-morning']) });
    expect(briefing.includedMessageIds).toEqual(['m-new']);
    expect(briefing.totalMessages).toBe(1);
  });

  it('晚间摘要排除用户已处理的消息并记录排除原因', () => {
    const decisions = [
      decision({ messageId: 'm-handled' }),
      decision({ messageId: 'm-open' }),
    ];
    const { briefing } = compose(decisions, { type: 'evening', handledIds: new Set(['m-handled']) });
    expect(briefing.includedMessageIds).toEqual(['m-open']);
    expect(briefing.excludedHandledIds).toEqual(['m-handled']);
  });

  it('窗口之外与重复 messageId 的决策被忽略', () => {
    const decisions = [
      decision({ messageId: 'm-out', decidedAt: '2026-07-17T09:00:00.000Z' }),
      decision({ messageId: 'm-dup' }),
      decision({ messageId: 'm-dup', decidedAt: '2026-07-18T11:30:00.000Z' }),
    ];
    const { briefing } = compose(decisions);
    expect(briefing.includedMessageIds).toEqual(['m-dup']);
  });

  it('item 按优先级排序：immediate 在前，低置信度边界消息标记 needs_judgment，suppress 不进条目', () => {
    const decisions = [
      decision({ messageId: 'm-suppressed', decision: 'suppress' }),
      decision({ messageId: 'm-boundary', decision: 'inbox', confidence: 0.4 }),
      decision({ messageId: 'm-urgent', decision: 'immediate' }),
    ];
    const { briefing, items } = compose(decisions);
    expect(items.map((item) => item.entityId)).toEqual(['m-urgent', 'm-boundary']);
    expect(items[0].priority).toBe('action_required');
    expect(items[0].actionRequired).toBe(true);
    expect(items[1].priority).toBe('needs_judgment');
    expect(briefing.suppressedCount).toBe(1);
  });

  it('summary 恒为 null（本地兜底），关联商机映射为 opportunity 条目', () => {
    const { briefing, items } = compose([decision({ messageId: 'm-1' })], {
      opportunityIdByMessageId: new Map([['m-1', 'opp-1']]),
    });
    expect(briefing.summary).toBeNull();
    expect(briefing.generatedBy).toBe('local');
    expect(items[0]).toMatchObject({ itemType: 'opportunity', entityId: 'opp-1' });
    expect(briefing.includedOpportunityIds).toEqual(['opp-1']);
  });
});

describe('composeAttentionSnapshot', () => {
  it('按投递模式与评估器统计，并计入边界消息', () => {
    const snapshot = composeAttentionSnapshot({
      id: 'snap-1',
      generatedAt: '2026-07-18T12:00:00.000Z',
      decisions: [
        decision({ messageId: 'a', decision: 'immediate' }),
        decision({ messageId: 'b', decision: 'suppress', evaluator: 'cloud_agent' }),
        decision({ messageId: 'c', decision: 'digest', confidence: 0.3 }),
      ],
    });
    expect(snapshot).toMatchObject({
      totalProcessed: 3,
      localProcessed: 2,
      deepAnalyzed: 1,
      immediateCount: 1,
      digestCount: 1,
      suppressedCount: 1,
      needsUserInputCount: 1,
    });
    expect(snapshot.categoryCounts[0]).toEqual({ category: 'purchase_intent', count: 3 });
  });
});

describe('summarizeQuietItems', () => {
  it('只统计 suppress、按类别聚合并保留有限样本', () => {
    const ad = (id: string) =>
      decision({
        messageId: id,
        decision: 'suppress',
        reasonSummary: 'advertising content',
        evidence: [{ kind: 'message_signal', label: 'advertising' }],
      });
    const summaries = summarizeQuietItems({
      decisions: [ad('q1'), ad('q2'), ad('q3'), decision({ messageId: 'kept' })],
      sourceOfMessage: (id) => (id === 'q1' ? 'group-a' : 'group-b'),
      sampleLimit: 2,
    });
    expect(summaries).toHaveLength(1);
    expect(summaries[0]).toMatchObject({
      category: 'advertising',
      count: 3,
      sourceCount: 2,
      topReason: 'advertising content',
    });
    expect(summaries[0].samples).toEqual(['q1', 'q2']);
  });

  it('无概念证据时归入 other', () => {
    expect(categoryOfDecision(decision({ messageId: 'x', evidence: [] }))).toBe('other');
  });
});
