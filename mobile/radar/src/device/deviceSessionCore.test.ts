import { describe, expect, it, vi } from 'vitest';

import {
  DeviceCredentialStorageError,
  InvalidStoredDeviceCredentialError,
  persistDeviceSessionCredentials,
  restoreDeviceAwareSession,
  type SecureValueStore,
} from './deviceSessionCore';

const refreshToken = `radar_device_1_${'a'.repeat(43)}`;
const deviceId = '01234567-89ab-cdef-0123-456789abcdef';

function memoryStore(initial: string | null = null): SecureValueStore & {
  failWrite: boolean;
  value: string | null;
} {
  return {
    failWrite: false,
    value: initial,
    async clear() { this.value = null; },
    async read() { return this.value; },
    async write(value) {
      if (this.failWrite) throw new Error('secure storage unavailable');
      this.value = value;
    },
  };
}

describe('device credential storage transaction', () => {
  it('verifies device and refresh values before replacing the access token', async () => {
    const access = memoryStore('previous-access-token');
    const refresh = memoryStore();
    const storedDeviceId = memoryStore();

    await persistDeviceSessionCredentials(
      { access, deviceId: storedDeviceId, refresh },
      {
        accessToken: 'replacement-access-token',
        deviceId,
        refreshToken,
      },
    );

    expect(access.value).toBe('replacement-access-token');
    expect(refresh.value).toBe(refreshToken);
    expect(storedDeviceId.value).toBe(deviceId);
  });

  it('restores every previous value when any secure write fails', async () => {
    const access = memoryStore('previous-access-token');
    const refresh = memoryStore(`radar_device_1_${'p'.repeat(43)}`);
    const storedDeviceId = memoryStore('11234567-89ab-cdef-0123-456789abcdef');
    storedDeviceId.failWrite = true;

    await expect(persistDeviceSessionCredentials(
      { access, deviceId: storedDeviceId, refresh },
      {
        accessToken: 'replacement-access-token',
        deviceId,
        refreshToken,
      },
    )).rejects.toBeInstanceOf(DeviceCredentialStorageError);

    expect(access.value).toBe('previous-access-token');
    expect(refresh.value).toBe(`radar_device_1_${'p'.repeat(43)}`);
    expect(storedDeviceId.value).toBe('11234567-89ab-cdef-0123-456789abcdef');
  });

  it('rejects malformed values without writing token material', async () => {
    const stores = {
      access: memoryStore(),
      deviceId: memoryStore(),
      refresh: memoryStore(),
    };

    await expect(persistDeviceSessionCredentials(stores, {
      accessToken: 'valid-access-token\r\nInjected: value',
      deviceId,
      refreshToken,
    })).rejects.toBeInstanceOf(DeviceCredentialStorageError);
    expect(stores.access.value).toBeNull();
    expect(stores.refresh.value).toBeNull();
  });
});

describe('device-aware session restoration', () => {
  function dependencies(overrides: Record<string, unknown> = {}) {
    return {
      clearCredentials: vi.fn(async () => undefined),
      isUnauthorized: (error: unknown) => (
        error instanceof InvalidStoredDeviceCredentialError
        || (error instanceof Error && error.message === 'unauthorized')
      ),
      loadUser: vi.fn(async () => ({ id: 'user-from-access' })),
      migrateAccessToken: vi.fn(async () => undefined),
      readAccessToken: vi.fn(async () => 'current-access-token'),
      readRefreshToken: vi.fn(async () => refreshToken),
      rotateSession: vi.fn(async () => ({ id: 'user-from-refresh' })),
      ...overrides,
    };
  }

  it('does not rotate while the current access token is accepted', async () => {
    const deps = dependencies();

    await expect(restoreDeviceAwareSession(deps)).resolves.toEqual({
      status: 'authenticated',
      user: { id: 'user-from-access' },
    });
    expect(deps.rotateSession).not.toHaveBeenCalled();
  });

  it('rotates once after an access 401 and authenticates only after persistence succeeds', async () => {
    const deps = dependencies({
      loadUser: vi.fn(async () => Promise.reject(new Error('unauthorized'))),
    });

    await expect(restoreDeviceAwareSession(deps)).resolves.toEqual({
      status: 'authenticated',
      user: { id: 'user-from-refresh' },
    });
    expect(deps.rotateSession).toHaveBeenCalledWith(refreshToken);
    expect(deps.clearCredentials).not.toHaveBeenCalled();
  });

  it('can recover a missing access token from the device credential', async () => {
    const deps = dependencies({ readAccessToken: vi.fn(async () => null) });

    await expect(restoreDeviceAwareSession(deps)).resolves.toMatchObject({
      status: 'authenticated',
    });
    expect(deps.loadUser).not.toHaveBeenCalled();
    expect(deps.rotateSession).toHaveBeenCalledOnce();
  });

  it('clears both credential classes when refresh is rejected or malformed', async () => {
    const rejected = dependencies({
      loadUser: vi.fn(async () => Promise.reject(new Error('unauthorized'))),
      rotateSession: vi.fn(async () => Promise.reject(new Error('unauthorized'))),
    });
    await expect(restoreDeviceAwareSession(rejected)).resolves.toEqual({
      status: 'requires-login',
      reason: 'expired',
    });
    expect(rejected.clearCredentials).toHaveBeenCalledOnce();

    const malformed = dependencies({
      readAccessToken: vi.fn(async () => null),
      readRefreshToken: vi.fn(async () => 'malformed'),
    });
    await expect(restoreDeviceAwareSession(malformed)).resolves.toEqual({
      status: 'requires-login',
      reason: 'expired',
    });
    expect(malformed.clearCredentials).toHaveBeenCalledOnce();
  });

  it('retains credentials when the provider is unavailable', async () => {
    const deps = dependencies({
      loadUser: vi.fn(async () => Promise.reject(new Error('unauthorized'))),
      rotateSession: vi.fn(async () => Promise.reject(new Error('network unavailable'))),
    });

    await expect(restoreDeviceAwareSession(deps)).rejects.toThrow('network unavailable');
    expect(deps.clearCredentials).not.toHaveBeenCalled();
  });
});
