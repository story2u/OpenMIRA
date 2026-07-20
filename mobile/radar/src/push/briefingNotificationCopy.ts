import type {
  AttentionSnapshot,
  Briefing,
} from '@story2u/radar-core/briefing/model';

import type { Translator } from '../i18n/core';

export interface BriefingNotificationCopy {
  body: string;
  data: {
    briefingId: string;
    briefingType: Briefing['type'];
    generatedAt: string;
    kind: 'briefing';
  };
  title: string;
}

export function composeBriefingNotificationCopy(
  input: { briefing: Briefing; snapshot?: AttentionSnapshot | null },
  t: Translator,
): BriefingNotificationCopy {
  const actionCount = input.briefing.immediateCount + (input.snapshot?.needsUserInputCount ?? 0);
  const title = t('notifications.briefing.title');
  const body = actionCount > 0
    ? t('notifications.briefing.bodyAction', {
      count: actionCount,
      total: input.briefing.totalMessages,
    })
    : input.briefing.totalMessages > 0
      ? t('notifications.briefing.bodyDigest', {
        digest: input.briefing.digestCount,
        total: input.briefing.totalMessages,
      })
      : t('notifications.briefing.bodyEmpty');
  return {
    title,
    body,
    data: {
      kind: 'briefing',
      briefingId: input.briefing.id,
      briefingType: input.briefing.type,
      generatedAt: input.briefing.generatedAt,
    },
  };
}
