import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createOpportunitiesApi,
  dashboardRequestPath,
  decodeDashboardResponse,
  opportunityDetailRequestPath,
} from './opportunities';

const opportunity = {
  id: '01234567-89ab-cdef-0123-456789abcdef',
  platform: 'telegram',
  contactName: 'Example Contact',
  contactAvatar: '',
  summary: 'Needs a reviewed deployment plan',
  matchedKeywords: ['deployment'],
  confidenceScore: 0.88,
  status: 'pending',
  internalStatus: 'pending_human',
  priority: 'high',
  lastMessagePreview: 'Can you help with deployment?',
  createdAt: '2026-07-17T01:00:00Z',
  updatedAt: '2026-07-17T01:05:00+00:00',
  sourceType: 'private',
  groupName: null,
  groupMemberRole: 'member',
  rawMessageLinks: [],
  linkVerification: {},
  extractedContacts: {},
  friendRequestStatus: 'n/a',
  sopStage: 'detected',
  trustScore: 72,
  agentActions: [],
  agentAnalysisStatus: 'not_requested',
  agentAnalysisError: null,
  agentAnalyzedAt: null,
  attentionRequired: true,
  archivedAt: null,
  archivedByUserId: null,
  archiveReason: null,
};

const dashboard = {
  items: [opportunity],
  total: 1,
  limit: 20,
  offset: 0,
  pendingCount: 1,
  attentionItems: [opportunity],
  keywordOptions: ['deployment'],
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createOpportunitiesApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'valid-access-token',
    })),
    fetch,
  };
}

describe('opportunities API', () => {
  it('serializes repeated filters in a stable order and forwards cancellation', async () => {
    const controller = new AbortController();
    const request = fixture(dashboard);
    await request.api.getDashboard({
      status: 'pending',
      platform: 'telegram',
      source_type: 'private',
      trust_levels: ['unverified', 'trusted'],
      sop_stages: ['verified'],
      keywords: ['beta', 'alpha'],
      sort: 'trust',
      limit: 20,
      offset: 40,
    }, { signal: controller.signal });

    expect(request.fetch.mock.calls[0][0]).toBe(
      'https://api.example.test/api/v1/opportunities/dashboard?' +
      'status=pending&platform=telegram&source_type=private&' +
      'trust_levels=trusted&trust_levels=unverified&sop_stages=verified&' +
      'keywords=alpha&keywords=beta&sort=trust&limit=20&offset=40',
    );
    expect(request.fetch.mock.calls[0][1]?.signal).toBe(controller.signal);
  });

  it('strictly decodes dashboard and list responses', async () => {
    await expect(fixture(dashboard).api.getDashboard()).resolves.toMatchObject({ total: 1 });
    await expect(fixture([opportunity]).api.list({ archive: 'all', limit: 200 })).resolves.toHaveLength(1);

    await expect(fixture({ ...dashboard, extra: true }).api.getDashboard()).rejects.toThrow();
    await expect(fixture([{ ...opportunity, trustScore: 101 }]).api.list()).rejects.toThrow();
  });

  it('loads a strict opportunity detail and forwards cancellation', async () => {
    const controller = new AbortController();
    const detail = {
      ...opportunity,
      aiReplyDraft: null,
      finalReply: null,
      detectionReason: 'Matched a reviewed purchasing rule',
      assignedTo: null,
    };
    const request = fixture(detail);

    await expect(request.api.getById(opportunity.id, { signal: controller.signal }))
      .resolves.toMatchObject({ detectionReason: detail.detectionReason });
    expect(request.fetch.mock.calls[0][0]).toBe(
      `https://api.example.test/api/v1/opportunities/${opportunity.id}`,
    );
    expect(request.fetch.mock.calls[0][1]?.signal).toBe(controller.signal);
    await expect(fixture({ ...detail, unexpected: true }).api.getById(opportunity.id))
      .rejects.toThrow();
    await expect(fixture({
      ...detail,
      linkVerification: Object.fromEntries(
        Array.from({ length: 65 }, (_, index) => [`field-${index}`, index]),
      ),
    }).api.getById(opportunity.id)).rejects.toThrow();
  });

  it('rejects inconsistent pagination and duplicate rows', () => {
    expect(() => decodeDashboardResponse({ ...dashboard, total: 0 })).toThrow('inconsistent');
    expect(() => decodeDashboardResponse({
      ...dashboard,
      items: [opportunity, opportunity],
      total: 2,
    })).toThrow('duplicate');
  });

  it('rejects invalid query values before issuing a request', () => {
    expect(() => dashboardRequestPath({ sort: 'random' })).toThrow('sort');
    expect(() => dashboardRequestPath({ created_from: 'not-a-date' })).toThrow('created_from');
    expect(() => dashboardRequestPath({ limit: 101 })).toThrow('limit');
    expect(() => opportunityDetailRequestPath('not-a-uuid')).toThrow('opportunity id');
  });
});
