import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createMessagesApi,
  decodeMessagePageResponse,
  messagePageRequestPath,
} from './messages';

const firstMessage = {
  id: '11111111-1111-4111-8111-111111111111',
  senderName: 'Fixture Contact',
  content: 'Can you share the enterprise plan?',
  isFromContact: true,
  sentAt: '2026-07-17T01:00:00Z',
  source: 'human',
};
const secondMessage = {
  ...firstMessage,
  id: '22222222-2222-4222-8222-222222222222',
  senderName: 'Opportunity Assistant',
  content: 'I will prepare a reviewed proposal.',
  isFromContact: false,
  sentAt: '2026-07-17T01:05:00+00:00',
  source: 'ai',
};
const page = {
  items: [firstMessage, secondMessage],
  total: 2,
  limit: 50,
  offset: 0,
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createMessagesApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'valid-access-token',
    })),
    fetch,
  };
}

describe('messages API', () => {
  it('serializes a bounded page and forwards cancellation', async () => {
    const controller = new AbortController();
    const request = fixture(page);
    await request.api.page({
      opportunity_id: '01234567-89ab-cdef-0123-456789abcdef',
      limit: 50,
      offset: 100,
    }, { signal: controller.signal });

    expect(request.fetch.mock.calls[0][0]).toBe(
      'https://api.example.test/api/v1/messages/page?' +
      'opportunity_id=01234567-89ab-cdef-0123-456789abcdef&limit=50&offset=100',
    );
    expect(request.fetch.mock.calls[0][1]?.signal).toBe(controller.signal);
  });

  it('rejects expanded, duplicated, out-of-order and inconsistent pages', async () => {
    await expect(fixture({ ...page, extra: true }).api.page({ opportunity_id: firstMessage.id }))
      .rejects.toThrow();
    expect(() => decodeMessagePageResponse({ ...page, items: [firstMessage, firstMessage] }))
      .toThrow('duplicate');
    expect(() => decodeMessagePageResponse({ ...page, items: [secondMessage, firstMessage] }))
      .toThrow('chronological');
    expect(() => decodeMessagePageResponse({ ...page, total: 1 })).toThrow('inconsistent');
    expect(() => decodeMessagePageResponse({
      ...page,
      items: Array.from({ length: 201 }, (_, index) => ({
        ...firstMessage,
        id: `${index.toString(16).padStart(8, '0')}-1111-4111-8111-111111111111`,
      })),
      limit: 200,
      total: 201,
    })).toThrow();
  });

  it('rejects invalid IDs and pagination before requesting', () => {
    expect(() => messagePageRequestPath({ opportunity_id: 'invalid' })).toThrow('opportunity id');
    expect(() => messagePageRequestPath({ opportunity_id: firstMessage.id, limit: 201 })).toThrow('limit');
    expect(() => messagePageRequestPath({ opportunity_id: firstMessage.id, offset: -1 })).toThrow('offset');
  });
});
