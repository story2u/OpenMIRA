import type {
  BillingInterval,
  PlanEntitlements,
  SubscriptionCatalogPlan,
  SubscriptionManagement,
  SubscriptionManagementClient,
  SubscriptionUsage,
} from '@story2u/radar-contracts/subscriptions';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';
const safeCount = { minimum: 0, maximum: 1_000_000_000 } as const;
const planCode = Type.Union([
  Type.Literal('free'),
  Type.Literal('plus'),
  Type.Literal('pro'),
  Type.Literal('max'),
]);
const billingInterval = Type.Union([
  Type.Literal('monthly'),
  Type.Literal('annual'),
  Type.Literal('unknown'),
]);
const purchasableInterval = Type.Union([
  Type.Literal('monthly'),
  Type.Literal('annual'),
]);
const billingStore = Type.Union([
  Type.Literal('app_store'),
  Type.Literal('play_store'),
  Type.Literal('paddle'),
  Type.Literal('test_store'),
  Type.Literal('unknown'),
]);
const nullableDateTime = Type.Union([Type.String({ pattern: dateTimePattern }), Type.Null()]);
const nullableHttpsUrl = Type.Union([Type.String({ minLength: 1, maxLength: 4096 }), Type.Null()]);

export const PlanEntitlementsSchema = Type.Object(
  {
    planCode,
    telegramGroupLimit: Type.Union([Type.Integer(safeCount), Type.Null()]),
    wecomGroupLimit: Type.Union([Type.Integer(safeCount), Type.Null()]),
    combinedGroupLimit: Type.Integer(safeCount),
    piAgentAnalysisMonthlyLimit: Type.Integer(safeCount),
  },
  { additionalProperties: false },
);

export const SubscriptionUsageSchema = Type.Object(
  {
    planCode,
    subscriptionStatus: Type.Union([
      Type.Literal('active'),
      Type.Literal('trialing'),
      Type.Literal('past_due'),
      Type.Literal('canceled'),
      Type.Literal('inactive'),
    ]),
    periodStart: Type.String({ pattern: dateTimePattern }),
    periodEnd: Type.String({ pattern: dateTimePattern }),
    cancelAtPeriodEnd: Type.Boolean(),
    entitlements: PlanEntitlementsSchema,
    telegramGroupsUsed: Type.Integer(safeCount),
    wecomGroupsUsed: Type.Integer(safeCount),
    combinedGroupsUsed: Type.Integer(safeCount),
    aiAnalysesConsumed: Type.Integer(safeCount),
    aiAnalysesReserved: Type.Integer(safeCount),
    aiAnalysesRemaining: Type.Integer(safeCount),
    effectiveStore: Type.Union([billingStore, Type.Null()]),
    billingInterval: Type.Union([billingInterval, Type.Null()]),
    billingPeriodStart: nullableDateTime,
    billingPeriodEnd: nullableDateTime,
    usagePeriodStart: Type.String({ pattern: dateTimePattern }),
    usagePeriodEnd: Type.String({ pattern: dateTimePattern }),
    entitlementExpiresAt: nullableDateTime,
    willRenew: Type.Boolean(),
    billingIssue: Type.Boolean(),
    multipleActiveSubscriptions: Type.Boolean(),
    managementUrl: nullableHttpsUrl,
    lastSyncedAt: nullableDateTime,
  },
  { additionalProperties: false },
);

export const SubscriptionCatalogPlanSchema = Type.Object(
  {
    planCode,
    displayName: Type.String({ minLength: 1, maxLength: 128 }),
    rank: Type.Integer({ minimum: 0, maximum: 100 }),
    entitlements: PlanEntitlementsSchema,
    availableIntervals: Type.Array(purchasableInterval, { maxItems: 2 }),
    revenuecatPackageIdentifiers: Type.Array(Type.String({ minLength: 1, maxLength: 128 }), { maxItems: 8 }),
  },
  { additionalProperties: false },
);

export const SubscriptionManagementSchema = Type.Object(
  {
    store: Type.Union([billingStore, Type.Null()]),
    managementUrl: nullableHttpsUrl,
    instruction: Type.String({ minLength: 1, maxLength: 1000 }),
    canOpenInCurrentClient: Type.Boolean(),
  },
  { additionalProperties: false },
);

const parseEntitlements = typeboxDecoder(PlanEntitlementsSchema);
const parseUsage = typeboxDecoder(SubscriptionUsageSchema);
const parseCatalog = typeboxDecoder(Type.Array(SubscriptionCatalogPlanSchema, { maxItems: 16 }));
const parseManagement = typeboxDecoder(SubscriptionManagementSchema);

function requireHttpsUrl(value: string | null, label: string) {
  if (!value) return;
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`Invalid ${label}`);
  }
  if (parsed.protocol !== 'https:' || !parsed.hostname || parsed.username || parsed.password) {
    throw new Error(`Invalid ${label}`);
  }
}

function requireDateOrder(start: string, end: string, label: string) {
  if (Date.parse(start) >= Date.parse(end)) throw new Error(`${label} end must be after start`);
}

function requireUnique<T>(items: readonly T[], label: string) {
  if (new Set(items).size !== items.length) throw new Error(`${label} contains duplicates`);
}

export const decodePlanEntitlements: ResponseDecoder<PlanEntitlements> = (value) =>
  parseEntitlements(value) as PlanEntitlements;

export const decodeSubscriptionUsage: ResponseDecoder<SubscriptionUsage> = (value) => {
  const parsed = parseUsage(value) as SubscriptionUsage;
  if (parsed.entitlements.planCode !== parsed.planCode) {
    throw new Error('Subscription entitlement plan does not match current plan');
  }
  if (parsed.combinedGroupsUsed !== parsed.telegramGroupsUsed + parsed.wecomGroupsUsed) {
    throw new Error('Subscription combined group usage is inconsistent');
  }
  if (parsed.aiAnalysesRemaining > parsed.entitlements.piAgentAnalysisMonthlyLimit) {
    throw new Error('Subscription remaining analysis usage exceeds plan limit');
  }
  requireDateOrder(parsed.periodStart, parsed.periodEnd, 'Subscription period');
  requireDateOrder(parsed.usagePeriodStart, parsed.usagePeriodEnd, 'Subscription usage period');
  if (parsed.billingPeriodStart && parsed.billingPeriodEnd) {
    requireDateOrder(parsed.billingPeriodStart, parsed.billingPeriodEnd, 'Subscription billing period');
  }
  requireHttpsUrl(parsed.managementUrl, 'subscription management URL');
  return parsed;
};

export const decodeSubscriptionCatalog: ResponseDecoder<SubscriptionCatalogPlan[]> = (value) => {
  const parsed = parseCatalog(value) as SubscriptionCatalogPlan[];
  requireUnique(parsed.map((item) => item.planCode), 'Subscription catalog plan codes');
  requireUnique(parsed.map((item) => item.rank), 'Subscription catalog ranks');
  for (const item of parsed) {
    if (item.entitlements.planCode !== item.planCode) {
      throw new Error('Subscription catalog entitlement plan does not match plan');
    }
    requireUnique(item.availableIntervals, 'Subscription catalog intervals');
    requireUnique(item.revenuecatPackageIdentifiers, 'Subscription package identifiers');
    const expected = new Set(item.availableIntervals.map((interval) => `${item.planCode}_${interval}`));
    if (
      item.revenuecatPackageIdentifiers.length !== expected.size ||
      item.revenuecatPackageIdentifiers.some((identifier) => !expected.has(identifier))
    ) {
      throw new Error('Subscription package identifiers do not match plan intervals');
    }
  }
  return parsed.sort((left, right) => left.rank - right.rank);
};

export const decodeSubscriptionManagement: ResponseDecoder<SubscriptionManagement> = (value) => {
  const parsed = parseManagement(value) as SubscriptionManagement;
  requireHttpsUrl(parsed.managementUrl, 'subscription management URL');
  if (parsed.canOpenInCurrentClient && !parsed.managementUrl) {
    throw new Error('Subscription management URL is required for the current client');
  }
  return parsed;
};

function requireManagementClient(client: string): asserts client is SubscriptionManagementClient {
  if (client !== 'web' && client !== 'ios' && client !== 'android') {
    throw new Error('Invalid subscription management client');
  }
}

export function createSubscriptionsApi(client: RadarApiClient) {
  return {
    plans(init: Pick<RequestInit, 'signal'> = {}): Promise<PlanEntitlements[]> {
      return client.request('/api/v1/subscriptions/plans', {
        ...init,
        decode: (value) => {
          const values = typeboxDecoder(Type.Array(PlanEntitlementsSchema, { maxItems: 16 }))(value) as PlanEntitlements[];
          requireUnique(values.map((item) => item.planCode), 'Subscription plans');
          return values;
        },
      });
    },

    catalog(init: Pick<RequestInit, 'signal'> = {}): Promise<SubscriptionCatalogPlan[]> {
      return client.request('/api/v1/subscriptions/catalog', {
        ...init,
        decode: decodeSubscriptionCatalog,
      });
    },

    usage(init: Pick<RequestInit, 'signal'> = {}): Promise<SubscriptionUsage> {
      return client.request('/api/v1/subscriptions/me', {
        ...init,
        decode: decodeSubscriptionUsage,
      });
    },

    sync(init: Pick<RequestInit, 'signal'> = {}): Promise<SubscriptionUsage> {
      return client.request('/api/v1/subscriptions/sync', {
        ...init,
        method: 'POST',
        decode: decodeSubscriptionUsage,
      });
    },

    management(
      managementClient: SubscriptionManagementClient,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SubscriptionManagement> {
      requireManagementClient(managementClient);
      return client.request(`/api/v1/subscriptions/management?client=${managementClient}`, {
        ...init,
        decode: decodeSubscriptionManagement,
      });
    },
  };
}
