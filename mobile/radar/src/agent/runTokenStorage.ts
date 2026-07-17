import * as SecureStore from 'expo-secure-store';

const keychainService = 'com.codeiy.im.radar.analysis-run';
const secureOptions: SecureStore.SecureStoreOptions = {
  keychainService,
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY,
};
const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

export interface AnalysisRunTokenStore {
  clear(runId: string): Promise<void>;
  read(runId: string): Promise<string | null>;
  write(runId: string, token: string): Promise<void>;
}

function tokenKey(runId: string) {
  if (!uuidPattern.test(runId)) throw new Error('invalid analysis run ID');
  return `radar-analysis-run-${runId}`;
}

export const analysisRunTokenStore: AnalysisRunTokenStore = {
  clear: (runId) => SecureStore.deleteItemAsync(tokenKey(runId), secureOptions),
  read: (runId) => SecureStore.getItemAsync(tokenKey(runId), secureOptions),
  async write(runId, token) {
    if (token.length < 16 || token.length > 16_384) {
      throw new Error('invalid analysis run token');
    }
    const key = tokenKey(runId);
    await SecureStore.setItemAsync(key, token, secureOptions);
    if ((await SecureStore.getItemAsync(key, secureOptions)) !== token) {
      await SecureStore.deleteItemAsync(key, secureOptions);
      throw new Error('analysis run token could not be stored securely');
    }
  },
};
