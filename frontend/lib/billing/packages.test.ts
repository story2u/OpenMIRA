import { describe, expect, it } from 'vitest'
import { annualSavings, parsePackageIdentifier, toBillingPackage } from './packages'
import type { BillingPackage } from './types'

function packageStub(identifier: string, amountMicros = 1_000_000, currency = 'USD') {
  return {
    identifier,
    webBillingProduct: { price: { formattedPrice: '$1.00', amountMicros, currency } },
  } as BillingPackage['rcPackage']
}

describe('RevenueCat package mapping', () => {
  it('accepts only the six supported custom package identifiers', () => {
    expect(parsePackageIdentifier('pro_annual')).toEqual({ planCode: 'pro', interval: 'annual' })
    expect(parsePackageIdentifier('$rc_annual')).toBeNull()
    expect(parsePackageIdentifier('free_monthly')).toBeNull()
    expect(toBillingPackage(packageStub('future_weekly'))).toBeNull()
  })

  it('preserves the localized price returned by RevenueCat', () => {
    expect(toBillingPackage(packageStub('plus_monthly'))?.formattedPrice).toBe('$1.00')
  })

  it('calculates annual savings only for matching currencies', () => {
    const monthly = toBillingPackage(packageStub('max_monthly', 1_000_000)) ?? undefined
    const annual = toBillingPackage(packageStub('max_annual', 9_000_000)) ?? undefined
    expect(annualSavings(monthly, annual)).toBe(25)
    const otherCurrency = toBillingPackage(packageStub('max_annual', 9_000_000, 'EUR')) ?? undefined
    expect(annualSavings(monthly, otherCurrency)).toBeNull()
  })
})
