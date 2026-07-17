import { describe, expect, it, vi } from 'vitest';

import { createAuthApi } from './auth';
import { createRadarApiClient } from './client';

const user = {
  id: '01234567-89ab-cdef-0123-456789abcdef',
  email: 'person@example.test',
  displayName: 'Person',
  avatarUrl: '',
  isAdmin: false,
};

function clientWithResponse(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createAuthApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => null,
    })),
    fetch,
  };
}

describe('auth API', () => {
  it('normalizes password login input and strictly decodes the token response', async () => {
    const fixture = clientWithResponse({
      accessToken: 'valid-access-token',
      tokenType: 'bearer',
      user,
    });

    await expect(fixture.api.loginWithPassword({
      email: '  PERSON@EXAMPLE.TEST ',
      password: 'private-password',
    })).resolves.toMatchObject({ user });

    const [url, init] = fixture.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/auth/password/login');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(String(init?.body))).toEqual({
      email: 'person@example.test',
      password: 'private-password',
    });
  });

  it('uses an explicit callback token without first persisting it', async () => {
    const fixture = clientWithResponse(user);

    await fixture.api.getCurrentUser('callback-access-token');

    const headers = new Headers(fixture.fetch.mock.calls[0][1]?.headers);
    expect(headers.get('Authorization')).toBe('Bearer callback-access-token');
  });

  it('exchanges only a bounded native identity token at the provider endpoint', async () => {
    const fixture = clientWithResponse({
      accessToken: 'valid-access-token',
      tokenType: 'bearer',
      user,
    });

    await fixture.api.loginWithNativeToken('google', { idToken: 'header.payload.signature' });

    const [url, init] = fixture.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/auth/oauth/google/native');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(String(init?.body))).toEqual({ idToken: 'header.payload.signature' });
  });

  it('rejects malformed native identity tokens before issuing a request', () => {
    const fixture = clientWithResponse({});

    expect(() => fixture.api.loginWithNativeToken('apple', { idToken: ' short ' }))
      .toThrow('Invalid native identity token');
    expect(fixture.fetch).not.toHaveBeenCalled();
  });

  it('rejects malformed or expanded auth responses at the network boundary', async () => {
    const malformed = clientWithResponse({ ...user, isAdmin: 'false' });
    await expect(malformed.api.getCurrentUser()).rejects.toThrow();

    const expanded = clientWithResponse({ ...user, accessToken: 'must-not-leak' });
    await expect(expanded.api.getCurrentUser()).rejects.toThrow();
  });

  it('rejects a header-injection token before issuing a request', async () => {
    const fixture = clientWithResponse(user);
    expect(() => fixture.api.getCurrentUser('valid-token\r\nX-Test: unsafe')).toThrow('Invalid access token');
    expect(fixture.fetch).not.toHaveBeenCalled();
  });
});
