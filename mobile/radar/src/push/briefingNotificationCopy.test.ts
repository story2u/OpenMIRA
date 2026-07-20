import type { Briefing } from '@story2u/radar-core/briefing/model';
import { describe, expect, it } from 'vitest';

import { fallbackTranslator } from '../i18n/core';
import { composeBriefingNotificationCopy } from './briefingNotificationCopy';

const briefing: Briefing = {
  id: 'briefing-1',
  type: 'morning',
  title: 'Morning',
  summary: 'A raw summary that is not copied verbatim into push.',
  coveredFrom: '2026-07-20T00:00:00.000Z',
  coveredTo: '2026-07-20T08:30:00.000Z',
  generatedAt: '2026-07-20T08:30:00.000Z',
  generatedBy: 'local',
  status: 'ready',
  totalMessages: 6,
  immediateCount: 2,
  inboxCount: 1,
  digestCount: 3,
  suppressedCount: 4,
  includedMessageIds: ['message-secret-1'],
  includedOpportunityIds: ['opportunity-secret-1'],
  excludedHandledIds: [],
  categorySummaries: [],
  evidenceRefs: [],
};

describe('composeBriefingNotificationCopy', () => {
  it('uses briefing-level counts without exposing message or opportunity ids', () => {
    const copy = composeBriefingNotificationCopy({
      briefing,
      snapshot: {
        id: 'snapshot-1',
        generatedAt: '2026-07-20T08:30:00.000Z',
        totalProcessed: 12,
        localProcessed: 10,
        deepAnalyzed: 2,
        immediateCount: 2,
        inboxCount: 1,
        digestCount: 3,
        suppressedCount: 4,
        needsUserInputCount: 1,
        categoryCounts: [],
      },
    }, fallbackTranslator);

    expect(copy.title).toBe('Mira 简报');
    expect(copy.body).toContain('3');
    expect(JSON.stringify(copy)).not.toContain('message-secret-1');
    expect(JSON.stringify(copy)).not.toContain('opportunity-secret-1');
    expect(copy.data).toEqual({
      kind: 'briefing',
      briefingId: 'briefing-1',
      briefingType: 'morning',
      generatedAt: '2026-07-20T08:30:00.000Z',
    });
  });
});
