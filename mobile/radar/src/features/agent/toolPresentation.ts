import type { InteractiveToolName } from '@story2u/radar-agent/interactive';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';

const internalStatuses = new Set<InternalOpportunityStatus>([
  'pending_human',
  'ai_auto_reply',
  'replied',
  'following',
  'ignored',
  'closed',
]);

export type InteractiveToolPresentation =
  | { kind: 'claim_confirmed' }
  | { draft: string; kind: 'draft_local' }
  | { kind: 'reply_sent' }
  | { kind: 'status_queued'; status: InternalOpportunityStatus }
  | { kind: 'complete' };

export function interactiveToolPresentation(
  toolName: InteractiveToolName,
  result: Record<string, unknown>,
): InteractiveToolPresentation {
  if (
    toolName === 'send_reply'
    && result.sent === true
    && result.state === 'sent'
    && typeof result.approval_id === 'string'
  ) {
    return { kind: 'reply_sent' };
  }

  if (
    toolName === 'draft_reply'
    && typeof result.draft === 'string'
    && result.draft.length > 0
    && result.draft.length <= 4_000
    && result.state === 'local_only'
    && result.sent === false
  ) {
    return { draft: result.draft, kind: 'draft_local' };
  }

  if (
    toolName === 'update_status'
    && result.state === 'queued'
    && typeof result.status === 'string'
    && internalStatuses.has(result.status as InternalOpportunityStatus)
  ) {
    return {
      kind: 'status_queued',
      status: result.status as InternalOpportunityStatus,
    };
  }

  if (
    toolName === 'claim_opportunity'
    && result.claimed === true
    && result.state === 'confirmed'
  ) {
    return { kind: 'claim_confirmed' };
  }

  return { kind: 'complete' };
}
