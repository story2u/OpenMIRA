import type {
  InteractiveAgentApprovalDecision,
  InteractiveAgentApprovalDecisionRequest,
  InteractiveAgentApprovedSend,
  InteractiveAgentApprovedSendRequest,
} from '@story2u/radar-contracts/interactive-agent';

import { readOfflineOpportunityDetail } from '../../sync/offlineRepository';
import type { SyncStoreDatabase } from '../../sync/syncStore';

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

interface SendReplyArguments {
  opportunity_id: string;
  text: string;
}

export interface InteractiveSendApprovalRequest {
  channel: 'telegram' | 'wecom';
  expectedVersion: number;
  idempotencyKey: string;
  opportunityId: string;
  proposedText: string;
  targetLabel: string;
  toolCallId: string;
}

export interface InteractiveSendApprovalDecision {
  approved: boolean;
  text: string;
}

export type RequestInteractiveSendApproval = (
  request: InteractiveSendApprovalRequest,
  signal?: AbortSignal,
) => Promise<InteractiveSendApprovalDecision>;

export interface InteractiveApprovedSendDependencies {
  decide(
    baseUrl: string,
    turnToken: string,
    payload: InteractiveAgentApprovalDecisionRequest,
    signal?: AbortSignal,
  ): Promise<InteractiveAgentApprovalDecision>;
  execute(
    baseUrl: string,
    approvalToken: string,
    payload: InteractiveAgentApprovedSendRequest,
    signal?: AbortSignal,
  ): Promise<InteractiveAgentApprovedSend>;
}

function sendReplyArguments(value: unknown): SendReplyArguments {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('interactive_agent_action_invalid');
  }
  const input = value as Record<string, unknown>;
  if (
    Object.keys(input).length !== 2
    || typeof input.opportunity_id !== 'string'
    || !uuidPattern.test(input.opportunity_id)
    || typeof input.text !== 'string'
    || input.text.length < 1
    || input.text.length > 4_000
    || !input.text.trim()
  ) {
    throw new Error('interactive_agent_action_invalid');
  }
  return { opportunity_id: input.opportunity_id.toLowerCase(), text: input.text };
}

function approvedText(value: string) {
  if (value.length < 1 || value.length > 4_000 || !value.trim()) {
    throw new Error('interactive_agent_action_invalid');
  }
  return value;
}

interface ProjectionVersionRow {
  aggregate_version: number;
  archived_at: string | null;
  deleted_at: string | null;
}

interface PendingApproval {
  approvalId: string;
  approvalToken: string;
  payload: InteractiveAgentApprovedSendRequest;
}

export function createInteractiveApprovedSendCoordinator(options: {
  baseUrl: string;
  database: SyncStoreDatabase;
  dependencies: InteractiveApprovedSendDependencies;
  ownerId: string;
  randomId(): string;
  requestApproval: RequestInteractiveSendApproval;
  turnToken: string;
}) {
  const { dependencies } = options;
  const pending = new Map<string, PendingApproval>();

  const prepare = async (
    toolCallId: string,
    rawArguments: unknown,
    signal?: AbortSignal,
  ) => {
    const args = sendReplyArguments(rawArguments);
    const [opportunity, projection] = await Promise.all([
      readOfflineOpportunityDetail(options.database, options.ownerId, args.opportunity_id),
      options.database.getFirstAsync<ProjectionVersionRow>(
        `SELECT aggregate_version, archived_at, deleted_at
         FROM opportunity_projection WHERE owner_id = ? AND id = ?`,
        options.ownerId,
        args.opportunity_id,
      ),
    ]);
    if (
      signal?.aborted
      || !opportunity
      || opportunity.archivedAt !== null
      || !projection
      || projection.archived_at !== null
      || projection.deleted_at !== null
      || !Number.isSafeInteger(projection.aggregate_version)
      || projection.aggregate_version < 1
    ) {
      throw new Error('interactive_agent_action_unavailable');
    }
    const request: InteractiveSendApprovalRequest = {
      channel: opportunity.platform,
      expectedVersion: projection.aggregate_version,
      idempotencyKey: `agent-reply:${options.randomId()}`,
      opportunityId: args.opportunity_id,
      proposedText: args.text,
      targetLabel: opportunity.contactName,
      toolCallId,
    };
    const decision = await options.requestApproval(request, signal);
    const text = approvedText(decision.text);
    const payload: InteractiveAgentApprovalDecisionRequest = {
      approved: decision.approved,
      toolCallId,
      opportunityId: request.opportunityId,
      expectedVersion: request.expectedVersion,
      idempotencyKey: request.idempotencyKey,
      text,
    };
    const serverDecision = await dependencies.decide(
      options.baseUrl,
      options.turnToken,
      payload,
      signal,
    );
    if (!decision.approved) return false;
    if (
      serverDecision.status !== 'granted'
      || !serverDecision.approvalToken
      || !serverDecision.expiresAt
    ) {
      throw new Error('interactive_agent_approval_rejected');
    }
    pending.set(toolCallId, {
      approvalId: serverDecision.id,
      approvalToken: serverDecision.approvalToken,
      payload: {
        opportunityId: request.opportunityId,
        expectedVersion: request.expectedVersion,
        idempotencyKey: request.idempotencyKey,
        text,
      },
    });
    return true;
  };

  const execute = async (toolCallId: string, rawArguments: unknown, signal?: AbortSignal) => {
    sendReplyArguments(rawArguments);
    const approval = pending.get(toolCallId);
    pending.delete(toolCallId);
    if (!approval) throw new Error('interactive_agent_approval_missing');
    const result = await dependencies.execute(
      options.baseUrl,
      approval.approvalToken,
      approval.payload,
      signal,
    );
    return {
      approval_id: approval.approvalId,
      message_id: result.message.id,
      opportunity_id: result.opportunity.id,
      sent: true,
      state: 'sent' as const,
    };
  };

  return { execute, prepare };
}
