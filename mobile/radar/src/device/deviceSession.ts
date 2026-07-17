import type { DeviceSession } from '@story2u/radar-contracts/devices';

import {
  registerDevice,
  revokeDevice,
  rotateDeviceCredential,
} from '../api/client';
import { currentTokenStore } from '../auth/migrateInstalledToken';
import {
  persistDeviceSessionCredentials,
  validateDeviceRefreshToken,
  validateStoredDeviceId,
} from './deviceSessionCore';
import { createDeviceRegistration } from './deviceRegistration';
import {
  currentDeviceIdStore,
  deviceRefreshTokenStore,
} from './deviceSessionStorage';

const credentialStores = {
  access: currentTokenStore,
  deviceId: currentDeviceIdStore,
  refresh: deviceRefreshTokenStore,
};

async function persistSession(session: DeviceSession) {
  await persistDeviceSessionCredentials(credentialStores, {
    accessToken: session.accessToken,
    deviceId: session.device.id,
    refreshToken: session.deviceRefreshToken,
  });
}

export async function ensureDeviceRegistration(baseUrl: string, signal?: AbortSignal) {
  const [deviceId, refreshToken] = await Promise.all([
    currentDeviceIdStore.read(),
    deviceRefreshTokenStore.read(),
  ]);
  if (deviceId && refreshToken) {
    try {
      validateStoredDeviceId(deviceId);
      validateDeviceRefreshToken(refreshToken);
      return null;
    } catch {
      await clearInstalledDeviceSession();
    }
  } else if (deviceId || refreshToken) {
    await clearInstalledDeviceSession();
  }

  const registration = await createDeviceRegistration();
  const session = await registerDevice(baseUrl, registration, signal);
  if (signal?.aborted) return null;
  await persistSession(session);
  return session;
}

export async function rotateInstalledDeviceSession(baseUrl: string, refreshToken: string) {
  const session = await rotateDeviceCredential(
    baseUrl,
    validateDeviceRefreshToken(refreshToken),
  );
  await persistSession(session);
  return session.user;
}

export async function revokeInstalledDevice(baseUrl: string, signal?: AbortSignal) {
  const deviceId = await currentDeviceIdStore.read();
  if (!deviceId) return;
  await revokeDevice(baseUrl, deviceId, signal);
}

export async function clearInstalledDeviceSession() {
  const results = await Promise.allSettled([
    currentDeviceIdStore.clear(),
    deviceRefreshTokenStore.clear(),
  ]);
  if (results.some((result) => result.status === 'rejected')) {
    throw new Error('Device session could not be cleared securely.');
  }
}
