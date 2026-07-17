import type { DeviceRegistrationRequest } from '@story2u/radar-contracts/devices';

export interface DeviceRuntimeMetadata {
  appBuild: string;
  appVariant: 'development' | 'production';
  appVersion: string;
  locale: string | null;
  osVersion: string;
  platform: 'android' | 'ios';
  pushEnvironment: 'production' | 'sandbox' | null;
  timezone: string | null;
}

export function buildDeviceRegistration(
  installationId: string,
  runtime: DeviceRuntimeMetadata,
): DeviceRegistrationRequest {
  return {
    installationId,
    platform: runtime.platform,
    displayName: runtime.platform === 'ios' ? 'iOS device' : 'Android device',
    appVariant: runtime.appVariant,
    appVersion: runtime.appVersion,
    appBuild: runtime.appBuild,
    osVersion: runtime.osVersion,
    locale: runtime.locale,
    timezone: runtime.timezone,
    capabilities: {
      'client.reactNative': true,
      'sqlite.schema': 5,
      'agent.streaming': true,
      'agent.runtime': 'pi-0.80.6',
      'agent.schema': 1,
      'agent.submitAnalysis': true,
      'agent.interactive': true,
      'agent.interactiveSchema': 3,
      ...(runtime.pushEnvironment === null
        ? {}
        : { 'push.environment': runtime.pushEnvironment }),
    },
  };
}
