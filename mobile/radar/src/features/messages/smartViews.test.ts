import type { Opportunity } from '@story2u/radar-contracts/opportunities';
import { describe, expect, it } from 'vitest';

import { buildSmartMessageSections } from './smartViews';

const baseOpportunity: Opportunity = {
  id: '00000000-0000-4000-8000-000000000001',
  opportunityType: 'business',
  platform: 'telegram',
  contactName: 'Example',
  contactAvatar: '',
  summary: 'Summary',
  matchedKeywords: [],
  confidenceScore: 0.8,
  status: 'pending',
  internalStatus: 'pending_human',
  priority: 'normal',
  lastMessagePreview: '',
  createdAt: '2026-07-20T08:00:00.000Z',
  updatedAt: '2026-07-20T08:00:00.000Z',
  sourceType: 'group',
  groupMemberRole: 'member',
  friendRequestStatus: 'not_sent',
  sopStage: 'detected',
  trustScore: 80,
  agentAnalysisStatus: 'completed',
  attentionRequired: false,
};

describe('buildSmartMessageSections', () => {
  it('separates business, jobs, judgment, digest, and quiet counts', () => {
    const sections = buildSmartMessageSections({
      items: [
        baseOpportunity,
        {
          ...baseOpportunity,
          id: '00000000-0000-4000-8000-000000000002',
          opportunityType: 'job',
        },
      ],
      total: 9,
      limit: 20,
      offset: 0,
      pendingCount: 3,
      attentionItems: [{ ...baseOpportunity, opportunityType: 'business', attentionRequired: true }],
      keywordOptions: [],
    }, {
      id: 'snapshot-1',
      generatedAt: '2026-07-20T08:00:00.000Z',
      totalProcessed: 20,
      localProcessed: 18,
      deepAnalyzed: 2,
      immediateCount: 1,
      inboxCount: 2,
      digestCount: 5,
      suppressedCount: 7,
      needsUserInputCount: 4,
      categoryCounts: [],
    });

    expect(sections.map((section) => [section.id, section.target, section.count])).toEqual([
      ['attention', 'pending', 1],
      ['pending', 'pending', 3],
      ['business', 'business', 1],
      ['jobs', 'jobs', 1],
      ['judgment', 'judgment', 4],
      ['digest', 'digest', 5],
      ['quiet', 'quiet', 7],
      ['all', 'all', 9],
    ]);
  });
});
