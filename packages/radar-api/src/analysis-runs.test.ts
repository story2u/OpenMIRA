import { describe, expect, it, vi } from 'vitest';

import { createAnalysisRunsApi } from './analysis-runs';
import { createRadarApiClient } from './client';

const runId = '01234567-89ab-cdef-0123-456789abcdef';
const messageId = '11234567-89ab-cdef-0123-456789abcdef';
const deviceId = '21234567-89ab-cdef-0123-456789abcdef';

const run = {
  id: runId,
  messageId,
  deviceId,
  status: 'claimed',
  executedBy: 'device',
  mode: 'primary',
  runtimeVersion: 'pi-0.80.6',
  schemaVersion: 1,
  modelAlias: 'radar-analysis-v1',
  policyVersion: 'agent-policy-v1',
  sourceMessageVersion: 1,
  lockVersion: 1,
  leaseExpiresAt: '2026-07-17T10:02:00Z',
  claimedAt: '2026-07-17T10:00:00Z',
  heartbeatAt: null,
  completedAt: null,
  failedAt: null,
  expiredAt: null,
  failureCode: null,
  shadowMatch: null,
  shadowDifferenceCount: null,
};

const claim = {
  ...run,
  runToken: 'header.payload.signature',
  input: {
    messageId,
    sourceMessageVersion: 1,
    channel: 'telegram',
    senderDisplayName: 'Customer',
    sourceType: 'private',
    groupName: null,
    text: 'Need a quote',
    links: [],
  },
};

const analysis = {
  is_opportunity: true,
  confidence: 0.9,
  title: 'Quote request',
  summary: 'The customer requested a quote.',
  priority: 'high' as const,
  trust_score: 80,
  attention_required: true,
  link_status: 'unverified' as const,
  link_summary: null,
  risk_flags: [],
  contacts: {
    email: null,
    phone: null,
    telegram_handle: null,
    wecom_id: null,
    extraction_source: null,
  },
  actions: [],
};

const linkEvidence = {
  runId,
  sourceMessageVersion: 1,
  fetchedAt: '2026-07-17T10:00:10Z',
  evidence: [{
    url: 'https://example.test/rfp',
    final_url: 'https://example.test/rfp',
    status: 'safe',
    http_status: 200,
    content_type: 'text/plain',
    title: 'RFP',
    text: 'Public evidence',
    emails: [],
    risk_reasons: [],
  }],
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createAnalysisRunsApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'device-bound-access-token',
    })),
    fetch,
  };
}

describe('analysis runs API', () => {
  it('strictly decodes a bounded claim without provider or raw payload fields', async () => {
    await expect(fixture(claim).api.claim({ messageId })).resolves.toEqual(claim);
    await expect(fixture({ ...claim, providerModel: 'secret-model' }).api.claim({ messageId }))
      .rejects.toThrow();
    await expect(fixture({
      ...claim,
      input: { ...claim.input, rawPayload: { secret: true } },
    }).api.claim({ messageId })).rejects.toThrow();
  });

  it('uses the purpose-limited run token for heartbeat and complete', async () => {
    const heartbeat = fixture({ ...run, status: 'running', lockVersion: 2 });
    await heartbeat.api.heartbeat(runId, claim.runToken, { expectedLockVersion: 1 });
    const complete = fixture({
      ...run,
      status: 'completed',
      lockVersion: 2,
      completedAt: '2026-07-17T10:00:30Z',
    });
    await complete.api.complete(runId, claim.runToken, {
      expectedLockVersion: 1,
      result: analysis,
    });

    for (const request of [heartbeat, complete]) {
      const [, init] = request.fetch.mock.calls[0];
      expect(new Headers(init?.headers).get('authorization')).toBe(
        `Bearer ${claim.runToken}`,
      );
      expect(new Headers(init?.headers).get('authorization')).not.toContain(
        'device-bound-access-token',
      );
    }
  });

  it('fetches only server-derived link evidence with the run token and no URL body', async () => {
    const request = fixture(linkEvidence);

    await expect(request.api.inspectLinks(runId, claim.runToken)).resolves.toEqual(linkEvidence);

    const [, init] = request.fetch.mock.calls[0];
    expect(new Headers(init?.headers).get('authorization')).toBe(`Bearer ${claim.runToken}`);
    expect(init?.body).toBeUndefined();
    expect(request.fetch.mock.calls[0][0]).toContain(`/agent/runs/${runId}/links/inspect`);
    await expect(fixture({
      ...linkEvidence,
      evidence: [{ ...linkEvidence.evidence[0], providerSecret: 'leak' }],
    }).api.inspectLinks(runId, claim.runToken)).rejects.toThrow();
  });

  it('rejects malformed results, failure codes, IDs and bearer lengths before fetch', () => {
    const request = fixture(run);
    expect(() => request.api.complete(runId, claim.runToken, {
      expectedLockVersion: 1,
      result: { ...analysis, extra: true } as never,
    })).toThrow();
    expect(() => request.api.fail(runId, claim.runToken, {
      expectedLockVersion: 1,
      failureCode: 'unsafe failure detail',
    })).toThrow();
    expect(() => request.api.heartbeat('not-a-uuid', claim.runToken, {
      expectedLockVersion: 1,
    })).toThrow('ID');
    expect(() => request.api.heartbeat(runId, 'short', {
      expectedLockVersion: 1,
    })).toThrow('token');
    expect(request.fetch).not.toHaveBeenCalled();
  });

  it('uses the device access token only for claim and explicit expiry', async () => {
    const claimRequest = fixture(claim);
    await claimRequest.api.claim({ messageId });
    const expiry = fixture({ ...run, status: 'expired', expiredAt: '2026-07-17T10:03:00Z' });
    await expiry.api.expire(runId);

    for (const request of [claimRequest, expiry]) {
      const [, init] = request.fetch.mock.calls[0];
      expect(new Headers(init?.headers).get('authorization')).toBe(
        'Bearer device-bound-access-token',
      );
    }
  });

  it('claims at most one server-selected shadow candidate without a message payload', async () => {
    const candidate = fixture({ claim: { ...claim, mode: 'shadow' } });
    await expect(candidate.api.claimShadow()).resolves.toMatchObject({
      id: runId,
      mode: 'shadow',
    });
    const empty = fixture({ claim: null });
    await expect(empty.api.claimShadow()).resolves.toBeNull();

    for (const request of [candidate, empty]) {
      const [url, init] = request.fetch.mock.calls[0];
      expect(url).toContain('/agent/runs/claim-shadow');
      expect(init?.body).toBeUndefined();
      expect(new Headers(init?.headers).get('authorization')).toBe(
        'Bearer device-bound-access-token',
      );
    }
  });

  it('claims at most one server-selected primary candidate without a message payload', async () => {
    const candidate = fixture({ claim });
    await expect(candidate.api.claimNext()).resolves.toMatchObject({
      id: runId,
      mode: 'primary',
    });
    const empty = fixture({ claim: null });
    await expect(empty.api.claimNext()).resolves.toBeNull();

    for (const request of [candidate, empty]) {
      const [url, init] = request.fetch.mock.calls[0];
      expect(url).toContain('/agent/runs/claim-next');
      expect(init?.body).toBeUndefined();
      expect(new Headers(init?.headers).get('authorization')).toBe(
        'Bearer device-bound-access-token',
      );
    }
  });
});
