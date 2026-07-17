import type {
  InteractiveAgentApprovalDecisionRequest,
  InteractiveAgentApprovedSendRequest,
  InteractiveAgentTurnClaim,
} from '@story2u/radar-contracts/interactive-agent';
import {
  INTERACTIVE_APPROVED_SEND_SYSTEM_PROMPT,
  INTERACTIVE_INTERNAL_SYSTEM_PROMPT,
} from '@story2u/radar-agent/interactive';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import { describe, expect, it, vi } from 'vitest';

import type { SyncStoreExecutor } from '../../sync/syncStore';

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));

import {
  boundedInteractiveContext,
  localEntriesToAgentMessages,
  runInteractiveAgentHost,
} from './host';

const ownerId = '01234567-89ab-4def-8123-456789abcdef';
const sessionId = '11234567-89ab-4def-8123-456789abcdef';
const turnId = '21234567-89ab-4def-8123-456789abcdef';
const opportunityId = '31234567-89ab-4def-8123-456789abcdef';
const turnToken = 'interactive-turn-token-long-enough';

function claim(): InteractiveAgentTurnClaim {
  return {
    id: turnId,
    localSessionId: sessionId,
    deviceId: '41234567-89ab-4def-8123-456789abcdef',
    status: 'running',
    runtimeVersion: 'pi-0.80.6',
    schemaVersion: 1,
    modelAlias: 'radar-interactive-v1',
    policyVersion: 'interactive-read-only-v1',
    lockVersion: 2,
    requestCount: 0,
    leaseExpiresAt: '2026-07-17T10:05:00Z',
    claimedAt: '2026-07-17T10:00:00Z',
    heartbeatAt: '2026-07-17T10:00:01Z',
    completedAt: null,
    failedAt: null,
    expiredAt: null,
    failureCode: null,
    turnToken,
  };
}

function response(chunks: object[]) {
  const data = [
    ...chunks.map((chunk) => JSON.stringify(chunk)),
    '[DONE]',
  ].map((item) => `data: ${item}\n\n`).join('');
  return {
    ok: true,
    status: 200,
    headers: new Headers({ 'content-type': 'text/event-stream; charset=utf-8' }),
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        const bytes = new TextEncoder().encode(data);
        controller.enqueue(bytes.slice(0, 31));
        controller.enqueue(bytes.slice(31));
        controller.close();
      },
    }),
  };
}

function toolResponse() {
  return response([
    {
      model: 'radar-interactive-v1',
      choices: [{
        index: 0,
        delta: {
          role: 'assistant',
          tool_calls: [{
            index: 0,
            id: 'call-local-opportunity',
            type: 'function',
            function: {
              name: 'get_opportunity',
              arguments: JSON.stringify({ opportunity_id: opportunityId }),
            },
          }],
        },
        finish_reason: null,
      }],
    },
    {
      model: 'radar-interactive-v1',
      choices: [{ index: 0, delta: {}, finish_reason: 'tool_calls' }],
    },
  ]);
}

function answerResponse() {
  return response([
    {
      model: 'radar-interactive-v1',
      choices: [{
        index: 0,
        delta: { role: 'assistant', content: 'No matching local opportunity was found.' },
        finish_reason: null,
      }],
    },
    {
      model: 'radar-interactive-v1',
      choices: [{ index: 0, delta: {}, finish_reason: 'stop' }],
    },
    {
      model: 'radar-interactive-v1',
      choices: [],
      usage: { prompt_tokens: 20, completion_tokens: 8, total_tokens: 28 },
    },
  ]);
}

function claimToolResponse() {
  return response([
    {
      model: 'radar-interactive-v1',
      choices: [{
        index: 0,
        delta: {
          role: 'assistant',
          tool_calls: [{
            index: 0,
            id: 'call-claim-opportunity',
            type: 'function',
            function: {
              name: 'claim_opportunity',
              arguments: JSON.stringify({ opportunity_id: opportunityId }),
            },
          }],
        },
        finish_reason: null,
      }],
    },
    {
      model: 'radar-interactive-v1',
      choices: [{ index: 0, delta: {}, finish_reason: 'tool_calls' }],
    },
  ]);
}

function sendToolResponse() {
  return response([
    {
      model: 'radar-interactive-v1',
      choices: [{
        index: 0,
        delta: {
          role: 'assistant',
          tool_calls: [{
            index: 0,
            id: 'call-send-reply',
            type: 'function',
            function: {
              name: 'send_reply',
              arguments: JSON.stringify({
                opportunity_id: opportunityId,
                text: 'Model-proposed reply',
              }),
            },
          }],
        },
        finish_reason: null,
      }],
    },
    {
      model: 'radar-interactive-v1',
      choices: [{ index: 0, delta: {}, finish_reason: 'tool_calls' }],
    },
  ]);
}

function opportunity(): OpportunityDetail {
  return {
    id: opportunityId,
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

describe('interactive RN pi host', () => {
  it('executes only the local read-only tool and persists an app-defined transcript', async () => {
    const requests: Array<{ url: string; payload: Record<string, unknown> }> = [];
    const fetch = vi.fn(async (url: string, init: RequestInit) => {
      requests.push({ url, payload: JSON.parse(String(init.body)) });
      return requests.length === 1 ? toolResponse() : answerResponse();
    });
    const database = {
      async getAllAsync<Row>() { return [] as Row[]; },
      async getFirstAsync<Row>(source: string) {
        if (source.includes('FROM sync_state')) {
          return { cursor: 10, phase: 'ready', last_error_code: null } as Row;
        }
        return null;
      },
      async runAsync() {},
      async withExclusiveTransactionAsync(
        task: (transaction: SyncStoreExecutor) => Promise<void>,
      ) {
        await task(database);
      },
    };
    const streamed: string[] = [];
    const result = await runInteractiveAgentHost({
      baseUrl: 'https://api.example.test',
      claim: claim(),
      database,
      entries: [{
        ownerId,
        sessionId,
        seq: 1,
        content: { type: 'user', text: 'Open the opportunity.' },
        createdAt: '2026-07-17T10:00:00Z',
      }],
      fetch,
      onStreamText: (text) => streamed.push(text),
      ownerId,
      randomId: () => '51234567-89ab-4def-8123-456789abcdef',
    });

    expect(requests).toHaveLength(2);
    expect(requests[0]?.url).toBe(
      'https://api.example.test/api/v1/agent/interactive/gateway/v1/chat/completions',
    );
    expect(requests[0]?.payload).toMatchObject({
      model: 'radar-interactive-v1',
      store: false,
      parallel_tool_calls: false,
    });
    expect(requests[1]?.payload.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({ role: 'tool', content: '{"opportunity":null}' }),
    ]));
    expect(result.finalText).toBe('No matching local opportunity was found.');
    expect(result.entries.map((entry) => entry.type)).toEqual([
      'tool_call',
      'tool_result',
      'assistant',
    ]);
    expect(streamed.at(-1)).toBe(result.finalText);
  });

  it('selects the v2 contract per turn and executes only the authenticated internal tool', async () => {
    const requests: Array<Record<string, unknown>> = [];
    const fetch = vi.fn(async (_url: string, init: RequestInit) => {
      requests.push(JSON.parse(String(init.body)));
      return requests.length === 1 ? claimToolResponse() : response([
        {
          model: 'radar-interactive-v1',
          choices: [{
            index: 0,
            delta: { role: 'assistant', content: 'The opportunity is now claimed.' },
            finish_reason: null,
          }],
        },
        {
          model: 'radar-interactive-v1',
          choices: [{ index: 0, delta: {}, finish_reason: 'stop' }],
        },
      ]);
    });
    const database = {
      async getAllAsync<Row>() { return [] as Row[]; },
      async getFirstAsync<Row>(source: string) {
        if (source.includes('FROM sync_state')) {
          return { cursor: 10, phase: 'ready', last_error_code: null } as Row;
        }
        if (source.includes('FROM opportunity_projection')) {
          return { payload_json: JSON.stringify(opportunity()) } as Row;
        }
        return null;
      },
      async runAsync() {},
      async withExclusiveTransactionAsync(
        task: (transaction: SyncStoreExecutor) => Promise<void>,
      ) {
        await task(database);
      },
    };
    const claimRequest = vi.fn(async () => opportunity());

    const result = await runInteractiveAgentHost({
      baseUrl: 'https://api.example.test',
      claim: {
        ...claim(),
        schemaVersion: 2,
        policyVersion: 'interactive-internal-v2',
      },
      database,
      entries: [{
        ownerId,
        sessionId,
        seq: 1,
        content: { type: 'user', text: 'Claim this opportunity.' },
        createdAt: '2026-07-18T10:00:00Z',
      }],
      fetch,
      internalToolDependencies: { claim: claimRequest },
      ownerId,
      randomId: () => '51234567-89ab-4def-8123-456789abcdef',
    });

    const firstRequest = requests[0] as {
      messages: Array<{ content: string; role: string }>;
      tools: Array<{ function: { name: string } }>;
    };
    expect(firstRequest.messages[0]).toEqual({
      role: 'system',
      content: INTERACTIVE_INTERNAL_SYSTEM_PROMPT,
    });
    expect(firstRequest.tools.map((tool) => tool.function.name)).toEqual([
      'search_opportunities',
      'get_opportunity',
      'get_messages',
      'draft_reply',
      'update_status',
      'claim_opportunity',
    ]);
    expect(claimRequest).toHaveBeenCalledWith(
      'https://api.example.test',
      opportunityId,
      expect.any(AbortSignal),
    );
    expect(result.entries).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'tool_call', toolName: 'claim_opportunity' }),
      expect.objectContaining({
        type: 'tool_result',
        toolName: 'claim_opportunity',
        result: { opportunity_id: opportunityId, claimed: true, state: 'confirmed' },
      }),
    ]));
  });

  it('pauses v3 send_reply for edited approval before executing once', async () => {
    const requests: Array<Record<string, unknown>> = [];
    const fetch = vi.fn(async (_url: string, init: RequestInit) => {
      requests.push(JSON.parse(String(init.body)));
      return requests.length === 1 ? sendToolResponse() : answerResponse();
    });
    const database = {
      async getAllAsync<Row>() { return [] as Row[]; },
      async getFirstAsync<Row>(source: string) {
        if (source.includes('FROM sync_state')) {
          return { cursor: 10, phase: 'ready', last_error_code: null } as Row;
        }
        if (source.includes('SELECT payload_json')) {
          return { payload_json: JSON.stringify(opportunity()) } as Row;
        }
        if (source.includes('SELECT aggregate_version')) {
          return { aggregate_version: 4, archived_at: null, deleted_at: null } as Row;
        }
        return null;
      },
      async runAsync() {},
      async withExclusiveTransactionAsync(
        task: (transaction: SyncStoreExecutor) => Promise<void>,
      ) {
        await task(database);
      },
    };
    const decide = vi.fn(async (
      _baseUrl: string,
      _token: string,
      payload: InteractiveAgentApprovalDecisionRequest,
    ) => ({
      id: '51234567-89ab-4def-8123-456789abcdef',
      status: 'granted' as const,
      toolCallId: payload.toolCallId,
      opportunityId: payload.opportunityId,
      expectedVersion: payload.expectedVersion,
      expiresAt: '2026-07-18T10:02:00Z',
      approvalToken: 'approval.payload.signature',
    }));
    const execute = vi.fn(async (
      _baseUrl: string,
      _token: string,
      payload: InteractiveAgentApprovedSendRequest,
    ) => ({
      approvalId: '51234567-89ab-4def-8123-456789abcdef',
      opportunity: { ...opportunity(), internalStatus: 'following' as const, status: 'replied' as const },
      message: {
        id: '61234567-89ab-4def-8123-456789abcdef',
        senderName: 'Me',
        content: payload.text,
        isFromContact: false,
        sentAt: '2026-07-18T10:00:00Z',
        source: 'human' as const,
      },
      messageTotal: 2,
    }));

    const result = await runInteractiveAgentHost({
      approvedSendDependencies: { decide, execute },
      baseUrl: 'https://api.example.test',
      claim: {
        ...claim(),
        schemaVersion: 3,
        policyVersion: 'interactive-approved-send-v3',
      },
      database,
      entries: [{
        ownerId,
        sessionId,
        seq: 1,
        content: { type: 'user', text: 'Send this reply.' },
        createdAt: '2026-07-18T10:00:00Z',
      }],
      fetch,
      ownerId,
      randomId: () => '71234567-89ab-4def-8123-456789abcdef',
      requestApproval: async () => ({ approved: true, text: 'User-edited exact reply' }),
    });

    expect((requests[0]?.messages as Array<Record<string, unknown>>)[0]).toEqual({
      role: 'system',
      content: INTERACTIVE_APPROVED_SEND_SYSTEM_PROMPT,
    });
    expect(decide.mock.calls[0]?.[2]).toMatchObject({
      expectedVersion: 4,
      text: 'User-edited exact reply',
    });
    expect(execute.mock.calls[0]?.[2]).toMatchObject({ text: 'User-edited exact reply' });
    expect(execute).toHaveBeenCalledTimes(1);
    expect(result.entries).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'tool_call', toolName: 'send_reply' }),
      expect.objectContaining({
        type: 'tool_result',
        toolName: 'send_reply',
        result: expect.objectContaining({ sent: true, state: 'sent' }),
      }),
    ]));
  });

  it('rehydrates complete tool pairs and bounds old turns without raw session JSON', () => {
    const model = {
      id: 'radar-interactive-v1',
      name: 'Gateway',
      api: 'radar-interactive-gateway',
      provider: 'radar-interactive-gateway',
      baseUrl: 'https://api.example.test',
      reasoning: false,
      input: ['text'] as Array<'text' | 'image'>,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 64_000,
      maxTokens: 4_096,
    };
    const messages = localEntriesToAgentMessages([
      {
        ownerId,
        sessionId,
        seq: 1,
        content: { type: 'user', text: 'Read one.' },
        createdAt: '2026-07-17T10:00:00Z',
      },
      {
        ownerId,
        sessionId,
        seq: 2,
        content: {
          type: 'tool_call',
          toolCallId: 'call-1',
          toolName: 'get_opportunity',
          arguments: { opportunity_id: opportunityId },
        },
        createdAt: '2026-07-17T10:00:01Z',
      },
      {
        ownerId,
        sessionId,
        seq: 3,
        content: {
          type: 'tool_result',
          toolCallId: 'call-1',
          toolName: 'get_opportunity',
          result: { opportunity: null },
        },
        createdAt: '2026-07-17T10:00:02Z',
      },
    ], model);
    expect(messages.map((message) => message.role)).toEqual(['user', 'assistant', 'toolResult']);

    const oversizedHistory = Array.from({ length: 20 }, (_, index) => ([
      { role: 'user' as const, content: `question ${index}`, timestamp: index * 2 },
      {
        role: 'assistant' as const,
        content: [{ type: 'text' as const, text: 'x'.repeat(4_000) }],
        api: model.api,
        provider: model.provider,
        model: model.id,
        usage: {
          input: 0,
          output: 0,
          cacheRead: 0,
          cacheWrite: 0,
          totalTokens: 0,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
        },
        stopReason: 'stop' as const,
        timestamp: index * 2 + 1,
      },
    ])).flat();
    const bounded = boundedInteractiveContext(oversizedHistory);
    expect(bounded[0]?.role).toBe('user');
    expect(bounded.at(-1)?.role).toBe('assistant');
    expect(bounded.length).toBeLessThanOrEqual(28);
    expect(JSON.stringify(bounded).length).toBeLessThan(60_000);
  });
});
