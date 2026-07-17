import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import { createTemplatesApi, decodeReplyTemplates } from './templates';

const template = {
  id: '11111111-1111-4111-8111-111111111111',
  title: 'Enterprise follow-up',
  content: 'I can prepare a reviewed proposal for your team.',
  category: 'Follow-up',
};

describe('templates API', () => {
  it('loads a strict bounded list with cancellation', async () => {
    const controller = new AbortController();
    const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json([template]));
    const api = createTemplatesApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'valid-access-token',
    }));

    await expect(api.list({ signal: controller.signal })).resolves.toEqual([template]);
    expect(fetch.mock.calls[0][0]).toBe('https://api.example.test/api/v1/templates');
    expect(fetch.mock.calls[0][1]?.signal).toBe(controller.signal);
  });

  it('rejects expanded, duplicate and unbounded lists', () => {
    expect(() => decodeReplyTemplates([{ ...template, extra: true }])).toThrow();
    expect(() => decodeReplyTemplates([template, template])).toThrow('duplicate');
    expect(() => decodeReplyTemplates(
      Array.from({ length: 201 }, (_, index) => ({
        ...template,
        id: `${index.toString(16).padStart(8, '0')}-1111-4111-8111-111111111111`,
      })),
    )).toThrow();
  });
});
