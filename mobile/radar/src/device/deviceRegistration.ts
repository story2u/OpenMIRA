import * as Application from 'expo-application';
import Constants from 'expo-constants';
import { getCalendars, getLocales } from 'expo-localization';
import { Platform } from 'react-native';

import { buildDeviceRegistration, type DeviceRuntimeMetadata } from './deviceRegistrationCore';
import { getOrCreateInstallationId } from './deviceSessionStorage';

async function runtimeMetadata(): Promise<DeviceRuntimeMetadata> {
  if (Platform.OS !== 'ios' && Platform.OS !== 'android') {
    throw new Error('Device registration requires iOS or Android.');
  }
  const config = Constants.expoConfig;
  const applicationId = Platform.OS === 'ios'
    ? config?.ios?.bundleIdentifier
    : config?.android?.package;
  const appBuild = Platform.OS === 'ios'
    ? config?.ios?.buildNumber
    : config?.android?.versionCode?.toString();
  if (!config?.version || !appBuild || !applicationId) {
    throw new Error('Device registration build metadata is unavailable.');
  }
  const pushEnvironment = Platform.OS === 'ios'
    ? await Application.getIosPushNotificationServiceEnvironmentAsync()
    : applicationId.endsWith('.dev') ? 'sandbox' : 'production';
  return {
    platform: Platform.OS,
    appVariant: applicationId.endsWith('.dev') ? 'development' : 'production',
    appVersion: config.version,
    appBuild,
    osVersion: String(Platform.Version),
    locale: getLocales()[0]?.languageTag ?? null,
    pushEnvironment: pushEnvironment === 'development' ? 'sandbox' : pushEnvironment,
    timezone: getCalendars()[0]?.timeZone ?? null,
  };
}

export async function createDeviceRegistration() {
  const [runtime, installationId] = await Promise.all([
    runtimeMetadata(),
    getOrCreateInstallationId(),
  ]);
  return buildDeviceRegistration(installationId, runtime);
}
