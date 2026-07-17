import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient, idempotencyHeaders, RadarApiError } from './client';

describe('Radar API client', () => {
  it('adds auth and no-store headers before decoding the response', async () => {
    const fetch = vi.fn(async (_input: string, _init?: RequestInit) =>
      Response.json({ id: 'user-1' }));
    const client = createRadarApiClient({
      baseUrl: 'https://api.example.test/',
      fetch,
      getAccessToken: () => 'secret-token',
    });

    await expect(client.request('/api/v1/auth/me', {
      decode: (value) => (value as { id: string }).id,
    })).resolves.toBe('user-1');
    const [url, init] = fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/auth/me');
    expect(init?.cache).toBe('no-store');
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer secret-token');
  });

  it('maps an API detail and request id into a typed error', async () => {
    const client = createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch: async () => Response.json(
        { detail: 'session expired' },
        { status: 401, headers: { 'x-request-id': 'request-1' } },
      ),
      getAccessToken: () => null,
    });

    const error = await client.request('/api/v1/auth/me').catch((caught) => caught);
    expect(error).toBeInstanceOf(RadarApiError);
    expect(error).toMatchObject({ message: 'session expired', status: 401, requestId: 'request-1' });
  });

  it('preserves a reviewed explicit authorization header', async () => {
    const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json({ ok: true }));
    const client = createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'stored-token',
    });

    await client.request('/api/v1/auth/me', {
      headers: { Authorization: 'Bearer callback-token' },
    });

    expect(new Headers(fetch.mock.calls[0][1]?.headers).get('Authorization')).toBe(
      'Bearer callback-token',
    );
  });
});

it('rejects idempotency keys outside the server contract', () => {
  expect(() => idempotencyHeaders('short')).toThrow('8 to 128');
  expect(idempotencyHeaders('request-123')).toEqual({ 'Idempotency-Key': 'request-123' });
});
