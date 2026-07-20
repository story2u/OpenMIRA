import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import resolveExpoConfig from '../app.config.js';

const managedEnvironmentKeys = [
  'EXPO_PUBLIC_API_BASE_URL',
  'EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID',
  'EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID',
  'RADAR_APP_VARIANT',
  'RADAR_APP_VERSION',
  'RADAR_BUILD_NUMBER',
  'RADAR_GOOGLE_SERVICES_FILE',
];
const originalEnvironment = new Map();

beforeEach(() => {
  for (const key of managedEnvironmentKeys) {
    originalEnvironment.set(key, process.env[key]);
    delete process.env[key];
  }
});

afterEach(() => {
  for (const key of managedEnvironmentKeys) {
    const original = originalEnvironment.get(key);
    if (original === undefined) delete process.env[key];
    else process.env[key] = original;
  }
  originalEnvironment.clear();
});

function notificationsMode(config) {
  const plugin = config.plugins.find(
    (candidate) => Array.isArray(candidate) && candidate[0] === 'expo-notifications',
  );
  return plugin?.[1]?.mode;
}

const baseConfig = {
  ios: {},
  android: {},
  plugins: [
    ['expo-notifications', { enableBackgroundRemoteNotifications: true }],
    ['expo-build-properties', { android: { usesCleartextTraffic: true } }],
  ],
};

describe('Expo application config', () => {
  it('uses the development APNs entitlement for the isolated build', () => {
    const config = resolveExpoConfig({ config: baseConfig });

    expect(notificationsMode(config)).toBe('development');
    expect(config.ios.entitlements['keychain-access-groups']).toEqual([
      '$(AppIdentifierPrefix)$(CFBundleIdentifier)',
    ]);
  });

  it('uses the production APNs entitlement and controlled FCM file for store builds', () => {
    process.env.RADAR_APP_VARIANT = 'production';
    process.env.RADAR_APP_VERSION = '1.0.0';
    process.env.RADAR_BUILD_NUMBER = '2';
    process.env.EXPO_PUBLIC_API_BASE_URL = 'https://api.example.com';
    process.env.RADAR_GOOGLE_SERVICES_FILE = '/run/secrets/google-services.json';

    const config = resolveExpoConfig({ config: baseConfig });

    expect(config.name).toBe('OpenMIRA：智能消息管家');
    expect(notificationsMode(config)).toBe('production');
    expect(config.android.googleServicesFile).toBe('/run/secrets/google-services.json');
  });
});
