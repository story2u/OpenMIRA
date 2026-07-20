import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type {
  InteractiveAgentApprovalDecisionRequest,
  InteractiveAgentApprovedSend,
  InteractiveAgentApprovedSendRequest,
} from '@story2u/radar-contracts/interactive-agent';
import { describe, expect, it, vi } from 'vitest';

import type { SyncStoreDatabase } from '../../sync/syncStore';

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));

import { createInteractiveApprovedSendCoordinator } from './approvedSend';

const ownerId = '01234567-89ab-4def-8123-456789abcdef';
const opportunityId = '31234567-89ab-4def-8123-456789abcdef';
const approvalId = '41234567-89ab-4def-8123-456789abcdef';

function opportunity(): OpportunityDetail {
  return {
    id: opportunityId,
    opportunityType: 'business',
    platform: 'telegram',
    contactName: 'Customer',
    contactAvatar: '',
    summary: 'Needs a quote',
    matchedKeywords: ['quote'],
    confidenceScore: 0.9,
    status: 'pending',
    internalStatus: 'pending_human',
    priority: 'high',
    lastMessagePreview: 'Please quote',
    createdAt: '2026-07-18T09:00:00Z',
    updatedAt: '2026-07-18T09:01:00Z',
    sourceType: 'private',
    groupName: null,
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: {},
    extractedContacts: {},
    friendRequestStatus: 'n/a',
    sopStage: 'detected',
    trustScore: 88,
    agentActions: [],
    agentAnalysisStatus: 'not_requested',
    agentAnalysisError: null,
    agentAnalyzedAt: null,
    attentionRequired: true,
    archivedAt: null,
    archivedByUserId: null,
    archiveReason: null,
    aiReplyDraft: null,
    finalReply: null,
    detectionReason: null,
    assignedTo: null,
  };
}

function database(): SyncStoreDatabase {
  const detail = opportunity();
  const subject: SyncStoreDatabase = {
    async getAllAsync<Row>() { return [] as Row[]; },
    async getFirstAsync<Row>(source: string) {
      if (source.includes('FROM sync_state')) {
        return { cursor: 8, phase: 'ready', last_error_code: null } as Row;
      }
      if (source.includes('SELECT payload_json')) {
        return { payload_json: JSON.stringify(detail) } as Row;
      }
      if (source.includes('SELECT aggregate_version')) {
        return { aggregate_version: 7, archived_at: null, deleted_at: null } as Row;
      }
      return null;
    },
    async runAsync() {},
    async withExclusiveTransactionAsync(task) { await task(subject); },
  };
  return subject;
}

function sendResult(): InteractiveAgentApprovedSend {
  return {
    approvalId,
    opportunity: { ...opportunity(), internalStatus: 'following', status: 'replied' },
    message: {
      id: '51234567-89ab-4def-8123-456789abcdef',
      senderName: 'Me',
      content: 'Edited exact reply',
      isFromContact: false,
      sentAt: '2026-07-18T10:00:00Z',
      source: 'human',
    },
    messageTotal: 2,
  };
}

describe('interactive approved send coordinator', () => {
  it('binds the edited body and consumes the approval token once', async () => {
    const decide = vi.fn(async (
      _baseUrl: string,
      _turnToken: string,
      _payload: InteractiveAgentApprovalDecisionRequest,
    ) => ({
      id: approvalId,
      status: 'granted' as const,
      toolCallId: 'call-send',
      opportunityId,
      expectedVersion: 7,
      expiresAt: '2026-07-18T10:02:00Z',
      approvalToken: 'approval.payload.signature',
    }));
    const execute = vi.fn(async (
      _baseUrl: string,
      _approvalToken: string,
      _payload: InteractiveAgentApprovedSendRequest,
    ) => sendResult());
    const coordinator = createInteractiveApprovedSendCoordinator({
      baseUrl: 'https://api.example.test',
      database: database(),
      dependencies: { decide, execute },
      ownerId,
      randomId: () => '61234567-89ab-4def-8123-456789abcdef',
      requestApproval: async () => ({ approved: true, text: 'Edited exact reply' }),
      turnToken: 'turn.payload.signature',
    });

    await expect(coordinator.prepare('call-send', {
      opportunity_id: opportunityId,
      text: 'Model proposal',
    })).resolves.toBe(true);
    expect(decide.mock.calls[0]?.[2]).toMatchObject({
      approved: true,
      expectedVersion: 7,
      text: 'Edited exact reply',
    });
    await expect(coordinator.execute('call-send', {
      opportunity_id: opportunityId,
      text: 'Model proposal',
    })).resolves.toEqual({
      approval_id: approvalId,
      message_id: '51234567-89ab-4def-8123-456789abcdef',
      opportunity_id: opportunityId,
      sent: true,
      state: 'sent',
    });
    expect(execute.mock.calls[0]?.[2]).toMatchObject({ text: 'Edited exact reply' });
    await expect(coordinator.execute('call-send', {
      opportunity_id: opportunityId,
      text: 'Model proposal',
    })).rejects.toThrow('approval_missing');
    expect(execute).toHaveBeenCalledTimes(1);
  });

  it('records a denial and never creates executable approval state', async () => {
    const decide = vi.fn(async (
      _baseUrl: string,
      _turnToken: string,
      _payload: InteractiveAgentApprovalDecisionRequest,
    ) => ({
      id: approvalId,
      status: 'denied' as const,
      toolCallId: 'call-denied',
      opportunityId,
      expectedVersion: 7,
      expiresAt: null,
      approvalToken: null,
    }));
    const execute = vi.fn(async (
      _baseUrl: string,
      _approvalToken: string,
      _payload: InteractiveAgentApprovedSendRequest,
    ) => sendResult());
    const coordinator = createInteractiveApprovedSendCoordinator({
      baseUrl: 'https://api.example.test',
      database: database(),
      dependencies: { decide, execute },
      ownerId,
      randomId: () => '61234567-89ab-4def-8123-456789abcdef',
      requestApproval: async (request) => ({ approved: false, text: request.proposedText }),
      turnToken: 'turn.payload.signature',
    });

    await expect(coordinator.prepare('call-denied', {
      opportunity_id: opportunityId,
      text: 'Do not send',
    })).resolves.toBe(false);
    expect(decide.mock.calls[0]?.[2]).toMatchObject({ approved: false, text: 'Do not send' });
    expect(execute).not.toHaveBeenCalled();
  });
});
