import { RadarApiError } from '@story2u/radar-api/client';
import { expect, it } from 'vitest';

import { opportunityDetailErrorMessage } from './errors';
import { createTranslator } from '../../i18n/core';

it('maps detail failures without exposing server or native diagnostics', () => {
  expect(opportunityDetailErrorMessage(new RadarApiError('private detail', 404, 'request-id')))
    .toBe('商机不存在，或你没有权限查看。');
  expect(opportunityDetailErrorMessage(new RadarApiError('database trace', 503, 'request-id')))
    .toBe('商机服务暂时不可用，请稍后重试。');
  const native = 'Keychain failed with secret diagnostic';
  expect(opportunityDetailErrorMessage(new Error(native))).not.toContain(native);
});

it('localizes safe opportunity errors without exposing server detail', () => {
  const message = opportunityDetailErrorMessage(
    new RadarApiError('database trace', 503, 'request-id'),
    createTranslator('en'),
  );
  expect(message).toBe('The opportunity service is temporarily unavailable. Try again later.');
  expect(message).not.toContain('database trace');
});
