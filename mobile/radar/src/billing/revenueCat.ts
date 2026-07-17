import type {
  BillingInterval,
  PlanCode,
} from '@story2u/radar-contracts/subscriptions';
import Constants, { AppOwnership } from 'expo-constants';
import { Platform } from 'react-native';
import Purchases, {
  type PurchasesError,
  type PurchasesOffering,
  type PurchasesPackage,
} from 'react-native-purchases';

import { parseBillingPackageIdentifier } from './packageIdentity';

export interface MobileBillingPackage {
  identifier: string;
  interval: Exclude<BillingInterval, 'unknown'>;
  localizedPrice: string;
  nativePackage: PurchasesPackage;
  planCode: Exclude<PlanCode, 'free'>;
}

export type MobileBillingAvailability =
  | { status: 'unconfigured' }
  | { status: 'preview-unavailable' }
  | { status: 'missing-offering' }
  | { status: 'unavailable' }
  | { status: 'ready'; packages: MobileBillingPackage[] };

const offeringId = process.env.EXPO_PUBLIC_REVENUECAT_OFFERING_ID?.trim() || 'default';
let identityTask: Promise<void> = Promise.resolve();

function publicApiKey() {
  if (Platform.OS === 'ios') return process.env.EXPO_PUBLIC_REVENUECAT_IOS_API_KEY?.trim() ?? '';
  if (Platform.OS === 'android') return process.env.EXPO_PUBLIC_REVENUECAT_ANDROID_API_KEY?.trim() ?? '';
  return '';
}

function requireUserId(appUserId: string) {
  const value = appUserId.trim();
  if (!/^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(value)) {
    throw new Error('登录账户无效，暂时无法连接购买服务');
  }
  return value;
}

function isExpoGo() {
  return Constants.appOwnership === AppOwnership.Expo;
}

async function identify(appUserId: string, apiKey: string) {
  const userId = requireUserId(appUserId);
  identityTask = identityTask.catch(() => undefined).then(async () => {
    if (!(await Purchases.isConfigured())) {
      Purchases.configure({ apiKey, appUserID: userId });
      return;
    }
    if (await Purchases.getAppUserID() !== userId) await Purchases.logIn(userId);
  });
  return identityTask;
}

function selectOffering(
  all: Record<string, PurchasesOffering>,
  current: PurchasesOffering | null,
) {
  return all[offeringId] ?? (current?.identifier === offeringId ? current : null);
}

function toMobilePackage(nativePackage: PurchasesPackage): MobileBillingPackage | null {
  const identity = parseBillingPackageIdentifier(nativePackage.identifier);
  if (!identity) return null;
  return {
    identifier: nativePackage.identifier,
    ...identity,
    localizedPrice: nativePackage.product.priceString,
    nativePackage,
  };
}

export async function loadRevenueCatBilling(appUserId: string): Promise<MobileBillingAvailability> {
  const apiKey = publicApiKey();
  if (!apiKey) return { status: 'unconfigured' };
  if (isExpoGo()) return { status: 'preview-unavailable' };
  await identify(appUserId, apiKey);
  const offerings = await Purchases.getOfferings();
  const offering = selectOffering(offerings.all, offerings.current);
  if (!offering) return { status: 'missing-offering' };
  const packages = offering.availablePackages
    .map(toMobilePackage)
    .filter((item): item is MobileBillingPackage => item !== null);
  return packages.length > 0 ? { status: 'ready', packages } : { status: 'missing-offering' };
}

function isPurchaseCancelled(error: unknown): error is PurchasesError {
  if (!error || typeof error !== 'object') return false;
  const candidate = error as Partial<PurchasesError>;
  return candidate.code === Purchases.PURCHASES_ERROR_CODE.PURCHASE_CANCELLED_ERROR ||
    candidate.userCancelled === true;
}

export async function purchaseRevenueCatPackage(
  item: MobileBillingPackage,
): Promise<'purchased' | 'cancelled'> {
  try {
    await Purchases.purchasePackage(item.nativePackage);
    return 'purchased';
  } catch (error) {
    if (isPurchaseCancelled(error)) return 'cancelled';
    throw error;
  }
}

export async function restoreRevenueCatPurchases(appUserId: string): Promise<void> {
  const apiKey = publicApiKey();
  if (!apiKey || isExpoGo()) throw new Error('支付尚未配置');
  await identify(appUserId, apiKey);
  await Purchases.restorePurchases();
}
