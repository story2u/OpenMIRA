import { describe, expect, it } from 'vitest';

import appVariant from './app-variant.js';

const { resolveAppVariant } = appVariant;

describe('resolveAppVariant', () => {
  it('defaults to the isolated development identity', () => {
    expect(resolveAppVariant({})).toEqual({
      variant: 'development',
      isProduction: false,
      name: '商机雷达 P0',
      slug: 'opportunity-radar-p0',
      scheme: 'opportunity-radar-dev',
      applicationId: 'com.codeiy.im.dev',
    });
  });

  it('requires explicit store versions before using the production identity', () => {
    expect(() => resolveAppVariant({ RADAR_APP_VARIANT: 'production' })).toThrow(
      'RADAR_APP_VERSION',
    );
    expect(() =>
      resolveAppVariant({
        RADAR_APP_VARIANT: 'production',
        RADAR_APP_VERSION: '1.0.0',
      }),
    ).toThrow('RADAR_BUILD_NUMBER');
    expect(() =>
      resolveAppVariant({
        RADAR_APP_VARIANT: 'production',
        RADAR_APP_VERSION: '1.0.0',
        RADAR_BUILD_NUMBER: '2',
      }),
    ).toThrow('EXPO_PUBLIC_API_BASE_URL');
  });

  it('resolves the existing store identity and deep-link scheme', () => {
    expect(
      resolveAppVariant({
        RADAR_APP_VARIANT: 'production',
        RADAR_APP_VERSION: '1.0.0',
        RADAR_BUILD_NUMBER: '2',
        EXPO_PUBLIC_API_BASE_URL: 'https://api.example.com/',
      }),
    ).toEqual({
      variant: 'production',
      isProduction: true,
      name: '商机雷达',
      slug: 'opportunity-radar',
      scheme: 'opportunity-radar',
      applicationId: 'com.codeiy.im',
      version: '1.0.0',
      buildNumber: '2',
      androidVersionCode: 2,
      apiBaseUrl: 'https://api.example.com',
    });
  });

  it('rejects unknown variants and invalid store version codes', () => {
    expect(() => resolveAppVariant({ RADAR_APP_VARIANT: 'preview' })).toThrow(
      'RADAR_APP_VARIANT',
    );
    expect(() =>
      resolveAppVariant({
        RADAR_APP_VARIANT: 'production',
        RADAR_APP_VERSION: '1.0.0',
        RADAR_BUILD_NUMBER: '2100000001',
        EXPO_PUBLIC_API_BASE_URL: 'https://api.example.com',
      }),
    ).toThrow('versionCode limit');
    expect(() =>
      resolveAppVariant({
        RADAR_APP_VARIANT: 'production',
        RADAR_APP_VERSION: '1.0.0',
        RADAR_BUILD_NUMBER: '2',
        EXPO_PUBLIC_API_BASE_URL: 'http://api.example.com/api/v1',
      }),
    ).toThrow('HTTPS origin');
  });
});
