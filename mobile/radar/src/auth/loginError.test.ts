import { RadarApiError } from '@story2u/radar-api/client';
import { describe, expect, it } from 'vitest';

import { loginErrorMessage, nativeLoginErrorMessage } from './loginError';
import { NativeIdentityFailure } from './nativeIdentityCore';
import { createTranslator } from '../i18n/core';

it.each([
  [401, '邮箱或密码错误。'],
  [429, '登录尝试过于频繁，请稍后重试。'],
  [503, '登录服务暂时不可用，请稍后重试。'],
])('maps API status %s to safe user-facing copy', (status, expected) => {
  expect(loginErrorMessage(new RadarApiError('private server detail', status, 'request-id')))
    .toBe(expected);
});

it('maps fetch network failures separately from secure storage failures', () => {
  expect(loginErrorMessage(new TypeError('Network request failed')))
    .toBe('暂时无法连接登录服务，请稍后重试。');
});

describe('nativeLoginErrorMessage', () => {
  it('never exposes provider or server error details', () => {
    expect(nativeLoginErrorMessage('google', new NativeIdentityFailure('state-mismatch')))
      .toBe('Google 返回的身份凭据无效，请重试。');
    expect(nativeLoginErrorMessage(
      'apple',
      new RadarApiError('secret provider detail', 503, 'request-id'),
    ))
      .toBe('Apple 登录尚未在服务器启用。');
  });

  it('explains when a native development build is required', () => {
    expect(nativeLoginErrorMessage(
      'google',
      new NativeIdentityFailure('native-module-unavailable'),
    )).toBe('Google 登录需要开发版或正式版 App。');
  });

  it('maps server reachability failures separately from provider failures', () => {
    expect(nativeLoginErrorMessage('apple', new TypeError('Network request failed')))
      .toBe('暂时无法连接登录服务，请稍后重试。');
  });
});

it('does not expose native credential-storage details', () => {
  const nativeMessage = 'KeyChainException: A required entitlement is not present';
  const result = loginErrorMessage(new Error(nativeMessage));

  expect(result).toBe('无法安全保存登录凭据，请重启 App 后重试。');
  expect(result).not.toContain(nativeMessage);
});

it('returns English copy when the UI locale is English', () => {
  expect(loginErrorMessage(
    new RadarApiError('private detail', 401, 'request-id'),
    createTranslator('en'),
  )).toBe('Incorrect email or password.');
});
