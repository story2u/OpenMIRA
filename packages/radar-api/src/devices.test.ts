import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import { createDevicesApi } from './devices';

const deviceId = '01234567-89ab-cdef-0123-456789abcdef';
const user = {
  id: '11234567-89ab-cdef-0123-456789abcdef',
  email: 'person@example.test',
  displayName: 'Person',
  avatarUrl: '',
  isAdmin: false,
};
const device = {
  id: deviceId,
  platform: 'ios',
  status: 'active',
  displayName: 'Test iPhone',
  appVariant: 'production',
  appVersion: '1.0.0',
  appBuild: '1',
  osVersion: '26.0',
  locale: 'en-US',
  timezone: 'Asia/Shanghai',
  capabilities: { 'sync.schema': 1 },
  lastSeenAt: '2026-07-17T10:00:00Z',
  revokedAt: null,
  createdAt: '2026-07-17T10:00:00Z',
  updatedAt: '2026-07-17T10:00:00Z',
};
const session = {
  accessToken: 'access-token-long-enough',
  tokenType: 'bearer',
  deviceRefreshToken: `radar_device_1_${'a'.repeat(43)}`,
  deviceRefreshTokenExpiresAt: '2026-08-16T10:00:00Z',
  device,
  user,
};
const clientCapabilities = {
  agentToolsAvailable: false,
  deviceAgentAvailable: false,
  e2eeAvailable: false,
  hostedFallbackAvailable: false,
  pushAvailable: false,
  rnClientSupported: true,
  syncAvailable: true,
  signalAppetiteSyncAvailable: true,
};
const pushRegistration = {
  id: '31234567-89ab-cdef-0123-456789abcdef',
  provider: 'apns',
  environment: 'production',
  status: 'active',
  tokenFingerprint: '0123456789ab',
  lastRegisteredAt: '2026-07-17T10:00:00Z',
  lastSuccessAt: null,
  lastNotifiedCursor: 0,
};

function clientWithResponse(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createDevicesApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'current-access-token',
    })),
    fetch,
  };
}

describe('devices API', () => {
  it('validates and sends bounded registration metadata', async () => {
    const fixture = clientWithResponse(session);
    const installationId = '21234567-89ab-cdef-0123-456789abcdef';

    await fixture.api.register({
      installationId,
      platform: 'ios',
      displayName: 'Test iPhone',
      appVariant: 'production',
      appVersion: '1.0.0',
      appBuild: '1',
      locale: 'en-US',
      timezone: 'Asia/Shanghai',
      capabilities: { 'sync.schema': 1 },
    });

    const [url, init] = fixture.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/devices/register');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(String(init?.body))).toEqual({
      installationId,
      platform: 'ios',
      displayName: 'Test iPhone',
      appVariant: 'production',
      appVersion: '1.0.0',
      appBuild: '1',
      locale: 'en-US',
      timezone: 'Asia/Shanghai',
      capabilities: { 'sync.schema': 1 },
    });
  });

  it('uses the device refresh token only as an explicit Authorization header', async () => {
    const fixture = clientWithResponse(session);
    const refreshToken = `radar_device_1_${'b'.repeat(43)}`;

    await fixture.api.rotateCredential(refreshToken);

    const [url, init] = fixture.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/devices/credentials/rotate');
    expect(init?.method).toBe('POST');
    expect(new Headers(init?.headers).get('Authorization')).toBe(`Bearer ${refreshToken}`);
    expect(init?.body).toBeUndefined();
  });

  it('rejects malformed credentials and IDs before issuing a request', () => {
    const fixture = clientWithResponse(session);

    expect(() => fixture.api.rotateCredential('radar_device_1_short')).toThrow(
      'Invalid device refresh token',
    );
    expect(() => fixture.api.rotateCredential(`radar_device_1_${'x'.repeat(43)}\r\nX-Test: unsafe`))
      .toThrow('Invalid device refresh token');
    expect(() => fixture.api.revoke('not-a-uuid')).toThrow('Invalid device ID');
    expect(fixture.fetch).not.toHaveBeenCalled();
  });

  it('strictly rejects leaked fields, malformed credentials and unbounded lists', async () => {
    await expect(clientWithResponse({ ...session, internalHash: 'secret' }).api.rotateCredential(
      session.deviceRefreshToken,
    )).rejects.toThrow();
    await expect(clientWithResponse({
      ...session,
      deviceRefreshToken: 'malformed',
    }).api.rotateCredential(session.deviceRefreshToken)).rejects.toThrow();
    await expect(clientWithResponse(Array.from({ length: 101 }, () => device)).api.list())
      .rejects.toThrow();
    await expect(clientWithResponse([{ ...device, installationIdHash: 'secret' }]).api.list())
      .rejects.toThrow();
  });

  it('lists and revokes only strict device resources', async () => {
    const listed = clientWithResponse([device]);
    await expect(listed.api.list()).resolves.toEqual([device]);

    const revoked = clientWithResponse({ ...device, status: 'revoked' });
    await expect(revoked.api.revoke(deviceId)).resolves.toMatchObject({
      id: deviceId,
      status: 'revoked',
    });
    expect(revoked.fetch.mock.calls[0][0]).toBe(
      `https://api.example.test/api/v1/devices/${deviceId}/revoke`,
    );
  });

  it('strictly decodes server-controlled client capabilities', async () => {
    const supported = clientWithResponse(clientCapabilities);
    await expect(supported.api.capabilities()).resolves.toEqual(clientCapabilities);
    expect(supported.fetch.mock.calls[0][0]).toBe(
      'https://api.example.test/api/v1/devices/current/capabilities',
    );

    await expect(clientWithResponse({
      ...clientCapabilities,
      syncAvailable: 'yes',
    }).api.capabilities()).rejects.toThrow();
    await expect(clientWithResponse({
      ...clientCapabilities,
      rolloutSecret: true,
    }).api.capabilities()).rejects.toThrow();
  });

  it('registers native push tokens without accepting leaked response fields', async () => {
    const fixture = clientWithResponse(pushRegistration);
    const nativeToken = 'native-push-token-long-enough';

    await expect(fixture.api.registerPushToken({
      provider: 'apns',
      environment: 'production',
      token: nativeToken,
    })).resolves.toEqual(pushRegistration);

    const [url, init] = fixture.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/devices/current/push-registration');
    expect(init?.method).toBe('PUT');
    expect(JSON.parse(String(init?.body))).toEqual({
      provider: 'apns',
      environment: 'production',
      token: nativeToken,
    });
    await expect(clientWithResponse({
      ...pushRegistration,
      token: nativeToken,
    }).api.registerPushToken({
      provider: 'apns',
      environment: 'production',
      token: nativeToken,
    })).rejects.toThrow();
  });

  it('bounds push registration input and encodes revoke path from enums only', async () => {
    const fixture = clientWithResponse(null);
    await fixture.api.revokePushToken('fcm', 'sandbox');
    expect(fixture.fetch.mock.calls[0][0]).toBe(
      'https://api.example.test/api/v1/devices/current/push-registration/fcm/sandbox',
    );
    expect(() => fixture.api.registerPushToken({
      provider: 'apns',
      environment: 'production',
      token: 'short',
    })).toThrow();
  });
});
