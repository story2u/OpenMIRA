export type NativeIdentityProvider = 'apple' | 'google';

export type NativeIdentityResult =
  | { type: 'cancelled' }
  | { type: 'success'; idToken: string };

export type NativeIdentityFailureCode =
  | 'invalid-client-configuration'
  | 'invalid-identity-response'
  | 'native-module-unavailable'
  | 'provider-unavailable'
  | 'state-mismatch';

export class NativeIdentityFailure extends Error {
  constructor(readonly code: NativeIdentityFailureCode) {
    super(code);
    this.name = 'NativeIdentityFailure';
  }
}

const googleClientIdPattern = /^[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$/;

function normalizeGoogleClientId(value: string | undefined) {
  const normalized = value?.trim();
  if (!normalized) return null;
  if (normalized.length > 512 || !googleClientIdPattern.test(normalized)) {
    throw new NativeIdentityFailure('invalid-client-configuration');
  }
  return normalized;
}

export type GoogleNativeConfiguration =
  | { enabled: false; reason: NativeIdentityFailureCode }
  | { enabled: true; iosClientId: string | null; serverClientId: string };

export function googleNativeConfiguration(
  platform: string,
  serverClientIdValue: string | undefined,
  iosClientIdValue: string | undefined,
  nativeModuleAvailable: boolean,
): GoogleNativeConfiguration {
  if (platform !== 'ios' && platform !== 'android') {
    return { enabled: false, reason: 'provider-unavailable' };
  }
  if (!nativeModuleAvailable) {
    return { enabled: false, reason: 'native-module-unavailable' };
  }

  try {
    const serverClientId = normalizeGoogleClientId(serverClientIdValue);
    const iosClientId = normalizeGoogleClientId(iosClientIdValue);
    if (!serverClientId || (platform === 'ios' && !iosClientId)) {
      return { enabled: false, reason: 'invalid-client-configuration' };
    }
    return {
      enabled: true,
      iosClientId: platform === 'ios' ? iosClientId : null,
      serverClientId,
    };
  } catch (error) {
    if (error instanceof NativeIdentityFailure) {
      return { enabled: false, reason: error.code };
    }
    throw error;
  }
}

export function validateIdentityToken(value: unknown) {
  if (
    typeof value !== 'string'
    || value.length < 16
    || value.length > 8192
    || value.trim() !== value
    || /[\r\n]/.test(value)
  ) {
    throw new NativeIdentityFailure('invalid-identity-response');
  }
  return value;
}

export function completeAppleIdentity(
  expectedState: string,
  returnedState: unknown,
  identityToken: unknown,
): NativeIdentityResult {
  if (returnedState !== expectedState) {
    throw new NativeIdentityFailure('state-mismatch');
  }
  return { type: 'success', idToken: validateIdentityToken(identityToken) };
}

export function decodeGoogleIdentity(value: unknown): NativeIdentityResult {
  if (!value || typeof value !== 'object') {
    throw new NativeIdentityFailure('invalid-identity-response');
  }
  const record = value as Record<string, unknown>;
  if (record.type === 'cancelled' && Object.keys(record).length === 1) {
    return { type: 'cancelled' };
  }
  if (
    record.type === 'success'
    && Object.keys(record).length === 2
    && Object.hasOwn(record, 'idToken')
  ) {
    return { type: 'success', idToken: validateIdentityToken(record.idToken) };
  }
  throw new NativeIdentityFailure('invalid-identity-response');
}
