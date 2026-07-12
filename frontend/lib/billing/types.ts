import type { Package } from '@revenuecat/purchases-js'
import type { BillingInterval, PlanCode } from '@/lib/types'

export interface BillingPackage {
  identifier: string
  planCode: Exclude<PlanCode, 'free'>
  interval: Exclude<BillingInterval, 'unknown'>
  formattedPrice: string
  amountMicros: number
  currency: string
  rcPackage: Package
}

export type BillingAvailability =
  | { status: 'unconfigured' }
  | { status: 'ready'; packages: BillingPackage[] }
  | { status: 'missing_offering' }
