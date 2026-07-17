import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import { createInteractiveAgentApi } from './interactive-agent';

const turnId = '01234567-89ab-cdef-0123-456789abcdef';
const sessionId = '11234567-89ab-cdef-0123-456789abcdef';
const deviceId = '21234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '31234567-89ab-cdef-0123-456789abcdef';
const approvalId = '41234567-89ab-cdef-0123-456789abcdef';

const turn = {
  id: turnId,
  localSessionId: sessionId,
  deviceId,
  status: 'claimed',
  runtimeVersion: 'pi-0.80.6',
  schemaVersion: 1,
  modelAlias: 'radar-interactive-v1',
  policyVersion: 'interactive-read-only-v1',
  lockVersion: 1,
  requestCount: 0,
  leaseExpiresAt: '2026-07-17T10:05:00Z',
  claimedAt: '2026-07-17T10:00:00Z',
  heartbeatAt: null,
  completedAt: null,
  failedAt: null,
  expiredAt: null,
  failureCode: null,
};

const claim = {
  ...turn,
  turnToken: 'header.payload.signature',
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createInteractiveAgentApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'device-bound-access-token',
    })),
    fetch,
  };
}

describe('interactive Agent API', () => {
  it('strictly decodes content-free turn metadata', async () => {
    await expect(fixture(claim).api.claim({
      localSessionId: sessionId,
      idempotencyKey: 'turn-1',
    })).resolves.toEqual(claim);
    for (const leaked of [
      { prompt: 'local prompt' },
      { messages: [] },
      { toolArgs: {} },
      { toolResult: {} },
      { providerKey: 'secret' },
    ]) {
      await expect(fixture({ ...claim, ...leaked }).api.claim({
        localSessionId: sessionId,
        idempotencyKey: 'turn-1',
      })).rejects.toThrow();
    }
  });

  it('uses the purpose-limited turn token for lifecycle calls', async () => {
    const requests = [
      fixture({ ...turn, status: 'running', lockVersion: 2 }),
      fixture({ ...turn, status: 'completed', completedAt: '2026-07-17T10:01:00Z' }),
      fixture({ ...turn, status: 'failed', failedAt: '2026-07-17T10:01:00Z', failureCode: 'cancelled' }),
    ];
    await requests[0].api.heartbeat(turnId, claim.turnToken, { expectedLockVersion: 1 });
    await requests[1].api.complete(turnId, claim.turnToken, { expectedLockVersion: 1 });
    await requests[2].api.fail(turnId, claim.turnToken, {
      expectedLockVersion: 1,
      failureCode: 'cancelled',
    });

    for (const request of requests) {
      const [, init] = request.fetch.mock.calls[0];
      expect(new Headers(init?.headers).get('authorization')).toBe(
        `Bearer ${claim.turnToken}`,
      );
      expect(new Headers(init?.headers).get('authorization')).not.toContain(
        'device-bound-access-token',
      );
    }
  });

  it('uses device authorization only for claim and explicit expiry', async () => {
    const claimRequest = fixture(claim);
    await claimRequest.api.claim({
      localSessionId: sessionId,
      idempotencyKey: 'turn-1',
    });
    const expiry = fixture({ ...turn, status: 'expired', expiredAt: '2026-07-17T10:06:00Z' });
    await expiry.api.expire(turnId);

    for (const request of [claimRequest, expiry]) {
      const [, init] = request.fetch.mock.calls[0];
      expect(new Headers(init?.headers).get('authorization')).toBe(
        'Bearer device-bound-access-token',
      );
    }
  });

  it('rejects malformed IDs, keys, failure codes and bearer lengths before fetch', () => {
    const request = fixture(turn);
    expect(() => request.api.claim({
      localSessionId: sessionId,
      idempotencyKey: 'contains spaces',
    })).toThrow();
    expect(() => request.api.fail(turnId, claim.turnToken, {
      expectedLockVersion: 1,
      failureCode: 'unsafe failure detail',
    })).toThrow();
    expect(() => request.api.heartbeat('not-a-uuid', claim.turnToken, {
      expectedLockVersion: 1,
    })).toThrow('ID');
    expect(() => request.api.heartbeat(turnId, 'short', {
      expectedLockVersion: 1,
    })).toThrow('token');
    expect(request.fetch).not.toHaveBeenCalled();
  });

  it('keeps decision and execution on separate purpose-limited bearer tokens', async () => {
    const decision = {
      id: approvalId,
      status: 'granted',
      toolCallId: 'call-send-1',
      opportunityId,
      expectedVersion: 3,
      expiresAt: '2026-07-17T10:02:00Z',
      approvalToken: 'approval.payload.signature',
    };
    const approval = fixture(decision);
    await expect(approval.api.decideAction(claim.turnToken, {
      approved: true,
      toolCallId: 'call-send-1',
      opportunityId,
      expectedVersion: 3,
      idempotencyKey: 'agent-reply:1',
      text: 'Approved exact reply',
    })).resolves.toEqual(decision);
    expect(new Headers(approval.fetch.mock.calls[0]?.[1]?.headers).get('authorization')).toBe(
      `Bearer ${claim.turnToken}`,
    );

    const execution = fixture({ approvalId });
    await expect(execution.api.sendApprovedReply(decision.approvalToken, {
      opportunityId,
      expectedVersion: 3,
      idempotencyKey: 'agent-reply:1',
      text: 'Approved exact reply',
    })).rejects.toThrow();
    expect(new Headers(execution.fetch.mock.calls[0]?.[1]?.headers).get('authorization')).toBe(
      `Bearer ${decision.approvalToken}`,
    );
  });

  it('rejects malformed approved action payloads before fetch', () => {
    const request = fixture({});
    expect(() => request.api.decideAction(claim.turnToken, {
      approved: true,
      toolCallId: 'call with spaces',
      opportunityId,
      expectedVersion: 1,
      idempotencyKey: 'agent-reply:1',
      text: 'reply',
    })).toThrow();
    expect(() => request.api.sendApprovedReply('approval.payload.signature', {
      opportunityId,
      expectedVersion: 1,
      idempotencyKey: 'too-short',
      text: '',
    })).toThrow();
    expect(request.fetch).not.toHaveBeenCalled();
  });
});
