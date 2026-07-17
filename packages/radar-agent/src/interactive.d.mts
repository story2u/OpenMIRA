import type { TSchema } from 'typebox';

export const INTERACTIVE_AGENT_READ_ONLY_SCHEMA_VERSION: 1;
export const INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION: 2;
export const INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION: 3;
export const INTERACTIVE_AGENT_SCHEMA_VERSION: 3;
export const INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION: 'interactive-read-only-v1';
export const INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION: 'interactive-internal-v2';
export const INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION: 'interactive-approved-send-v3';

export type InteractiveReadOnlyToolName =
  | 'search_opportunities'
  | 'get_opportunity'
  | 'get_messages';

export type InteractiveInternalToolName =
  | 'draft_reply'
  | 'update_status'
  | 'claim_opportunity';

export type InteractiveExternalToolName = 'send_reply';

export type InteractiveToolName =
  | InteractiveReadOnlyToolName
  | InteractiveInternalToolName
  | InteractiveExternalToolName;

export interface InteractiveToolDefinition {
  name: InteractiveToolName;
  label: string;
  description: string;
  parameters: TSchema;
}

export const SearchOpportunitiesParameters: TSchema;
export const GetOpportunityParameters: TSchema;
export const GetMessagesParameters: TSchema;
export const DraftReplyParameters: TSchema;
export const UpdateStatusParameters: TSchema;
export const ClaimOpportunityParameters: TSchema;
export const SendReplyParameters: TSchema;
export const INTERACTIVE_READ_ONLY_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_INTERNAL_ACTION_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_INTERNAL_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_EXTERNAL_ACTION_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_APPROVED_SEND_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_READ_ONLY_SYSTEM_PROMPT: string;
export const INTERACTIVE_INTERNAL_SYSTEM_PROMPT: string;
export const INTERACTIVE_APPROVED_SEND_SYSTEM_PROMPT: string;
export const INTERACTIVE_AGENT_SYSTEM_PROMPT: string;

export interface InteractiveAgentContract {
  schemaVersion: 1 | 2 | 3;
  policyVersion:
    | 'interactive-read-only-v1'
    | 'interactive-internal-v2'
    | 'interactive-approved-send-v3';
  systemPrompt: string;
  tools: readonly InteractiveToolDefinition[];
}

export function interactiveAgentContractForSchema(schemaVersion: number): InteractiveAgentContract;
