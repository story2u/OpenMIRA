import { describe, expect, it } from 'vitest';

import { buildPushRegistration, extractCursorHint } from './pushCore';

describe('push cursor hints', () => {
  it('accepts only a positive versioned sync cursor through native payload wrappers', () => {
    expect(extractCursorHint({
      radar: { type: 'sync_cursor', schemaVersion: '1', cursor: '42' },
    })).toBe(42);
    expect(extractCursorHint({
      dataString: JSON.stringify({ type: 'sync_cursor', schemaVersion: '1', cursor: '43' }),
    })).toBe(43);
    expect(extractCursorHint({ type: 'sync_cursor', schemaVersion: '2', cursor: '42' })).toBeNull();
    expect(extractCursorHint({ type: 'sync_cursor', schemaVersion: '1', cursor: '-1' })).toBeNull();
    expect(extractCursorHint({ type: 'message', cursor: '42', body: 'secret' })).toBeNull();
  });

  it('maps native token type and build variant to direct platform providers', () => {
    expect(buildPushRegistration('ios', true, 'ios', 'a'.repeat(64))).toEqual({
      provider: 'apns',
      environment: 'sandbox',
      token: 'a'.repeat(64),
    });
    expect(buildPushRegistration('android', false, 'android', 'fcm-token-value-long')).toEqual({
      provider: 'fcm',
      environment: 'production',
      token: 'fcm-token-value-long',
    });
    expect(() => buildPushRegistration('ios', false, 'android', 'x'.repeat(64))).toThrow();
  });
});
