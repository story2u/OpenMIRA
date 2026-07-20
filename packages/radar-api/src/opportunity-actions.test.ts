import { describe, expect, it, vi } from 'vitest';

import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';

import { createRadarApiClient } from './client';
import {
  createOpportunityActionsApi,
  decodeManualReplyResult,
} from './opportunity-actions';

const opportunityId = '01234567-89ab-cdef-0123-456789abcdef';
const opportunity = {
  id: opportunityId,
  opportunityType: 'business',
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
  updatedAt: '2026-07-17T01:05:00Z',
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
  attentionRequired: false,
  archivedAt: null,
  archivedByUserId: null,
  archiveReason: null,
  aiReplyDraft: null,
  finalReply: null,
  detectionReason: null,
  assignedTo: null,
};
const message = {
  id: '11111111-1111-4111-8111-111111111111',
  senderName: 'Opportunity Assistant',
  content: 'I will prepare the plan.',
  isFromContact: false,
  sentAt: '2026-07-17T01:06:00Z',
  source: 'human',
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createOpportunityActionsApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'valid-access-token',
    })),
    fetch,
  };
}

describe('opportunity actions API', () => {
  it('sends a normalized idempotent manual reply and strictly decodes server truth', async () => {
    const controller = new AbortController();
    const request = fixture({ opportunity, message, messageTotal: 2 });

    await expect(request.api.manualReply(
      opportunityId,
      { text: '  I will prepare the plan.  ' },
      'manual-request-001',
      { signal: controller.signal },
    )).resolves.toMatchObject({ message: { id: message.id } });

    const [url, init] = request.fetch.mock.calls[0];
    expect(url).toBe(
      `https://api.example.test/api/v1/opportunities/${opportunityId}/manual-reply/result`,
    );
    expect(init?.method).toBe('POST');
    expect(init?.signal).toBe(controller.signal);
    expect(new Headers(init?.headers).get('Idempotency-Key')).toBe('manual-request-001');
    expect(JSON.parse(String(init?.body))).toEqual({
      text: 'I will prepare the plan.',
      mark_following: true,
    });
  });

  it('calls AI draft, status and claim endpoints with validated payloads', async () => {
    const draftRequest = fixture({ opportunity_id: opportunityId, draft: 'Reviewed draft' });
    await draftRequest.api.generateAIDraft(opportunityId);
    expect(draftRequest.fetch.mock.calls[0][0]).toContain(`${opportunityId}/ai-draft`);
    expect(draftRequest.fetch.mock.calls[0][1]?.method).toBe('POST');

    const statusRequest = fixture({ ...opportunity, internalStatus: 'following' });
    await statusRequest.api.updateStatus(opportunityId, 'following');
    expect(statusRequest.fetch.mock.calls[0][0]).toContain(`${opportunityId}/status`);
    expect(statusRequest.fetch.mock.calls[0][1]?.method).toBe('PATCH');
    expect(JSON.parse(String(statusRequest.fetch.mock.calls[0][1]?.body))).toEqual({
      status: 'following',
    });

    const queuedStatus = fixture({ ...opportunity, internalStatus: 'following' });
    await queuedStatus.api.updateStatus(opportunityId, 'following', {
      expectedVersion: 7,
      idempotencyKey: 'status-command-001',
    });
    expect(new Headers(queuedStatus.fetch.mock.calls[0][1]?.headers).get('Idempotency-Key'))
      .toBe('status-command-001');
    expect(JSON.parse(String(queuedStatus.fetch.mock.calls[0][1]?.body))).toEqual({
      status: 'following',
      expectedVersion: 7,
    });

    const claimRequest = fixture({ ...opportunity, assignedTo: opportunityId });
    await claimRequest.api.claim(opportunityId);
    expect(claimRequest.fetch.mock.calls[0][0]).toContain(`${opportunityId}/claim`);
    expect(claimRequest.fetch.mock.calls[0][1]?.method).toBe('POST');
  });

  it('rejects malformed inputs and expanded responses before use', async () => {
    const request = fixture({ opportunity, message, messageTotal: 2 });
    expect(() => request.api.manualReply('invalid', { text: 'hello' }, 'request-001')).toThrow(
      'opportunity id',
    );
    expect(() => request.api.manualReply(opportunityId, { text: '   ' }, 'request-001')).toThrow(
      '1 to 4000',
    );
    expect(() => request.api.manualReply(opportunityId, { text: 'hello' }, 'short')).toThrow(
      '8 to 128',
    );
    expect(() => request.api.updateStatus(
      opportunityId,
      'unknown' as InternalOpportunityStatus,
    )).toThrow('status');
    expect(() => request.api.updateStatus(opportunityId, 'following', {
      expectedVersion: 1,
    })).toThrow('provided together');
    expect(() => request.api.updateStatus(opportunityId, 'following', {
      expectedVersion: 0,
      idempotencyKey: 'status-command-001',
    })).toThrow('expected version');
    expect(request.fetch).not.toHaveBeenCalled();

    expect(() => decodeManualReplyResult({
      opportunity,
      message: { ...message, unexpected: true },
      messageTotal: 2,
    }))
      .toThrow();
    expect(() => decodeManualReplyResult({
      opportunity,
      message: { ...message, isFromContact: true },
      messageTotal: 2,
    })).toThrow('incoming');
  });
});
