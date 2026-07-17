import {
  ClaimOpportunityParameters,
  DraftReplyParameters,
  INTERACTIVE_INTERNAL_ACTION_TOOLS,
  UpdateStatusParameters,
  type InteractiveInternalToolName,
} from '@story2u/radar-agent/interactive';
import { RadarApiError } from '@story2u/radar-api/client';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { TSchema } from 'typebox';
import { Value } from 'typebox/value';

import {
  enqueueOpportunityStatusCommand,
  InternalCommandQueueError,
} from '../../sync/commandOutbox';
import {
  LocalProjectionCorruptError,
  OfflineProjectionUnavailableError,
  readOfflineOpportunityDetail,
} from '../../sync/offlineRepository';
import type { SyncStoreDatabase } from '../../sync/syncStore';

const statusCommandLifetimeMilliseconds = 7 * 24 * 60 * 60 * 1_000;
const actionToolSchemas: Readonly<Record<InteractiveInternalToolName, TSchema>> = Object.freeze({
  draft_reply: DraftReplyParameters,
  update_status: UpdateStatusParameters,
  claim_opportunity: ClaimOpportunityParameters,
});
const knownActionToolNames = new Set<InteractiveInternalToolName>(
  INTERACTIVE_INTERNAL_ACTION_TOOLS.map((tool) => tool.name as InteractiveInternalToolName),
);
const exposedQueueErrors = new Set([
  'sync_not_ready',
  'command_already_queued',
  'queue_full',
  'opportunity_not_queueable',
  'invalid_expiry',
]);

export interface InteractiveInternalToolCall {
  arguments: unknown;
  name: string;
}

export interface InteractiveInternalToolDependencies {
  claim(
    baseUrl: string,
    opportunityId: string,
    signal?: AbortSignal,
  ): Promise<OpportunityDetail>;
}

const installedDependencies: InteractiveInternalToolDependencies = {
  claim: async (baseUrl, opportunityId, signal) => (
    await import('../../api/client')
  ).claimOpportunity(baseUrl, opportunityId, signal),
};

export class InteractiveInternalToolError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'InteractiveInternalToolError';
  }
}

function stableToolError(error: unknown, signal?: AbortSignal) {
  if (signal?.aborted || (error instanceof Error && error.name === 'AbortError')) {
    return 'interactive_agent_cancelled';
  }
  if (error instanceof InteractiveInternalToolError) return error.code;
  if (error instanceof LocalProjectionCorruptError) return 'local_projection_corrupt';
  if (error instanceof OfflineProjectionUnavailableError) return 'local_projection_unavailable';
  if (error instanceof InternalCommandQueueError) {
    return exposedQueueErrors.has(error.code)
      ? error.code
      : 'internal_command_rejected';
  }
  if (error instanceof RadarApiError) {
    if (error.status === 401) return 'authentication_required';
    if (error.status === 404) return 'opportunity_not_found';
    if (error.status === 409) return 'opportunity_conflict';
    if (error.status >= 400 && error.status < 500) return 'internal_action_rejected';
    return 'internal_action_unavailable';
  }
  return 'internal_action_failed';
}

async function requireActiveOpportunity(
  database: SyncStoreDatabase,
  ownerId: string,
  opportunityId: string,
) {
  const opportunity = await readOfflineOpportunityDetail(database, ownerId, opportunityId);
  if (!opportunity) throw new InteractiveInternalToolError('opportunity_not_found');
  if (opportunity.archivedAt) {
    throw new InteractiveInternalToolError('opportunity_archived');
  }
  return opportunity;
}

export async function executeInteractiveInternalTool(
  database: SyncStoreDatabase,
  options: {
    allowedTools: ReadonlySet<InteractiveInternalToolName>;
    baseUrl: string;
    call: InteractiveInternalToolCall;
    dependencies?: InteractiveInternalToolDependencies;
    now?: Date;
    ownerId: string;
    randomId(): string;
    signal?: AbortSignal;
  },
): Promise<Record<string, unknown>> {
  const {
    allowedTools,
    baseUrl,
    call,
    dependencies = installedDependencies,
    now = new Date(),
    ownerId,
    randomId,
    signal,
  } = options;
  if (!knownActionToolNames.has(call.name as InteractiveInternalToolName)) {
    throw new InteractiveInternalToolError('unknown_tool');
  }
  const toolName = call.name as InteractiveInternalToolName;
  if (!allowedTools.has(toolName)) {
    throw new InteractiveInternalToolError('tool_not_authorized');
  }
  if (!Value.Check(actionToolSchemas[toolName], call.arguments)) {
    throw new InteractiveInternalToolError('invalid_tool_arguments');
  }
  if (signal?.aborted) throw new InteractiveInternalToolError('interactive_agent_cancelled');

  try {
    const args = call.arguments as {
      opportunity_id: string;
      status?: InternalOpportunityStatus;
      text?: string;
    };
    await requireActiveOpportunity(database, ownerId, args.opportunity_id);
    if (toolName === 'draft_reply') {
      const text = args.text?.trim() ?? '';
      if (!text || text.length > 4_000) {
        throw new InteractiveInternalToolError('invalid_tool_arguments');
      }
      return {
        opportunity_id: args.opportunity_id,
        draft: text,
        state: 'local_only',
        sent: false,
      };
    }
    if (toolName === 'update_status') {
      const commandId = randomId();
      const status = args.status as InternalOpportunityStatus;
      await enqueueOpportunityStatusCommand(database, {
        ownerId,
        opportunityId: args.opportunity_id,
        status,
        commandId,
        idempotencyKey: `agent-status:${commandId}`,
        expiresAt: new Date(now.getTime() + statusCommandLifetimeMilliseconds).toISOString(),
      }, now);
      if (signal?.aborted) throw new InteractiveInternalToolError('interactive_agent_cancelled');
      return {
        opportunity_id: args.opportunity_id,
        status,
        state: 'queued',
      };
    }
    const claimed = await dependencies.claim(baseUrl, args.opportunity_id, signal);
    if (signal?.aborted) throw new InteractiveInternalToolError('interactive_agent_cancelled');
    return {
      opportunity_id: claimed.id,
      claimed: true,
      state: 'confirmed',
    };
  } catch (error) {
    throw new InteractiveInternalToolError(stableToolError(error, signal));
  }
}
