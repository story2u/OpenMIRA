import { describe, expect, it } from 'vitest';

import { parseBillingPackageIdentifier } from './packageIdentity';

describe('RevenueCat package identity', () => {
  it('accepts only the six backend-owned plan and interval identifiers', () => {
    expect(parseBillingPackageIdentifier('plus_monthly')).toEqual({
      planCode: 'plus',
      interval: 'monthly',
    });
    expect(parseBillingPackageIdentifier('max_annual')).toEqual({
      planCode: 'max',
      interval: 'annual',
    });
    expect(parseBillingPackageIdentifier('free_monthly')).toBeNull();
    expect(parseBillingPackageIdentifier('plus_lifetime')).toBeNull();
    expect(parseBillingPackageIdentifier('$rc_monthly')).toBeNull();
  });
});
