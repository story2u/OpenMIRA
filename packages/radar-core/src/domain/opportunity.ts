export const opportunityStatuses = ['pending', 'replied', 'ignored'] as const;
export type OpportunityStatus = (typeof opportunityStatuses)[number];

export const opportunityPriorities = ['low', 'normal', 'high', 'urgent'] as const;
export type OpportunityPriority = (typeof opportunityPriorities)[number];

export function isOpportunityStatus(value: unknown): value is OpportunityStatus {
  return typeof value === 'string' && opportunityStatuses.includes(value as OpportunityStatus);
}
