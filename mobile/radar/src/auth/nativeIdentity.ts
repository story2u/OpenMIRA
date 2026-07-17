import * as AppleAuthentication from 'expo-apple-authentication';
import { randomUUID } from 'expo-crypto';
import { Platform } from 'react-native';

import RadarGoogleAuth from '../../modules/radar-google-auth';
import {
  completeAppleIdentity,
  decodeGoogleIdentity,
  googleNativeConfiguration,
  NativeIdentityFailure,
  type NativeIdentityProvider,
  type NativeIdentityResult,
} from './nativeIdentityCore';

const googleConfiguration = googleNativeConfiguration(
  Platform.OS,
  process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID,
  process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
  RadarGoogleAuth !== null,
);

function isAppleCancellation(error: unknown) {
  return Boolean(
    error
    && typeof error === 'object'
    && 'code' in error
    && error.code === 'ERR_REQUEST_CANCELED',
  );
}

export function isGoogleNativeLoginAvailable() {
  return googleConfiguration.enabled;
}

export async function isAppleNativeLoginAvailable() {
  return Platform.OS === 'ios' && AppleAuthentication.isAvailableAsync();
}

async function requestAppleIdentity(): Promise<NativeIdentityResult> {
  if (!(await isAppleNativeLoginAvailable())) {
    throw new NativeIdentityFailure('provider-unavailable');
  }
  const state = randomUUID();
  try {
    const credential = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
      state,
    });
    return completeAppleIdentity(state, credential.state, credential.identityToken);
  } catch (error) {
    if (isAppleCancellation(error)) return { type: 'cancelled' };
    if (error instanceof NativeIdentityFailure) throw error;
    throw new NativeIdentityFailure('provider-unavailable');
  }
}

async function requestGoogleIdentity(): Promise<NativeIdentityResult> {
  if (!googleConfiguration.enabled || !RadarGoogleAuth) {
    throw new NativeIdentityFailure(
      googleConfiguration.enabled ? 'native-module-unavailable' : googleConfiguration.reason,
    );
  }
  try {
    return decodeGoogleIdentity(await RadarGoogleAuth.signInAsync(
      googleConfiguration.serverClientId,
      googleConfiguration.iosClientId,
    ));
  } catch (error) {
    if (error instanceof NativeIdentityFailure) throw error;
    throw new NativeIdentityFailure('provider-unavailable');
  }
}

export function requestNativeIdentity(provider: NativeIdentityProvider) {
  return provider === 'apple' ? requestAppleIdentity() : requestGoogleIdentity();
}

export type { NativeIdentityProvider, NativeIdentityResult } from './nativeIdentityCore';
