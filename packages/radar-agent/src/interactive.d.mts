import type { TSchema } from 'typebox';

export const INTERACTIVE_AGENT_READ_ONLY_SCHEMA_VERSION: 1;
export const INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION: 2;
export const INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION: 3;
export const INTERACTIVE_AGENT_SIGNAL_APPETITE_SCHEMA_VERSION: 4;
export const INTERACTIVE_AGENT_SCHEMA_VERSION: 4;
export const INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION: 'interactive-read-only-v1';
export const INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION: 'interactive-internal-v2';
export const INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION: 'interactive-approved-send-v3';
export const INTERACTIVE_AGENT_SIGNAL_APPETITE_POLICY_VERSION: 'interactive-signal-appetite-v4';

export type InteractiveReadOnlyToolName =
  | 'search_opportunities'
  | 'get_opportunity'
  | 'get_messages';

export type InteractiveInternalToolName =
  | 'draft_reply'
  | 'update_status'
  | 'claim_opportunity';

export type InteractiveExternalToolName = 'send_reply';

export type InteractiveAppetiteToolName =
  | 'inspect_signal_appetite'
  | 'start_teaching_session'
  | 'capture_preference_example'
  | 'summarize_teaching_session'
  | 'propose_appetite_change'
  | 'simulate_appetite'
  | 'apply_appetite_change'
  | 'start_shadow_mode'
  | 'explain_message_decision'
  | 'list_suppressed_samples'
  | 'correct_message_decision'
  | 'create_temporary_focus'
  | 'update_attention_schedule'
  | 'undo_preference_change'
  | 'compare_preference_versions';

export type InteractiveToolName =
  | InteractiveReadOnlyToolName
  | InteractiveInternalToolName
  | InteractiveExternalToolName
  | InteractiveAppetiteToolName;

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
export const InspectSignalAppetiteParameters: TSchema;
export const StartTeachingSessionParameters: TSchema;
export const CapturePreferenceExampleParameters: TSchema;
export const SummarizeTeachingSessionParameters: TSchema;
export const ProposeAppetiteChangeParameters: TSchema;
export const SimulateAppetiteParameters: TSchema;
export const ApplyAppetiteChangeParameters: TSchema;
export const StartShadowModeParameters: TSchema;
export const ExplainMessageDecisionParameters: TSchema;
export const ListSuppressedSamplesParameters: TSchema;
export const CorrectMessageDecisionParameters: TSchema;
export const CreateTemporaryFocusParameters: TSchema;
export const UpdateAttentionScheduleParameters: TSchema;
export const UndoPreferenceChangeParameters: TSchema;
export const ComparePreferenceVersionsParameters: TSchema;
export const INTERACTIVE_READ_ONLY_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_INTERNAL_ACTION_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_INTERNAL_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_EXTERNAL_ACTION_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_APPROVED_SEND_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_SIGNAL_APPETITE_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_SIGNAL_APPETITE_ALL_TOOLS: readonly InteractiveToolDefinition[];
export const INTERACTIVE_READ_ONLY_SYSTEM_PROMPT: string;
export const INTERACTIVE_INTERNAL_SYSTEM_PROMPT: string;
export const INTERACTIVE_APPROVED_SEND_SYSTEM_PROMPT: string;
export const INTERACTIVE_SIGNAL_APPETITE_SYSTEM_PROMPT: string;
export const INTERACTIVE_AGENT_SYSTEM_PROMPT: string;

export interface InteractiveAgentContract {
  schemaVersion: 1 | 2 | 3 | 4;
  policyVersion:
    | 'interactive-read-only-v1'
    | 'interactive-internal-v2'
    | 'interactive-approved-send-v3'
    | 'interactive-signal-appetite-v4';
  systemPrompt: string;
  tools: readonly InteractiveToolDefinition[];
}

export function interactiveAgentContractForSchema(schemaVersion: number): InteractiveAgentContract;
