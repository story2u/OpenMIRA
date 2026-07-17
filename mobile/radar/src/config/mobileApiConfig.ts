import Constants from 'expo-constants';

import { parseApiBaseUrl } from './apiBaseUrl';

function isParallelDevelopmentBundle() {
  const expoConfig = Constants.expoConfig;
  return (
    expoConfig?.ios?.bundleIdentifier === 'com.codeiy.im.dev' ||
    expoConfig?.android?.package === 'com.codeiy.im.dev'
  );
}

export function getMobileApiBaseUrl() {
  // Release builds with the production bundle id remain HTTPS-only. The parallel .dev app may
  // reach loopback fixtures because its native manifest already carries the explicit P0 exception.
  return parseApiBaseUrl(
    process.env.EXPO_PUBLIC_API_BASE_URL,
    __DEV__ || isParallelDevelopmentBundle(),
  );
}
