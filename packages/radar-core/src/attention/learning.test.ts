import { describe, expect, it } from 'vitest';

import { selectTeachingCards, summarizeTeachingExamples, type TeachingCandidate } from './learning';

function candidate(
  id: number,
  overrides: Partial<TeachingCandidate> = {},
): TeachingCandidate {
  return {
    messageId: `message-${id}`,
    sourceKey: `source-${id % 3}`,
    topicKeys: [`topic-${id % 4}`],
    confidence: 0.5,
    currentDecision: 'inbox',
    candidateDecision: 'inbox',
    openedRecently: false,
    ignoredRecently: false,
    duplicate: false,
    likelyNoise: false,
    highValueDomain: false,
    allowedForTeaching: true,
    sensitive: false,
    ...overrides,
  };
}

describe('active teaching selection', () => {
  it('prioritizes boundaries and decision changes while enforcing source diversity', () => {
    const selected = selectTeachingCards([
      candidate(1, { confidence: 0.51, currentDecision: 'suppress', candidateDecision: 'inbox' }),
      candidate(2, {
        confidence: 0.49,
        openedRecently: true,
        currentDecision: 'digest',
        candidateDecision: 'digest',
      }),
      candidate(3, { confidence: 0.99 }),
      candidate(4, { confidence: 0.52, sensitive: true }),
      ...Array.from({ length: 12 }, (_, index) => candidate(index + 10)),
    ], { targetCount: 8, maximumPerSource: 2 });

    expect(selected).toHaveLength(6);
    expect(selected[0]?.messageId).toBe('message-1');
    expect(selected.some((item) => item.messageId === 'message-4')).toBe(false);
    expect(Math.max(...['source-0', 'source-1', 'source-2'].map(
      (source) => selected.filter((item) => item.sourceKey === source).length,
    ))).toBeLessThanOrEqual(2);
  });

  it('summarizes only non-reverted positive and negative teaching reasons', () => {
    const summary = summarizeTeachingExamples([
      {
        id: 'a', messageId: 'm1', label: 'positive', selectedReasons: ['needs_reply'],
        freeformReason: null, capturedAt: 'now', teachingSessionId: 's', revertedAt: null,
      },
      {
        id: 'b', messageId: 'm2', label: 'negative', selectedReasons: ['advertising'],
        freeformReason: null, capturedAt: 'now', teachingSessionId: 's', revertedAt: null,
      },
      {
        id: 'c', messageId: 'm3', label: 'negative', selectedReasons: ['training'],
        freeformReason: null, capturedAt: 'now', teachingSessionId: 's', revertedAt: 'later',
      },
      {
        id: 'd', messageId: 'm4', label: 'skipped', selectedReasons: [],
        freeformReason: null, capturedAt: 'now', teachingSessionId: 's', revertedAt: null,
      },
    ]);

    expect(summary).toEqual({
      increase: ['needs_reply'], reduce: ['advertising'],
      positiveCount: 1, negativeCount: 1, skippedCount: 1,
    });
  });
});
