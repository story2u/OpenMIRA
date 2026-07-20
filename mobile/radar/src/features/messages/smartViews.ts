import type { Dashboard } from '@story2u/radar-contracts/opportunities';
import type { AttentionSnapshot } from '@story2u/radar-core/briefing/model';

import type { MessageKey } from '../../i18n/catalog';

export type SmartMessageTarget =
  | 'all'
  | 'business'
  | 'digest'
  | 'jobs'
  | 'judgment'
  | 'pending'
  | 'quiet';
export type SmartMessageSectionId = SmartMessageTarget | 'attention';

export interface SmartMessageSection {
  count: number;
  detailKey: MessageKey;
  id: SmartMessageSectionId;
  target: SmartMessageTarget;
  tone: 'action' | 'calm' | 'focus' | 'muted';
  titleKey: MessageKey;
}

export function buildSmartMessageSections(
  dashboard: Dashboard | null | undefined,
  snapshot: AttentionSnapshot | null | undefined,
): SmartMessageSection[] {
  const items = dashboard?.items ?? [];
  const jobs = items.filter((item) => item.opportunityType === 'job').length;
  const business = Math.max(0, items.length - jobs);
  const attention = dashboard?.attentionItems?.length ?? 0;
  const pending = dashboard?.pendingCount ?? 0;
  return [
    {
      count: attention,
      detailKey: 'messages.smart.attention.detail',
      id: 'attention',
      target: 'pending',
      tone: attention > 0 ? 'action' : 'calm',
      titleKey: 'messages.smart.attention.title',
    },
    {
      count: pending,
      detailKey: 'messages.smart.pending.detail',
      id: 'pending',
      target: 'pending',
      tone: pending > 0 ? 'focus' : 'calm',
      titleKey: 'messages.smart.pending.title',
    },
    {
      count: business,
      detailKey: 'messages.smart.business.detail',
      id: 'business',
      target: 'business',
      tone: business > 0 ? 'focus' : 'muted',
      titleKey: 'messages.smart.business.title',
    },
    {
      count: jobs,
      detailKey: 'messages.smart.jobs.detail',
      id: 'jobs',
      target: 'jobs',
      tone: jobs > 0 ? 'focus' : 'muted',
      titleKey: 'messages.smart.jobs.title',
    },
    {
      count: snapshot?.needsUserInputCount ?? 0,
      detailKey: 'messages.smart.judgment.detail',
      id: 'judgment',
      target: 'judgment',
      tone: (snapshot?.needsUserInputCount ?? 0) > 0 ? 'action' : 'calm',
      titleKey: 'messages.smart.judgment.title',
    },
    {
      count: snapshot?.digestCount ?? 0,
      detailKey: 'messages.smart.digest.detail',
      id: 'digest',
      target: 'digest',
      tone: (snapshot?.digestCount ?? 0) > 0 ? 'calm' : 'muted',
      titleKey: 'messages.smart.digest.title',
    },
    {
      count: snapshot?.suppressedCount ?? 0,
      detailKey: 'messages.smart.quiet.detail',
      id: 'quiet',
      target: 'quiet',
      tone: 'muted',
      titleKey: 'messages.smart.quiet.title',
    },
    {
      count: dashboard?.total ?? 0,
      detailKey: 'messages.smart.all.detail',
      id: 'all',
      target: 'all',
      tone: 'calm',
      titleKey: 'messages.smart.all.title',
    },
  ];
}
