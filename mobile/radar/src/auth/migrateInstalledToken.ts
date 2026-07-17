import * as SecureStore from 'expo-secure-store';

import { legacyTokenStore } from './legacyToken';
import { migrateLegacyToken } from './tokenMigrationCore';

const CURRENT_KEY = 'radar-access-token';
const CURRENT_KEYCHAIN_SERVICE = 'com.codeiy.im.radar';
const currentOptions: SecureStore.SecureStoreOptions = {
  keychainService: CURRENT_KEYCHAIN_SERVICE,
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY,
};

export const currentTokenStore = {
  read: () => SecureStore.getItemAsync(CURRENT_KEY, currentOptions),
  write: (token: string) => SecureStore.setItemAsync(CURRENT_KEY, token, currentOptions),
  clear: () => SecureStore.deleteItemAsync(CURRENT_KEY, currentOptions),
};

export function migrateInstalledToken() {
  return migrateLegacyToken({ current: currentTokenStore, legacy: legacyTokenStore });
}
