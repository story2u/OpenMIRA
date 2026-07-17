import { RadarApiError } from '@story2u/radar-api/client';

import { readAuthenticatedUser } from '../api/client';
import { currentTokenStore, migrateInstalledToken } from './migrateInstalledToken';
import { clearSessionTokens } from './sessionCore';
import {
  InvalidStoredDeviceCredentialError,
  restoreDeviceAwareSession,
} from '../device/deviceSessionCore';
import {
  clearInstalledDeviceSession,
  rotateInstalledDeviceSession,
} from '../device/deviceSession';
import { deviceRefreshTokenStore } from '../device/deviceSessionStorage';
import { legacyTokenStore } from './legacyToken';

export function restoreInstalledSession(baseUrl: string) {
  return restoreDeviceAwareSession({
    clearCredentials: async () => {
      await clearSessionTokens(currentTokenStore, legacyTokenStore);
      await clearInstalledDeviceSession();
    },
    isUnauthorized: (error) => (
      (error instanceof RadarApiError && error.status === 401)
      || error instanceof InvalidStoredDeviceCredentialError
    ),
    loadUser: (token) => readAuthenticatedUser(baseUrl, token),
    migrateAccessToken: migrateInstalledToken,
    readAccessToken: currentTokenStore.read,
    readRefreshToken: deviceRefreshTokenStore.read,
    rotateSession: (refreshToken) => rotateInstalledDeviceSession(baseUrl, refreshToken),
  });
}
