import Constants from 'expo-constants';
import * as Application from 'expo-application';
import * as Notifications from 'expo-notifications';
import * as TaskManager from 'expo-task-manager';
import { Platform } from 'react-native';

import { registerPushToken, revokePushToken } from '../api/client';
import { getMobileApiBaseUrl } from '../config/mobileApiConfig';
import {
  pushOptInStore,
  pushOwnerIdStore,
} from '../device/deviceSessionStorage';
import { logEvent } from '../logging/redactedLogger';
import { synchronizeInstalledOwnerForCursorHint } from '../sync/installedSync';
import {
  buildPushRegistration,
  extractCursorHint,
  type PushEnrollmentState,
} from './pushCore';

const BACKGROUND_PUSH_TASK = 'radar-sync-cursor-hint-v1';
const ANDROID_CHANNEL_ID = 'radar-sync';

function nativePlatform(): 'ios' | 'android' {
  if (Platform.OS !== 'ios' && Platform.OS !== 'android') {
    throw new Error('Push notifications require iOS or Android.');
  }
  return Platform.OS;
}

function isDevelopmentVariant() {
  const config = Constants.expoConfig;
  const applicationId = Platform.OS === 'ios'
    ? config?.ios?.bundleIdentifier
    : config?.android?.package;
  return applicationId?.endsWith('.dev') === true;
}

async function providerAndEnvironment() {
  const platform = nativePlatform();
  const apnsEnvironment = platform === 'ios'
    ? await Application.getIosPushNotificationServiceEnvironmentAsync()
    : null;
  if (platform === 'ios' && apnsEnvironment === null) {
    throw new Error('APNs environment is unavailable on this device.');
  }
  return {
    provider: platform === 'ios' ? 'apns' as const : 'fcm' as const,
    environment: (
      apnsEnvironment === 'development'
      || (platform === 'android' && isDevelopmentVariant())
    ) ? 'sandbox' as const : 'production' as const,
  };
}

async function prepareAndroidChannel() {
  if (Platform.OS !== 'android') return;
  await Notifications.setNotificationChannelAsync(ANDROID_CHANNEL_ID, {
    name: 'Radar sync',
    importance: Notifications.AndroidImportance.DEFAULT,
    vibrationPattern: [0, 200],
  });
}

async function permissionGranted(requestPermission: boolean) {
  await prepareAndroidChannel();
  const current = await Notifications.getPermissionsAsync();
  if (current.granted) return true;
  if (!requestPermission) return false;
  const requested = await Notifications.requestPermissionsAsync({
    ios: { allowAlert: true, allowBadge: true, allowSound: true },
  });
  return requested.granted;
}

async function registerCurrentToken(
  baseUrl: string,
  ownerId: string,
  signal?: AbortSignal,
) {
  const platform = nativePlatform();
  const token = await Notifications.getDevicePushTokenAsync();
  const target = await providerAndEnvironment();
  const request = buildPushRegistration(
    platform,
    target.environment === 'sandbox',
    token.type,
    token.data,
  );
  await registerPushToken(baseUrl, request, signal);
  await pushOwnerIdStore.write(ownerId);
}

export async function enableInstalledPush(
  baseUrl: string,
  ownerId: string,
  signal?: AbortSignal,
): Promise<PushEnrollmentState> {
  if (!(await permissionGranted(true))) return 'denied';
  await registerCurrentToken(baseUrl, ownerId, signal);
  await pushOptInStore.write('enabled');
  return 'active';
}

export async function restoreInstalledPush(
  baseUrl: string,
  ownerId: string,
  signal?: AbortSignal,
): Promise<PushEnrollmentState> {
  if ((await pushOptInStore.read()) !== 'enabled') return 'disabled';
  if (!(await permissionGranted(false))) {
    const target = await providerAndEnvironment();
    try {
      await revokePushToken(baseUrl, target.provider, target.environment, signal);
    } catch {
      // A later foreground/device revocation will retry cleanup; never log the native token.
    }
    await pushOwnerIdStore.clear();
    return 'denied';
  }
  await registerCurrentToken(baseUrl, ownerId, signal);
  return 'active';
}

export async function disableInstalledPush(baseUrl: string, signal?: AbortSignal) {
  const target = await providerAndEnvironment();
  await revokePushToken(baseUrl, target.provider, target.environment, signal);
  await Promise.all([pushOptInStore.clear(), pushOwnerIdStore.clear()]);
}

export async function clearInstalledPushOwner() {
  await pushOwnerIdStore.clear();
}

export async function clearInstalledPushPreference() {
  await Promise.all([pushOptInStore.clear(), pushOwnerIdStore.clear()]);
}

export function subscribeToInstalledPush(options: {
  baseUrl: string;
  ownerId: string;
  onCursorHint(cursor: number, interacted: boolean): void;
}) {
  const tokenSubscription = Notifications.addPushTokenListener((token) => {
    void (async () => {
      if ((await pushOptInStore.read()) !== 'enabled') return;
      try {
        const target = await providerAndEnvironment();
        const request = buildPushRegistration(
          nativePlatform(),
          target.environment === 'sandbox',
          token.type,
          token.data,
        );
        await registerPushToken(options.baseUrl, request);
        await pushOwnerIdStore.write(options.ownerId);
      } catch (error) {
        logEvent('push.token_rotation_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
        });
      }
    })();
  });
  const receivedSubscription = Notifications.addNotificationReceivedListener((notification) => {
    const cursor = extractCursorHint(notification.request.content.data);
    if (cursor !== null) options.onCursorHint(cursor, false);
  });
  const responseSubscription = Notifications.addNotificationResponseReceivedListener((response) => {
    const cursor = extractCursorHint(response.notification.request.content.data);
    if (cursor !== null) options.onCursorHint(cursor, true);
  });
  void Notifications.getLastNotificationResponseAsync().then((response) => {
    const cursor = extractCursorHint(response?.notification.request.content.data);
    if (cursor !== null) {
      options.onCursorHint(cursor, true);
      Notifications.clearLastNotificationResponse();
    }
  }).catch((error: unknown) => {
    logEvent('push.initial_response_failed', {
      errorClass: error instanceof Error ? error.name : 'UnknownError',
    });
  });
  return () => {
    tokenSubscription.remove();
    receivedSubscription.remove();
    responseSubscription.remove();
  };
}

if (!TaskManager.isTaskDefined(BACKGROUND_PUSH_TASK)) {
  TaskManager.defineTask<Notifications.NotificationTaskPayload>(
    BACKGROUND_PUSH_TASK,
    async ({ data, error }) => {
      const cursor = extractCursorHint(data);
      if (error || cursor === null) {
        return Notifications.BackgroundNotificationTaskResult.Failed;
      }
      const [enabled, ownerId] = await Promise.all([
        pushOptInStore.read(),
        pushOwnerIdStore.read(),
      ]);
      if (enabled !== 'enabled' || !ownerId) {
        return Notifications.BackgroundNotificationTaskResult.NoData;
      }
      try {
        const outcome = await synchronizeInstalledOwnerForCursorHint(
          getMobileApiBaseUrl(),
          ownerId,
          cursor,
        );
        return outcome.status === 'synchronized'
          ? Notifications.BackgroundNotificationTaskResult.NewData
          : Notifications.BackgroundNotificationTaskResult.NoData;
      } catch (syncError) {
        logEvent('push.background_sync_failed', {
          errorClass: syncError instanceof Error ? syncError.name : 'UnknownError',
        });
        return Notifications.BackgroundNotificationTaskResult.Failed;
      }
    },
  );
}

void Notifications.registerTaskAsync(BACKGROUND_PUSH_TASK).catch((error: unknown) => {
  logEvent('push.background_registration_failed', {
    errorClass: error instanceof Error ? error.name : 'UnknownError',
  });
});
