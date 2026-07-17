import { RadarApiError } from '@story2u/radar-api/client';
import { useCallback, useEffect, useReducer, useRef } from 'react';
import { Linking, Platform } from 'react-native';

import {
  loadRevenueCatBilling,
  purchaseRevenueCatPackage,
  restoreRevenueCatPurchases,
  type MobileBillingPackage,
} from '../../billing/revenueCat';
import {
  readSubscriptionOverview,
  syncSubscriptionOverview,
} from '../../api/client';
import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import type { Translator } from '../../i18n/core';
import { logEvent } from '../../logging/redactedLogger';
import { initialSubscriptionState, subscriptionReducer } from './state';

function managementClient() {
  return Platform.OS === 'ios' ? 'ios' as const : 'android' as const;
}

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

function backendLoadError(error: unknown, t: Translator) {
  if (error instanceof RadarApiError && error.status >= 500) {
    return t('subscription.error.unavailable');
  }
  return t('subscription.error.network');
}

function billingActionError(error: unknown, t: Translator) {
  if (error instanceof RadarApiError) {
    if (error.status === 429) return t('subscription.error.syncRate');
    if (error.status >= 500) return t('subscription.error.syncUnavailable');
  }
  return t('subscription.error.action');
}

export function useSubscription(
  userId: string,
  expireSession: () => Promise<void>,
) {
  const { t } = useI18n();
  const [state, dispatch] = useReducer(subscriptionReducer, initialSubscriptionState);
  const [revision, retry] = useReducer((value: number) => value + 1, 0);
  const actionRunning = useRef(false);

  const expireIfUnauthorized = useCallback(async (error: unknown) => {
    if (error instanceof RadarApiError && error.status === 401) {
      await expireSession();
      return true;
    }
    return false;
  }, [expireSession]);

  useEffect(() => {
    const controller = new AbortController();
    dispatch({ type: 'load-started' });

    async function load() {
      const [overviewResult, billingResult] = await Promise.allSettled([
        readSubscriptionOverview(
          getMobileApiBaseUrl(),
          managementClient(),
          controller.signal,
        ),
        loadRevenueCatBilling(userId),
      ]);
      if (controller.signal.aborted) return;
      if (billingResult.status === 'rejected') {
        logEvent('subscription.billing_load_failed', {
          errorClass: billingResult.reason instanceof Error ? billingResult.reason.name : 'UnknownError',
        });
      }
      if (overviewResult.status === 'rejected') {
        if (isAbortError(overviewResult.reason)) return;
        if (await expireIfUnauthorized(overviewResult.reason)) return;
        logEvent('subscription.load_failed', {
          errorClass: overviewResult.reason instanceof Error ? overviewResult.reason.name : 'UnknownError',
          status: overviewResult.reason instanceof RadarApiError ? overviewResult.reason.status : null,
        });
        dispatch({ type: 'load-failed', error: backendLoadError(overviewResult.reason, t) });
        return;
      }
      dispatch({
        type: 'load-succeeded',
        ...overviewResult.value,
        billing: billingResult.status === 'fulfilled'
          ? billingResult.value
          : { status: 'unavailable' },
      });
    }

    void load();
    return () => controller.abort();
  }, [expireIfUnauthorized, revision, t, userId]);

  const purchase = useCallback(async (item: MobileBillingPackage) => {
    if (actionRunning.current || state.usage?.planCode !== 'free' || state.billing?.status !== 'ready') {
      return false;
    }
    if (!state.billing.packages.some((candidate) => candidate.identifier === item.identifier)) return false;
    actionRunning.current = true;
    dispatch({ type: 'purchase-started', packageId: item.identifier });
    let storePurchaseCompleted = false;
    try {
      if (await purchaseRevenueCatPackage(item) === 'cancelled') {
        dispatch({
          type: 'purchase-cancelled',
          packageId: item.identifier,
          message: t('subscription.message.cancelled'),
        });
        return false;
      }
      storePurchaseCompleted = true;
      const result = await syncSubscriptionOverview(getMobileApiBaseUrl(), managementClient());
      dispatch({
        type: 'purchase-succeeded',
        packageId: item.identifier,
        ...result,
        message: result.usage.planCode === item.planCode
          ? t('subscription.message.active')
          : t('subscription.message.pending'),
      });
      return true;
    } catch (error) {
      if (!(await expireIfUnauthorized(error))) {
        logEvent('subscription.purchase_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({
          type: 'purchase-failed',
          packageId: item.identifier,
          error: storePurchaseCompleted
            ? t('subscription.error.purchaseSync')
            : billingActionError(error, t),
        });
      }
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [expireIfUnauthorized, state.billing, state.usage?.planCode, t]);

  const restore = useCallback(async () => {
    if (actionRunning.current || state.billing?.status !== 'ready') return false;
    actionRunning.current = true;
    dispatch({ type: 'restore-started' });
    let storeRestoreCompleted = false;
    try {
      await restoreRevenueCatPurchases(userId);
      storeRestoreCompleted = true;
      const result = await syncSubscriptionOverview(getMobileApiBaseUrl(), managementClient());
      dispatch({
        type: 'restore-succeeded',
        ...result,
        message: t('subscription.message.restored'),
      });
      return true;
    } catch (error) {
      if (!(await expireIfUnauthorized(error))) {
        logEvent('subscription.restore_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({
          type: 'restore-failed',
          error: storeRestoreCompleted
            ? t('subscription.error.restoreSync')
            : billingActionError(error, t),
        });
      }
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [expireIfUnauthorized, state.billing?.status, t, userId]);

  const openManagement = useCallback(async () => {
    const url = state.management?.managementUrl;
    if (!state.management?.canOpenInCurrentClient || !url) return false;
    try {
      await Linking.openURL(url);
      return true;
    } catch {
      dispatch({
        type: 'management-open-failed',
        error: t('subscription.error.management'),
      });
      return false;
    }
  }, [state.management, t]);

  return {
    openManagement,
    purchase,
    restore,
    retry,
    selectInterval: (interval: 'monthly' | 'annual') =>
      dispatch({ type: 'interval-selected', interval }),
    state,
  };
}
