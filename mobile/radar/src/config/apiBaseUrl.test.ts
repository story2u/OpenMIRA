import { expect, it } from 'vitest';

import { MobileConfigurationError, parseApiBaseUrl } from './apiBaseUrl';

it('accepts a production HTTPS origin and removes its trailing slash', () => {
  expect(parseApiBaseUrl(' https://radar.example.test/ ', false)).toBe(
    'https://radar.example.test',
  );
});

it('allows only loopback HTTP during development', () => {
  expect(parseApiBaseUrl('http://10.0.2.2:8000', true)).toBe('http://10.0.2.2:8000');
  expect(() => parseApiBaseUrl('http://192.168.1.2:8000', true)).toThrow(
    MobileConfigurationError,
  );
  expect(() => parseApiBaseUrl('http://localhost:8000', false)).toThrow(
    MobileConfigurationError,
  );
});

it('rejects secrets, paths and missing configuration', () => {
  expect(() => parseApiBaseUrl(undefined, false)).toThrow('EXPO_PUBLIC_API_BASE_URL');
  expect(() => parseApiBaseUrl('https://user:secret@example.test', false)).toThrow('凭据');
  expect(() => parseApiBaseUrl('https://example.test/api/v1', false)).toThrow('origin');
});
