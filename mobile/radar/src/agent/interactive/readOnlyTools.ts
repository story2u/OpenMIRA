import {
  GetMessagesParameters,
  GetOpportunityParameters,
  INTERACTIVE_READ_ONLY_TOOLS,
  SearchOpportunitiesParameters,
  type InteractiveReadOnlyToolName,
} from '@story2u/radar-agent/interactive';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { TSchema } from 'typebox';
import { Value } from 'typebox/value';

import {
  LocalProjectionCorruptError,
  OfflineProjectionUnavailableError,
  readOfflineMessagePage,
  readOfflineOpportunityDetail,
  searchOfflineOpportunities,
} from '../../sync/offlineRepository';
import type { SyncStoreExecutor } from '../../sync/syncStore';

// One provider turn may request four tools. Keeping each result below 10 KB leaves
// room for the user prompt and tool metadata inside the 64 KB gateway envelope.
const maximumToolResultBytes = 10_000;
const toolSchemas: Readonly<Record<InteractiveReadOnlyToolName, TSchema>> = Object.freeze({
  search_opportunities: SearchOpportunitiesParameters,
  get_opportunity: GetOpportunityParameters,
  get_messages: GetMessagesParameters,
});
const knownToolNames = new Set(INTERACTIVE_READ_ONLY_TOOLS.map((tool) => tool.name));

export interface InteractiveReadToolCall {
  name: string;
  arguments: unknown;
}

export class InteractiveReadToolError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'InteractiveReadToolError';
  }
}

function compactOpportunity(opportunity: OpportunityDetail) {
  return {
    id: opportunity.id,
    platform: opportunity.platform,
    contactName: opportunity.contactName.slice(0, 160),
    summary: opportunity.summary.slice(0, 1_000),
    status: opportunity.status,
    internalStatus: opportunity.internalStatus,
    priority: opportunity.priority,
    sourceType: opportunity.sourceType,
    groupName: opportunity.groupName?.slice(0, 160) ?? null,
    lastMessagePreview: opportunity.lastMessagePreview.slice(0, 1_000),
    confidenceScore: opportunity.confidenceScore,
    trustScore: opportunity.trustScore,
    sopStage: opportunity.sopStage,
    attentionRequired: opportunity.attentionRequired,
    archivedAt: opportunity.archivedAt,
    updatedAt: opportunity.updatedAt,
  };
}

function boundedResult(result: Record<string, unknown>) {
  const serialized = JSON.stringify(result);
  if (new TextEncoder().encode(serialized).byteLength > maximumToolResultBytes) {
    throw new InteractiveReadToolError('tool_result_too_large');
  }
  return result;
}

export async function executeInteractiveReadOnlyTool(
  database: SyncStoreExecutor,
  ownerId: string,
  allowedTools: ReadonlySet<InteractiveReadOnlyToolName>,
  call: InteractiveReadToolCall,
): Promise<Record<string, unknown>> {
  if (!knownToolNames.has(call.name as InteractiveReadOnlyToolName)) {
    throw new InteractiveReadToolError('unknown_tool');
  }
  const toolName = call.name as InteractiveReadOnlyToolName;
  if (!allowedTools.has(toolName)) {
    throw new InteractiveReadToolError('tool_not_authorized');
  }
  if (!Value.Check(toolSchemas[toolName], call.arguments)) {
    throw new InteractiveReadToolError('invalid_tool_arguments');
  }

  try {
    if (toolName === 'search_opportunities') {
      const args = call.arguments as { query: string; limit?: number };
      const matches = await searchOfflineOpportunities(
        database,
        ownerId,
        args.query,
        args.limit ?? 10,
      );
      return boundedResult({
        matches: matches.map(compactOpportunity),
        count: matches.length,
      });
    }
    if (toolName === 'get_opportunity') {
      const args = call.arguments as { opportunity_id: string };
      const opportunity = await readOfflineOpportunityDetail(
        database,
        ownerId,
        args.opportunity_id,
      );
      return boundedResult({
        opportunity: opportunity ? compactOpportunity(opportunity) : null,
      });
    }
    const args = call.arguments as {
      opportunity_id: string;
      limit?: number;
      offset?: number;
    };
    const page = await readOfflineMessagePage(database, ownerId, args.opportunity_id, {
      limit: args.limit ?? 20,
      offset: args.offset ?? 0,
    });
    return boundedResult({
      messages: page.items.map((message) => ({
        id: message.id,
        senderName: message.senderName.slice(0, 160),
        content: message.content.slice(0, 2_000),
        isFromContact: message.isFromContact,
        sentAt: message.sentAt,
        source: message.source,
      })),
      total: page.total,
      limit: page.limit,
      offset: page.offset,
      hasMore: page.offset + page.items.length < page.total,
    });
  } catch (error) {
    if (error instanceof LocalProjectionCorruptError) {
      throw new InteractiveReadToolError('local_projection_corrupt');
    }
    if (error instanceof OfflineProjectionUnavailableError) {
      throw new InteractiveReadToolError('local_projection_unavailable');
    }
    throw error;
  }
}
