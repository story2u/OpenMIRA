import type { AnalysisRunClaim, AnalysisRunLinks } from '@story2u/radar-contracts/analysis-runs';
import { describe, expect, it, vi } from 'vitest';

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));

import { ANALYSIS_SYSTEM_PROMPT } from '@story2u/radar-agent/analysis';

import { runDeviceAnalysis } from './deviceAnalysis';
import { createGatewayStreamFn } from './gatewayStream';

const runId = '12345678-1234-4234-8234-123456789abc';
const messageId = '22345678-1234-4234-8234-123456789abc';
const deviceId = '32345678-1234-4234-8234-123456789abc';
const runToken = 'run-token-with-more-than-sixteen-characters';

const expectedResult = {
  is_opportunity: true,
  confidence: 0.91,
  title: 'Qualified request',
  summary: 'The message asks for a commercial proposal.',
  priority: 'high' as const,
  trust_score: 80,
  attention_required: true,
  link_status: 'safe' as const,
  link_summary: 'The server-fetched page had no deterministic risk flags.',
  risk_flags: [],
  contacts: {
    email: 'buyer@example.test',
    phone: null,
    telegram_handle: null,
    wecom_id: null,
    extraction_source: 'message_text' as const,
  },
  actions: [],
};

function claim(): AnalysisRunClaim {
  return {
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
    sourceMessageVersion: 4,
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
    runToken,
    input: {
      messageId,
      sourceMessageVersion: 4,
      channel: 'wecom',
      senderDisplayName: 'Buyer',
      sourceType: 'private',
      groupName: null,
      text: 'Please send a proposal to buyer@example.test.',
      links: ['https://example.test/request'],
    },
  };
}

function links(): AnalysisRunLinks {
  return {
    runId,
    sourceMessageVersion: 4,
    fetchedAt: '2026-07-17T10:00:05Z',
    evidence: [{
      url: 'https://example.test/request',
      final_url: 'https://example.test/request',
      status: 'safe',
      http_status: 200,
      content_type: 'text/html',
      title: 'Request',
      text: 'Request for proposal',
      emails: ['buyer@example.test'],
      risk_reasons: [],
    }],
  };
}

function gatewayResponse(result = expectedResult) {
  const serialized = JSON.stringify(result);
  const events = [
    JSON.stringify({
      id: 'chatcmpl_gateway',
      model: 'radar-analysis-v1',
      choices: [{
        index: 0,
        delta: {
          role: 'assistant',
          tool_calls: [{
            index: 0,
            id: 'call_submit',
            type: 'function',
            function: { name: 'submit_analysis', arguments: serialized.slice(0, 80) },
          }],
        },
        finish_reason: null,
      }],
    }),
    JSON.stringify({
      id: 'chatcmpl_gateway',
      model: 'radar-analysis-v1',
      choices: [{
        index: 0,
        delta: { tool_calls: [{ index: 0, function: { arguments: serialized.slice(80) } }] },
        finish_reason: null,
      }],
    }),
    JSON.stringify({
      id: 'chatcmpl_gateway',
      model: 'radar-analysis-v1',
      choices: [{ index: 0, delta: {}, finish_reason: 'tool_calls' }],
    }),
    JSON.stringify({
      id: 'chatcmpl_gateway',
      model: 'radar-analysis-v1',
      choices: [],
      usage: { prompt_tokens: 100, completion_tokens: 40, total_tokens: 140 },
    }),
    '[DONE]',
  ];
  const bytes = new TextEncoder().encode(events.map((event) => `data: ${event}\n\n`).join(''));
  return {
    ok: true,
    status: 200,
    headers: new Headers({ 'content-type': 'text/event-stream; charset=utf-8' }),
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes.slice(0, 41));
        controller.enqueue(bytes.slice(41, 133));
        controller.enqueue(bytes.slice(133));
        controller.close();
      },
    }),
  };
}

describe('RN pi gateway stream', () => {
  it('runs the real shared submit_analysis harness without exposing provider identity', async () => {
    let request: { url: string; init: RequestInit } | undefined;
    const fetch = vi.fn(async (url: string, init: RequestInit) => {
      request = { url, init };
      return gatewayResponse();
    });

    await expect(runDeviceAnalysis({
      baseUrl: 'https://api.example.test',
      claim: claim(),
      links: links(),
      fetch,
    })).resolves.toEqual(expectedResult);

    expect(request?.url).toBe(
      'https://api.example.test/api/v1/agent/gateway/v1/chat/completions',
    );
    const headers = new Headers(request?.init.headers);
    expect(headers.get('authorization')).toBe(`Bearer ${runToken}`);
    const payload = JSON.parse(String(request?.init.body));
    expect(payload).toMatchObject({
      model: 'radar-analysis-v1',
      stream: true,
      stream_options: { include_usage: true },
      store: false,
      tool_choice: { type: 'function', function: { name: 'submit_analysis' } },
    });
    expect(payload.messages).toHaveLength(2);
    expect(payload.messages[0]).toEqual({ role: 'system', content: ANALYSIS_SYSTEM_PROMPT });
    expect(payload.tools).toHaveLength(1);
    expect(payload.tools[0].function.name).toBe('submit_analysis');
    expect(JSON.stringify(payload)).not.toContain('provider-model');
    expect(JSON.stringify(payload)).not.toContain('provider-key');
  });

  it('encodes cancellation as a terminal stream event and forwards AbortSignal', async () => {
    let forwardedSignal: AbortSignal | null | undefined;
    const fetch = vi.fn((_url: string, init: RequestInit) => {
      forwardedSignal = init.signal;
      return new Promise<never>((_resolve, reject) => {
        init.signal?.addEventListener('abort', () => reject(new Error('aborted')), { once: true });
      });
    });
    const controller = new AbortController();
    const streamFn = createGatewayStreamFn({
      baseUrl: 'https://api.example.test',
      runToken,
      fetch,
    });
    const stream = streamFn(
      {
        id: 'radar-analysis-v1',
        name: 'Gateway',
        api: 'radar-gateway',
        provider: 'radar-analysis-gateway',
        baseUrl: 'https://api.example.test',
        reasoning: false,
        input: ['text'],
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        contextWindow: 32_000,
        maxTokens: 4_096,
      },
      {
        systemPrompt: ANALYSIS_SYSTEM_PROMPT,
        messages: [{ role: 'user', content: 'Analyze <message-data>{}</message-data>', timestamp: 1 }],
        tools: [{ name: 'submit_analysis', description: 'Submit', parameters: { type: 'object' } }],
      },
      { signal: controller.signal },
    );
    controller.abort();
    const events = [];
    for await (const event of stream) events.push(event);

    expect(forwardedSignal).toBe(controller.signal);
    expect(events.at(-1)).toMatchObject({
      type: 'error',
      reason: 'aborted',
      error: { errorMessage: 'analysis_cancelled', stopReason: 'aborted' },
    });
  });
});
