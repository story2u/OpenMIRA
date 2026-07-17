import { describe, expect, it } from 'vitest';

import { initialSubscriptionState, subscriptionReducer } from './state';

describe('subscription state', () => {
  it('prevents overlapping purchase and restore transitions', () => {
    const purchasing = subscriptionReducer(initialSubscriptionState, {
      type: 'purchase-started',
      packageId: 'plus_monthly',
    });
    expect(purchasing.busyPackageId).toBe('plus_monthly');
    expect(subscriptionReducer(purchasing, { type: 'restore-started' })).toBe(purchasing);
    expect(subscriptionReducer(purchasing, {
      type: 'purchase-started',
      packageId: 'pro_monthly',
    })).toBe(purchasing);
  });

  it('treats cancellation as a non-error and ignores stale package results', () => {
    const purchasing = subscriptionReducer(initialSubscriptionState, {
      type: 'purchase-started',
      packageId: 'plus_annual',
    });
    expect(subscriptionReducer(purchasing, {
      type: 'purchase-cancelled',
      packageId: 'pro_annual',
    })).toBe(purchasing);

    const cancelled = subscriptionReducer(purchasing, {
      type: 'purchase-cancelled',
      packageId: 'plus_annual',
    });
    expect(cancelled.busyPackageId).toBeNull();
    expect(cancelled.error).toBeNull();
    expect(cancelled.message).toContain('已取消购买');
  });
});
