import { expect, it } from 'vitest';

import { opportunityIdFromRoute, safeReturnTo } from './returnTo';

const id = '01234567-89ab-cdef-0123-456789abcdef';

it('accepts only local dashboard and valid opportunity return routes', () => {
  expect(safeReturnTo(`/opportunity/${id}`)).toBe(`/opportunity/${id}`);
  expect(safeReturnTo('/(tabs)/dashboard')).toBe('/(tabs)/dashboard');
  expect(safeReturnTo('https://evil.example.test')).toBe('/(tabs)/dashboard');
  expect(safeReturnTo('/opportunity/not-a-uuid')).toBe('/(tabs)/dashboard');
});

it('rejects ambiguous or malformed deep-link IDs before network access', () => {
  expect(opportunityIdFromRoute(id)).toBe(id);
  expect(opportunityIdFromRoute([id])).toBeNull();
  expect(opportunityIdFromRoute('not-a-uuid')).toBeNull();
  expect(opportunityIdFromRoute(undefined)).toBeNull();
});
