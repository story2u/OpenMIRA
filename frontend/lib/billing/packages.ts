import type { Package } from '@revenuecat/purchases-js'
import type { BillingInterval, PlanCode } from '@/lib/types'
import type { BillingPackage } from './types'

const PACKAGE_PATTERN = /^(plus|pro|max)_(monthly|annual)$/

export function parsePackageIdentifier(identifier: string): {
  planCode: Exclude<PlanCode, 'free'>
  interval: Exclude<BillingInterval, 'unknown'>
} | null {
  const match = PACKAGE_PATTERN.exec(identifier)
  if (!match) return null
  return {
    planCode: match[1] as Exclude<PlanCode, 'free'>,
    interval: match[2] as Exclude<BillingInterval, 'unknown'>,
  }
}

export function toBillingPackage(rcPackage: Package): BillingPackage | null {
  const identity = parsePackageIdentifier(rcPackage.identifier)
  if (!identity) return null
  const price = rcPackage.webBillingProduct.price
  return {
    identifier: rcPackage.identifier,
    ...identity,
    formattedPrice: price.formattedPrice,
    amountMicros: price.amountMicros,
    currency: price.currency,
    rcPackage,
  }
}

export function annualSavings(monthly: BillingPackage | undefined, annual: BillingPackage | undefined): number | null {
  if (!monthly || !annual || monthly.currency !== annual.currency || monthly.amountMicros <= 0) return null
  return Math.max(Math.round((1 - annual.amountMicros / (monthly.amountMicros * 12)) * 100), 0)
}
