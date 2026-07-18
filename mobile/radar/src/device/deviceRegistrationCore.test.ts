import { describe, expect, it } from 'vitest';

import { buildDeviceRegistration } from './deviceRegistrationCore';

describe('device registration metadata', () => {
  it('reports runtime facts without claiming unfinished sync or push capabilities', () => {
    const registration = buildDeviceRegistration(
      '01234567-89ab-cdef-0123-456789abcdef',
      {
        platform: 'android',
        appVariant: 'production',
        appVersion: '1.2.3',
        appBuild: '42',
        osVersion: '36',
        locale: 'en-US',
        pushEnvironment: 'production',
        timezone: 'Asia/Shanghai',
      },
    );

    expect(registration).toEqual({
      installationId: '01234567-89ab-cdef-0123-456789abcdef',
      platform: 'android',
      displayName: 'Android device',
      appVariant: 'production',
      appVersion: '1.2.3',
      appBuild: '42',
      osVersion: '36',
      locale: 'en-US',
      timezone: 'Asia/Shanghai',
      capabilities: {
        'client.reactNative': true,
        'agent.runtime': 'pi-0.80.6',
        'agent.schema': 1,
        'agent.streaming': true,
        'agent.submitAnalysis': true,
        'agent.interactive': true,
        'agent.interactiveSchema': 4,
        'push.environment': 'production',
        'sqlite.schema': 7,
      },
    });
    expect(registration.capabilities).not.toHaveProperty('syncAvailable');
    expect(registration.capabilities).not.toHaveProperty('pushAvailable');
  });

  it('does not claim a push environment when the native entitlement is unavailable', () => {
    const registration = buildDeviceRegistration(
      '11234567-89ab-cdef-0123-456789abcdef',
      {
        platform: 'ios',
        appVariant: 'development',
        appVersion: '1.2.3',
        appBuild: '42',
        osVersion: '26.0',
        locale: 'zh-CN',
        pushEnvironment: null,
        timezone: 'Asia/Shanghai',
      },
    );

    expect(registration.capabilities).not.toHaveProperty('push.environment');
  });
});
