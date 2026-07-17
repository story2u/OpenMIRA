import { opportunityDetailRequestPath } from '@story2u/radar-api/opportunities';

export type SafeReturnTo = '/(tabs)/dashboard' | `/opportunity/${string}`;

export function safeReturnTo(value: string | string[] | undefined): SafeReturnTo {
  if (value === '/(tabs)/dashboard') return value;
  if (typeof value === 'string' && value.startsWith('/opportunity/')) {
    const opportunityId = value.slice('/opportunity/'.length);
    try {
      opportunityDetailRequestPath(opportunityId);
      return `/opportunity/${opportunityId}`;
    } catch {
      // Fall through to the authenticated default; never honor arbitrary return URLs.
    }
  }
  return '/(tabs)/dashboard';
}

export function opportunityIdFromRoute(value: string | string[] | undefined) {
  if (typeof value !== 'string') return null;
  try {
    opportunityDetailRequestPath(value);
    return value;
  } catch {
    return null;
  }
}
