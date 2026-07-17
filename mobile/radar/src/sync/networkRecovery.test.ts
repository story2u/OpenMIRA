import { describe, expect, it } from 'vitest';

import { createNetworkRecoveryDetector } from './networkRecovery';

describe('network recovery detector', () => {
  it('fires once only after a confirmed offline to online transition', () => {
    const detector = createNetworkRecoveryDetector();

    detector.seed({ isConnected: false, isInternetReachable: false });
    expect(detector.observe({ isConnected: false, isInternetReachable: false })).toBe(false);
    expect(detector.observe({ isConnected: true, isInternetReachable: true })).toBe(true);
    expect(detector.observe({ isConnected: true, isInternetReachable: true })).toBe(false);
  });

  it('does not treat the initial online state or an online transport change as recovery', () => {
    const detector = createNetworkRecoveryDetector();

    detector.seed({ isConnected: true, isInternetReachable: true });
    expect(detector.observe({ isConnected: true, isInternetReachable: true })).toBe(false);
    expect(detector.observe({ isConnected: true })).toBe(false);
  });

  it('keeps an offline observation through unknown states and captive connectivity', () => {
    const detector = createNetworkRecoveryDetector();

    detector.seed({ isConnected: false });
    expect(detector.observe({})).toBe(false);
    expect(detector.observe({ isConnected: true, isInternetReachable: false })).toBe(false);
    expect(detector.observe({ isConnected: true, isInternetReachable: true })).toBe(true);
  });
});
