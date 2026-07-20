import { RadarApiError } from '@story2u/radar-api/client';

import { NativeIdentityFailure, type NativeIdentityProvider } from './nativeIdentityCore';
import { SessionPersistenceError } from './sessionCore';
import { fallbackTranslator, type Translator } from '../i18n/core';

function isNetworkFailure(error: unknown) {
  return error instanceof TypeError;
}

function persistenceDiagnostic(error: unknown, t: Translator) {
  if (
    process.env.EXPO_PUBLIC_AUTH_PERSISTENCE_DIAGNOSTIC !== 'true'
    || !(error instanceof SessionPersistenceError)
  ) {
    return null;
  }
  const cause = error.cause instanceof Error ? ` / ${error.cause.name}` : '';
  return `${t('auth.error.secureStorage')}（诊断：${error.stage}${cause}）`;
}

export function loginErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 401) return t('auth.error.invalidCredentials');
    if (error.status === 429) return t('auth.error.rateLimited');
    if (error.status >= 500) return t('auth.error.serviceUnavailable');
  }
  if (isNetworkFailure(error)) return t('auth.error.networkUnavailable');
  const diagnostic = persistenceDiagnostic(error, t);
  if (diagnostic) return diagnostic;
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
  if (isNetworkFailure(error)) return t('auth.error.networkUnavailable');
  const diagnostic = persistenceDiagnostic(error, t);
  if (diagnostic) return diagnostic;
  return t('auth.error.secureStorage');
}
