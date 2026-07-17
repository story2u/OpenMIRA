import { describe, expect, it } from 'vitest';

import {
  completeAppleIdentity,
  decodeGoogleIdentity,
  googleNativeConfiguration,
  NativeIdentityFailure,
  validateIdentityToken,
} from './nativeIdentityCore';

const token = 'header.payload.signature';
const serverClientId = '123-server.apps.googleusercontent.com';
const iosClientId = '123-ios.apps.googleusercontent.com';

describe('native identity boundaries', () => {
  it('enables only platform-complete Google configuration with a native module', () => {
    expect(googleNativeConfiguration('android', serverClientId, undefined, true)).toEqual({
      enabled: true,
      iosClientId: null,
      serverClientId,
    });
    expect(googleNativeConfiguration('ios', serverClientId, iosClientId, true)).toEqual({
      enabled: true,
      iosClientId,
      serverClientId,
    });
    expect(googleNativeConfiguration('ios', serverClientId, undefined, true)).toEqual({
      enabled: false,
      reason: 'invalid-client-configuration',
    });
    expect(googleNativeConfiguration('android', serverClientId, undefined, false)).toEqual({
      enabled: false,
      reason: 'native-module-unavailable',
    });
  });

  it('rejects malformed client IDs and identity tokens', () => {
    expect(googleNativeConfiguration('android', 'not-a-client-id', undefined, true)).toEqual({
      enabled: false,
      reason: 'invalid-client-configuration',
    });
    expect(() => validateIdentityToken(' short ')).toThrow(NativeIdentityFailure);
    expect(() => validateIdentityToken(`${token}\nunsafe`)).toThrow(NativeIdentityFailure);
  });

  it('requires Apple state correlation before accepting the token', () => {
    expect(completeAppleIdentity('state-1', 'state-1', token)).toEqual({
      type: 'success',
      idToken: token,
    });
    expect(() => completeAppleIdentity('state-1', 'state-2', token)).toThrow('state-mismatch');
  });

  it('decodes only the two closed Google result shapes', () => {
    expect(decodeGoogleIdentity({ type: 'cancelled' })).toEqual({ type: 'cancelled' });
    expect(decodeGoogleIdentity({ type: 'success', idToken: token })).toEqual({
      type: 'success',
      idToken: token,
    });
    expect(() => decodeGoogleIdentity({ type: 'success', idToken: token, profile: {} }))
      .toThrow('invalid-identity-response');
  });
});
