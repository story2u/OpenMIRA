import * as SecureStore from 'expo-secure-store';

import type { LegacyTokenStore } from './tokenMigrationCore';

const LEGACY_KEY = 'access-token';
const LEGACY_KEYCHAIN_SERVICE = 'com.codeiy.im.jwt';
const options = { keychainService: LEGACY_KEYCHAIN_SERVICE };

export const legacyTokenStore: LegacyTokenStore = {
  read: () => SecureStore.getItemAsync(LEGACY_KEY, options),
  clear: () => SecureStore.deleteItemAsync(LEGACY_KEY, options),
};
