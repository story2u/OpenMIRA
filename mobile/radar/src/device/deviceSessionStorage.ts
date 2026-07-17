import { randomUUID } from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';

const DEVICE_KEYCHAIN_SERVICE = 'com.codeiy.im.radar.device';
const secureOptions: SecureStore.SecureStoreOptions = {
  keychainService: DEVICE_KEYCHAIN_SERVICE,
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY,
};

function secureStore(key: string) {
  return {
    read: () => SecureStore.getItemAsync(key, secureOptions),
    write: (value: string) => SecureStore.setItemAsync(key, value, secureOptions),
    clear: () => SecureStore.deleteItemAsync(key, secureOptions),
  };
}

export const deviceRefreshTokenStore = secureStore('radar-device-refresh-token');
export const currentDeviceIdStore = secureStore('radar-device-id');
export const pushOptInStore = secureStore('radar-push-opt-in');
export const pushOwnerIdStore = secureStore('radar-push-owner-id');
const installationIdStore = secureStore('radar-device-installation-id');

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

export async function getOrCreateInstallationId() {
  const current = await installationIdStore.read();
  if (current && uuidPattern.test(current)) return current;
  if (current) await installationIdStore.clear();

  const created = randomUUID();
  await installationIdStore.write(created);
  if ((await installationIdStore.read()) !== created) {
    await installationIdStore.clear();
    throw new Error('Device installation ID could not be stored securely.');
  }
  return created;
}
