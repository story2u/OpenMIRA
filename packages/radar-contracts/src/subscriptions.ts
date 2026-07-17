import type { components } from './openapi';

export type PlanCode = components['schemas']['PlanCode'];
export type SubscriptionStatus = components['schemas']['SubscriptionStatus'];
export type BillingStore = components['schemas']['BillingStore'];
export type BillingInterval = components['schemas']['BillingInterval'];
export type PlanEntitlements = components['schemas']['PlanEntitlementsRead'];

type GeneratedSubscriptionUsage = components['schemas']['SubscriptionUsageRead'];
type GeneratedSubscriptionManagement = components['schemas']['SubscriptionManagementRead'];

export interface SubscriptionUsage extends Omit<
  GeneratedSubscriptionUsage,
  | 'billingInterval'
  | 'billingPeriodEnd'
  | 'billingPeriodStart'
  | 'effectiveStore'
  | 'entitlementExpiresAt'
  | 'lastSyncedAt'
  | 'managementUrl'
> {
  billingInterval: BillingInterval | null;
  billingPeriodEnd: string | null;
  billingPeriodStart: string | null;
  effectiveStore: BillingStore | null;
  entitlementExpiresAt: string | null;
  lastSyncedAt: string | null;
  managementUrl: string | null;
}

export type SubscriptionCatalogPlan = components['schemas']['SubscriptionCatalogPlanRead'];

export interface SubscriptionManagement extends Omit<
  GeneratedSubscriptionManagement,
  'managementUrl' | 'store'
> {
  managementUrl: string | null;
  store: BillingStore | null;
}

export type SubscriptionManagementClient = 'web' | 'ios' | 'android';
