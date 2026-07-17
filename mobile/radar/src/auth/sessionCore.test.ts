import { expect, it, vi } from 'vitest';

import {
  clearSessionTokens,
  endSession,
  persistReplacingLegacyToken,
  persistSessionToken,
  restoreSession,
} from './sessionCore';

function dependencies(overrides: Record<string, unknown> = {}) {
  return {
    clearToken: vi.fn(async () => undefined),
    isUnauthorized: (error: unknown) => error === 'unauthorized',
    loadUser: vi.fn(async () => ({ id: 'user-1' })),
    migrateToken: vi.fn(async () => 'no-legacy-token'),
    readToken: vi.fn(async () => 'token'),
    ...overrides,
  };
}

it('restores an authenticated session after token migration', async () => {
  await expect(restoreSession(dependencies())).resolves.toEqual({
    status: 'authenticated',
    user: { id: 'user-1' },
  });
});

it('clears only an expired token and requires login', async () => {
  const deps = dependencies({ loadUser: vi.fn(async () => Promise.reject('unauthorized')) });
  await expect(restoreSession(deps)).resolves.toEqual({ status: 'requires-login', reason: 'expired' });
  expect(deps.clearToken).toHaveBeenCalledOnce();
});

it('preserves the token when user loading fails offline', async () => {
  const deps = dependencies({ loadUser: vi.fn(async () => Promise.reject(new Error('offline'))) });
  await expect(restoreSession(deps)).rejects.toThrow('offline');
  expect(deps.clearToken).not.toHaveBeenCalled();
});

it('only accepts a newly issued token after SecureStore read-back', async () => {
  let stored: string | null = null;
  const clear = vi.fn(async () => {
    stored = null;
  });
  await persistSessionToken({
    clear,
    read: async () => stored,
    write: async (token) => {
      stored = token;
    },
  }, 'new-access-token-value');
  expect(stored).toBe('new-access-token-value');
  expect(clear).not.toHaveBeenCalled();
});

it('clears an unverifiable token and never exposes it as a session', async () => {
  const clear = vi.fn(async () => undefined);
  await expect(persistSessionToken({
    clear,
    read: async () => 'different-token-value',
    write: async () => undefined,
  }, 'new-access-token-value')).rejects.toThrow('could not be verified');
  expect(clear).toHaveBeenCalledOnce();
});

it('never writes a new token when legacy credentials cannot be retired', async () => {
  const write = vi.fn(async () => undefined);
  const current = {
    clear: vi.fn(async () => undefined),
    read: async () => null,
    write,
  };
  await expect(persistReplacingLegacyToken(
    current,
    { clear: async () => Promise.reject(new Error('keychain unavailable')) },
    'new-access-token-value',
  )).rejects.toThrow('keychain unavailable');
  expect(write).not.toHaveBeenCalled();
  expect(current.clear).not.toHaveBeenCalled();
});

it('retires legacy credentials before persisting a replacement', async () => {
  const order: string[] = [];
  let stored: string | null = null;
  await persistReplacingLegacyToken(
    {
      clear: async () => { order.push('current-clear'); },
      read: async () => {
        order.push('current-read');
        return stored;
      },
      write: async (token) => {
        order.push('current-write');
        stored = token;
      },
    },
    { clear: async () => { order.push('legacy-clear'); } },
    'new-access-token-value',
  );
  expect(order).toEqual(['legacy-clear', 'current-write', 'current-read']);
});

it('clears legacy before current so partial logout cannot resurrect a token', async () => {
  const order: string[] = [];
  await clearSessionTokens(
    { clear: async () => { order.push('current'); } },
    { clear: async () => { order.push('legacy'); } },
  );
  expect(order).toEqual(['legacy', 'current']);
});

it('ends authentication even when local cache cleanup must be retried later', async () => {
  const clearToken = vi.fn(async () => undefined);
  await expect(endSession({
    clearToken,
    clearLocalData: async () => Promise.reject(new Error('database busy')),
  })).resolves.toEqual({ localDataCleared: false });
  expect(clearToken).toHaveBeenCalledOnce();
});
