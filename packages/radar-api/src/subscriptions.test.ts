import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createSubscriptionsApi,
  decodeSubscriptionCatalog,
  decodeSubscriptionManagement,
  decodeSubscriptionUsage,
} from './subscriptions';

const entitlements = {
  planCode: 'plus',
  telegramGroupLimit: null,
  wecomGroupLimit: null,
  combinedGroupLimit: 10,
  piAgentAnalysisMonthlyLimit: 1_000,
} as const;

const usage = {
  planCode: 'plus',
  subscriptionStatus: 'active',
  periodStart: '2026-07-01T00:00:00Z',
  periodEnd: '2026-08-01T00:00:00Z',
  cancelAtPeriodEnd: false,
  entitlements,
  telegramGroupsUsed: 2,
  wecomGroupsUsed: 1,
  combinedGroupsUsed: 3,
  aiAnalysesConsumed: 220,
  aiAnalysesReserved: 5,
  aiAnalysesRemaining: 775,
  effectiveStore: 'app_store',
  billingInterval: 'monthly',
  billingPeriodStart: '2026-07-01T00:00:00Z',
  billingPeriodEnd: '2026-08-01T00:00:00Z',
  usagePeriodStart: '2026-07-01T00:00:00Z',
  usagePeriodEnd: '2026-08-01T00:00:00Z',
  entitlementExpiresAt: '2026-08-01T00:00:00Z',
  willRenew: true,
  billingIssue: false,
  multipleActiveSubscriptions: false,
  managementUrl: 'https://apps.apple.com/account/subscriptions',
  lastSyncedAt: '2026-07-17T02:00:00Z',
} as const;

const catalog = [
  {
    planCode: 'free',
    displayName: 'Free',
    rank: 0,
    entitlements: {
      planCode: 'free',
      telegramGroupLimit: 1,
      wecomGroupLimit: 1,
      combinedGroupLimit: 2,
      piAgentAnalysisMonthlyLimit: 100,
    },
    availableIntervals: [],
    revenuecatPackageIdentifiers: [],
  },
  {
    planCode: 'plus',
    displayName: 'Plus',
    rank: 1,
    entitlements,
    availableIntervals: ['monthly', 'annual'],
    revenuecatPackageIdentifiers: ['plus_monthly', 'plus_annual'],
  },
] as const;

const management = {
  store: 'app_store',
  managementUrl: 'https://apps.apple.com/account/subscriptions',
  instruction: 'Manage this subscription in Apple App Store subscriptions.',
  canOpenInCurrentClient: true,
} as const;

describe('subscriptions API', () => {
  it('strictly decodes server-authoritative usage and rejects inconsistent totals', () => {
    expect(decodeSubscriptionUsage(usage)).toEqual(usage);
    expect(() => decodeSubscriptionUsage({ ...usage, apiKey: 'secret' })).toThrow();
    expect(() => decodeSubscriptionUsage({ ...usage, combinedGroupsUsed: 9 })).toThrow('inconsistent');
    expect(() => decodeSubscriptionUsage({
      ...usage,
      managementUrl: 'http://billing.example.test/manage',
    })).toThrow('management URL');
  });

  it('validates catalog identity and package mappings before clients display them', () => {
    expect(decodeSubscriptionCatalog([...catalog].reverse())).toEqual(catalog);
    expect(() => decodeSubscriptionCatalog([catalog[1], catalog[1]])).toThrow('duplicates');
    expect(() => decodeSubscriptionCatalog([{
      ...catalog[1],
      revenuecatPackageIdentifiers: ['pro_monthly', 'plus_annual'],
    }])).toThrow('do not match');
  });

  it('only enables management when the backend supplies a secure URL for this client', () => {
    expect(decodeSubscriptionManagement(management)).toEqual(management);
    expect(() => decodeSubscriptionManagement({
      ...management,
      managementUrl: null,
    })).toThrow('required');
    expect(() => decodeSubscriptionManagement({
      ...management,
      managementUrl: 'https://user:password@example.test/manage',
    })).toThrow('management URL');
  });

  it('loads usage, catalog and client-scoped management independently and syncs without a payload', async () => {
    const fetch = vi.fn(async (input: string, init?: RequestInit) => {
      if (input.endsWith('/catalog')) return Response.json(catalog);
      if (input.includes('/management?client=ios')) return Response.json(management);
      expect(init?.body).toBeUndefined();
      return Response.json(usage);
    });
    const api = createSubscriptionsApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'access-token',
    }));

    await expect(Promise.all([
      api.usage(),
      api.catalog(),
      api.management('ios'),
    ])).resolves.toEqual([usage, catalog, management]);
    await expect(api.sync()).resolves.toEqual(usage);
    expect(fetch).toHaveBeenCalledTimes(4);
    expect(fetch.mock.calls[3][1]?.method).toBe('POST');
  });
});
