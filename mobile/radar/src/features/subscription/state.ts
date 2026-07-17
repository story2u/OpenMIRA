import type {
  BillingInterval,
  SubscriptionCatalogPlan,
  SubscriptionManagement,
  SubscriptionUsage,
} from '@story2u/radar-contracts/subscriptions';

import type { MobileBillingAvailability } from '../../billing/revenueCat';
import { fallbackTranslator } from '../../i18n/core';

export interface SubscriptionState {
  billing: MobileBillingAvailability | null;
  busyPackageId: string | null;
  catalog: SubscriptionCatalogPlan[];
  error: string | null;
  interval: Exclude<BillingInterval, 'unknown'>;
  isLoading: boolean;
  isRestoring: boolean;
  management: SubscriptionManagement | null;
  message: string | null;
  usage: SubscriptionUsage | null;
}

export const initialSubscriptionState: SubscriptionState = {
  billing: null,
  busyPackageId: null,
  catalog: [],
  error: null,
  interval: 'monthly',
  isLoading: true,
  isRestoring: false,
  management: null,
  message: null,
  usage: null,
};

export type SubscriptionAction =
  | { type: 'load-started' }
  | {
      type: 'load-succeeded';
      billing: MobileBillingAvailability;
      catalog: SubscriptionCatalogPlan[];
      management: SubscriptionManagement;
      usage: SubscriptionUsage;
    }
  | { type: 'load-failed'; error: string }
  | { type: 'interval-selected'; interval: Exclude<BillingInterval, 'unknown'> }
  | { type: 'purchase-started'; packageId: string }
  | { type: 'purchase-cancelled'; packageId: string; message?: string }
  | {
      type: 'purchase-succeeded';
      packageId: string;
      management: SubscriptionManagement;
      message: string;
      usage: SubscriptionUsage;
    }
  | { type: 'purchase-failed'; packageId: string; error: string }
  | { type: 'restore-started' }
  | {
      type: 'restore-succeeded';
      management: SubscriptionManagement;
      message?: string;
      usage: SubscriptionUsage;
    }
  | { type: 'restore-failed'; error: string }
  | { type: 'management-open-failed'; error: string };

export function subscriptionReducer(
  state: SubscriptionState,
  action: SubscriptionAction,
): SubscriptionState {
  switch (action.type) {
    case 'load-started':
      return { ...state, isLoading: true, error: null };
    case 'load-succeeded':
      return {
        ...state,
        billing: action.billing,
        catalog: action.catalog,
        management: action.management,
        usage: action.usage,
        isLoading: false,
        error: null,
      };
    case 'load-failed':
      return { ...state, isLoading: false, error: action.error };
    case 'interval-selected':
      return { ...state, interval: action.interval };
    case 'purchase-started':
      if (state.busyPackageId || state.isRestoring) return state;
      return { ...state, busyPackageId: action.packageId, error: null, message: null };
    case 'purchase-cancelled':
      if (state.busyPackageId !== action.packageId) return state;
      return {
        ...state,
        busyPackageId: null,
        message: action.message ?? fallbackTranslator('subscription.message.cancelled'),
      };
    case 'purchase-succeeded':
      if (state.busyPackageId !== action.packageId) return state;
      return {
        ...state,
        busyPackageId: null,
        management: action.management,
        message: action.message,
        usage: action.usage,
      };
    case 'purchase-failed':
      if (state.busyPackageId !== action.packageId) return state;
      return { ...state, busyPackageId: null, error: action.error };
    case 'restore-started':
      if (state.busyPackageId || state.isRestoring) return state;
      return { ...state, isRestoring: true, error: null, message: null };
    case 'restore-succeeded':
      if (!state.isRestoring) return state;
      return {
        ...state,
        isRestoring: false,
        management: action.management,
        message: action.message ?? fallbackTranslator('subscription.message.restored'),
        usage: action.usage,
      };
    case 'restore-failed':
      if (!state.isRestoring) return state;
      return { ...state, isRestoring: false, error: action.error };
    case 'management-open-failed':
      return { ...state, error: action.error };
  }
}
