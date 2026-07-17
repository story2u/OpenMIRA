import { RadarApiError } from '@story2u/radar-api/client';

import { NativeIdentityFailure, type NativeIdentityProvider } from './nativeIdentityCore';
import { fallbackTranslator, type Translator } from '../i18n/core';

export function loginErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 401) return t('auth.error.invalidCredentials');
    if (error.status === 429) return t('auth.error.rateLimited');
    if (error.status >= 500) return t('auth.error.serviceUnavailable');
  }
  return t('auth.error.secureStorage');
}

export function nativeLoginErrorMessage(
  provider: NativeIdentityProvider,
  error: unknown,
  t: Translator = fallbackTranslator,
) {
  const providerName = provider === 'apple' ? 'Apple' : 'Google';
  if (error instanceof NativeIdentityFailure) {
    if (error.code === 'invalid-client-configuration') {
      return t('auth.error.providerNotConfigured', { provider: providerName });
    }
    if (error.code === 'native-module-unavailable') {
      return t('auth.error.nativeBuildRequired', { provider: providerName });
    }
    if (error.code === 'state-mismatch' || error.code === 'invalid-identity-response') {
      return t('auth.error.invalidProviderCredential', { provider: providerName });
    }
    return t('auth.error.providerUnavailable', { provider: providerName });
  }
  if (error instanceof RadarApiError) {
    if (error.status === 401 || error.status === 403) {
      return t('auth.error.providerRejected', { provider: providerName });
    }
    if (error.status === 429) return t('auth.error.rateLimited');
    if (error.status === 503) {
      return t('auth.error.providerDisabled', { provider: providerName });
    }
    if (error.status >= 500) return t('auth.error.serviceUnavailable');
  }
  return t('auth.error.secureStorage');
}
