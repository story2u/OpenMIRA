import { ErrorCode, Purchases, PurchasesError, type Offering } from '@revenuecat/purchases-js'
import { toBillingPackage } from './packages'
import type { BillingAvailability, BillingPackage } from './types'

const publicApiKey = process.env.NEXT_PUBLIC_REVENUECAT_WEB_API_KEY?.trim() ?? ''
const offeringId = process.env.NEXT_PUBLIC_REVENUECAT_OFFERING_ID?.trim() || 'default'
let activeAppUserId: string | null = null

function requireUserId(appUserId: string): string {
  const value = appUserId.trim()
  if (!value) throw new Error('登录完成后才能购买套餐')
  return value
}

export async function getWebPurchases(appUserId: string): Promise<Purchases | null> {
  if (!publicApiKey) return null
  const userId = requireUserId(appUserId)
  if (!Purchases.isConfigured()) {
    activeAppUserId = userId
    return Purchases.configure({ apiKey: publicApiKey, appUserId: userId })
  }
  const purchases = Purchases.getSharedInstance()
  if (activeAppUserId !== userId) {
    await purchases.changeUser(userId)
    activeAppUserId = userId
  }
  return purchases
}

function selectOffering(all: Record<string, Offering>, current: Offering | null): Offering | null {
  return all[offeringId] ?? (current?.identifier === offeringId ? current : null)
}

export async function loadWebBilling(appUserId: string): Promise<BillingAvailability> {
  const purchases = await getWebPurchases(appUserId)
  if (!purchases) return { status: 'unconfigured' }
  const offerings = await purchases.getOfferings()
  const offering = selectOffering(offerings.all, offerings.current)
  if (!offering) return { status: 'missing_offering' }
  const packages = Object.values(offering.packagesById)
    .map(toBillingPackage)
    .filter((item): item is BillingPackage => item !== null)
  return { status: 'ready', packages }
}

export async function purchaseWebPackage(appUserId: string, item: BillingPackage): Promise<'purchased' | 'cancelled'> {
  const purchases = await getWebPurchases(appUserId)
  if (!purchases) throw new Error('支付尚未配置')
  try {
    await purchases.purchase({ rcPackage: item.rcPackage, selectedLocale: 'zh-Hans' })
    return 'purchased'
  } catch (error) {
    if (error instanceof PurchasesError && error.errorCode === ErrorCode.UserCancelledError) {
      return 'cancelled'
    }
    throw error
  }
}
