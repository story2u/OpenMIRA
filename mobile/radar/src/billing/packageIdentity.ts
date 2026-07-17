import type { BillingInterval, PlanCode } from '@story2u/radar-contracts/subscriptions';

const PACKAGE_PATTERN = /^(plus|pro|max)_(monthly|annual)$/;

export interface BillingPackageIdentity {
  interval: Exclude<BillingInterval, 'unknown'>;
  planCode: Exclude<PlanCode, 'free'>;
}

export function parseBillingPackageIdentifier(identifier: string): BillingPackageIdentity | null {
  const match = PACKAGE_PATTERN.exec(identifier);
  if (!match) return null;
  return {
    planCode: match[1] as BillingPackageIdentity['planCode'],
    interval: match[2] as BillingPackageIdentity['interval'],
  };
}
