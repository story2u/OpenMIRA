import { Agent, type AgentMessage, type AgentTool } from '@earendil-works/pi-agent-core';
import type { AssistantMessage, Model, ToolResultMessage } from '@earendil-works/pi-ai';
import {
  INTERACTIVE_AGENT_SCHEMA_VERSION,
  INTERACTIVE_APPROVED_SEND_TOOLS,
  INTERACTIVE_EXTERNAL_ACTION_TOOLS,
  INTERACTIVE_INTERNAL_ACTION_TOOLS,
  INTERACTIVE_INTERNAL_TOOLS,
  INTERACTIVE_READ_ONLY_TOOLS,
  INTERACTIVE_SIGNAL_APPETITE_ALL_TOOLS,
  INTERACTIVE_SIGNAL_APPETITE_TOOLS,
  interactiveAgentContractForSchema,
  type InteractiveAgentContract,
  type InteractiveAppetiteToolName,
  type InteractiveExternalToolName,
  type InteractiveInternalToolName,
  type InteractiveReadOnlyToolName,
  type InteractiveToolName,
} from '@story2u/radar-agent/interactive';
import type { InteractiveAgentTurnClaim } from '@story2u/radar-contracts/interactive-agent';

import type { SyncStoreDatabase } from '../../sync/syncStore';
import { createInteractiveGatewayStreamFn, type InteractiveGatewayFetch } from './gatewayStream';
import {
  executeInteractiveInternalTool,
  type InteractiveInternalToolDependencies,
} from './internalTools';
import { executeInteractiveReadOnlyTool } from './readOnlyTools';
import { executeInteractiveAppetiteTool } from './appetiteTools';
import type { AgentEntryContent, LocalAgentEntry } from './sessionStore';
import {
  createInteractiveApprovedSendCoordinator,
  type InteractiveApprovedSendDependencies,
  type RequestInteractiveSendApproval,
} from './approvedSend';

export const INTERACTIVE_AGENT_RUNTIME_VERSION = 'pi-0.80.6';
const maximumContextMessages = 28;
const maximumContextChars = 48_000;
const readOnlyToolNames = new Set<InteractiveReadOnlyToolName>(
  INTERACTIVE_READ_ONLY_TOOLS.map((tool) => tool.name as InteractiveReadOnlyToolName),
);
const internalToolNames = new Set<InteractiveInternalToolName>(
  INTERACTIVE_INTERNAL_ACTION_TOOLS.map((tool) => tool.name as InteractiveInternalToolName),
);
const externalToolNames = new Set<InteractiveExternalToolName>(
  INTERACTIVE_EXTERNAL_ACTION_TOOLS.map((tool) => tool.name as InteractiveExternalToolName),
);
const appetiteToolNames = new Set<InteractiveAppetiteToolName>(
  INTERACTIVE_SIGNAL_APPETITE_TOOLS.map((tool) => tool.name as InteractiveAppetiteToolName),
);
const allToolNames = new Set<InteractiveToolName>(
  INTERACTIVE_SIGNAL_APPETITE_ALL_TOOLS.map((tool) => tool.name),
);

export interface InteractiveAppetiteApprovalRequest {
  preferenceVersion: number;
  toolCallId: string;
}

export type RequestInteractiveAppetiteApproval = (
  request: InteractiveAppetiteApprovalRequest,
  signal?: AbortSignal,
) => Promise<boolean>;

function gatewayModel(baseUrl: string, modelAlias: string): Model<'radar-interactive-gateway'> {
  return {
    id: modelAlias,
    name: 'Radar interactive gateway',
    api: 'radar-interactive-gateway',
    provider: 'radar-interactive-gateway',
    baseUrl,
    reasoning: false,
    input: ['text'],
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    contextWindow: 64_000,
    maxTokens: 4_096,
  };
}

function assistantMessage(
  model: Model<string>,
  content: AssistantMessage['content'],
  timestamp: number,
): AssistantMessage {
  return {
    role: 'assistant',
    content,
    api: model.api,
    provider: model.provider,
    model: model.id,
    usage: {
      input: 0,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason: content.some((item) => item.type === 'toolCall') ? 'toolUse' : 'stop',
    timestamp,
  };
}

/** Rehydrates only the app-defined transcript; raw provider/pi session JSON is never persisted. */
export function localEntriesToAgentMessages(
  entries: LocalAgentEntry[],
  model: Model<string>,
  allowedToolNames: ReadonlySet<InteractiveToolName> = allToolNames,
): AgentMessage[] {
  const messages: AgentMessage[] = [];
  let pendingAssistant: AssistantMessage['content'] = [];
  let pendingTimestamp = 0;
  const flushAssistant = () => {
    if (pendingAssistant.length === 0) return;
    messages.push(assistantMessage(model, pendingAssistant, pendingTimestamp));
    pendingAssistant = [];
    pendingTimestamp = 0;
  };

  for (const entry of entries) {
    const timestamp = Date.parse(entry.createdAt);
    if (entry.content.type === 'user') {
      flushAssistant();
      messages.push({
        role: 'user',
        content: entry.content.text,
        timestamp: Number.isFinite(timestamp) ? timestamp : Date.now(),
      });
      continue;
    }
    if (entry.content.type === 'assistant') {
      pendingTimestamp ||= Number.isFinite(timestamp) ? timestamp : Date.now();
      pendingAssistant.push({ type: 'text', text: entry.content.text });
      continue;
    }
    if (entry.content.type === 'tool_call') {
      pendingTimestamp ||= Number.isFinite(timestamp) ? timestamp : Date.now();
      pendingAssistant.push({
        type: 'toolCall',
        id: entry.content.toolCallId,
        name: entry.content.toolName,
        arguments: entry.content.arguments,
      });
      continue;
    }
    if (entry.content.type === 'tool_result') {
      flushAssistant();
      messages.push({
        role: 'toolResult',
        toolCallId: entry.content.toolCallId,
        toolName: entry.content.toolName,
        content: [{ type: 'text', text: JSON.stringify(entry.content.result) }],
        details: entry.content.result,
        isError: typeof entry.content.result.error === 'string',
        timestamp: Number.isFinite(timestamp) ? timestamp : Date.now(),
      });
    }
  }
  flushAssistant();
  return splitUserTurns(messages)
    .filter((turn) => turn.every((message) => {
      if (message.role === 'toolResult') return allowedToolNames.has(
        message.toolName as InteractiveToolName,
      );
      if (message.role !== 'assistant') return true;
      return message.content.every((item) => (
        item.type !== 'toolCall' || allowedToolNames.has(item.name as InteractiveToolName)
      ));
    }))
    .flat();
}

function messageSize(message: AgentMessage) {
  try {
    return JSON.stringify(message).length;
  } catch {
    return maximumContextChars + 1;
  }
}

function splitUserTurns(messages: AgentMessage[]) {
  const turns: AgentMessage[][] = [];
  for (const message of messages) {
    if (message.role === 'user') turns.push([message]);
    else if (turns.length > 0) turns[turns.length - 1]?.push(message);
  }
  return turns;
}

function currentTurnBlocks(turn: AgentMessage[]) {
  const blocks: AgentMessage[][] = [];
  for (let index = 1; index < turn.length; index += 1) {
    const message = turn[index];
    if (!message) continue;
    if (message.role === 'assistant') {
      const block: AgentMessage[] = [message];
      const callIds = new Set(
        message.content
          .filter((item) => item.type === 'toolCall')
          .map((item) => item.id),
      );
      while (
        callIds.size > 0
        && index + 1 < turn.length
        && turn[index + 1]?.role === 'toolResult'
      ) {
        const result = turn[index + 1] as ToolResultMessage;
        if (!callIds.delete(result.toolCallId)) break;
        block.push(result);
        index += 1;
      }
      if (callIds.size === 0) blocks.push(block);
    }
  }
  return blocks;
}

function shrinkLatestTurn(turn: AgentMessage[]) {
  const user = turn[0];
  if (!user || user.role !== 'user') return [];
  const selected: AgentMessage[] = [user];
  let chars = messageSize(user);
  for (const block of currentTurnBlocks(turn).reverse()) {
    const blockChars = block.reduce((total, message) => total + messageSize(message), 0);
    if (
      selected.length + block.length <= maximumContextMessages
      && chars + blockChars <= maximumContextChars
    ) {
      selected.splice(1, 0, ...block);
      chars += blockChars;
    }
  }
  return selected;
}

/** Keeps complete user turns/tool pairs under the gateway's stricter prompt envelope. */
export function boundedInteractiveContext(messages: AgentMessage[]): AgentMessage[] {
  const turns = splitUserTurns(messages);
  if (turns.length === 0) return [];
  const selected = shrinkLatestTurn(turns.at(-1) ?? []);
  let chars = selected.reduce((total, message) => total + messageSize(message), 0);
  for (let index = turns.length - 2; index >= 0; index -= 1) {
    const turn = turns[index];
    if (!turn) continue;
    const turnChars = turn.reduce((total, message) => total + messageSize(message), 0);
    if (
      selected.length + turn.length > maximumContextMessages
      || chars + turnChars > maximumContextChars
    ) break;
    selected.unshift(...turn);
    chars += turnChars;
  }
  return selected;
}

function createTools(
  baseUrl: string,
  contract: InteractiveAgentContract,
  database: SyncStoreDatabase,
  ownerId: string,
  randomId: () => string,
  executeApprovedSend: (
    toolCallId: string,
    parameters: unknown,
    signal?: AbortSignal,
  ) => Promise<Record<string, unknown>>,
  internalToolDependencies?: InteractiveInternalToolDependencies,
  approvedApplyCalls: ReadonlySet<string> = new Set(),
  deviceId = '',
): AgentTool[] {
  const allowedReadTools = new Set<InteractiveReadOnlyToolName>(
    contract.tools
      .map((tool) => tool.name)
      .filter((name): name is InteractiveReadOnlyToolName => readOnlyToolNames.has(
        name as InteractiveReadOnlyToolName,
      )),
  );
  const allowedInternalTools = new Set<InteractiveInternalToolName>(
    contract.tools
      .map((tool) => tool.name)
      .filter((name): name is InteractiveInternalToolName => internalToolNames.has(
        name as InteractiveInternalToolName,
      )),
  );
  const allowedAppetiteTools = new Set<InteractiveAppetiteToolName>(
    contract.tools
      .map((tool) => tool.name)
      .filter((name): name is InteractiveAppetiteToolName => appetiteToolNames.has(
        name as InteractiveAppetiteToolName,
      )),
  );
  return contract.tools.map((definition) => ({
    ...definition,
    executionMode: 'sequential' as const,
    execute: async (_toolCallId: string, parameters: unknown, signal?: AbortSignal) => {
      if (signal?.aborted) throw new Error('interactive_agent_cancelled');
      const result = readOnlyToolNames.has(definition.name as InteractiveReadOnlyToolName)
        ? await executeInteractiveReadOnlyTool(
          database,
          ownerId,
          allowedReadTools,
          { name: definition.name, arguments: parameters },
        )
        : externalToolNames.has(definition.name as InteractiveExternalToolName)
          ? await executeApprovedSend(_toolCallId, parameters, signal)
          : appetiteToolNames.has(definition.name as InteractiveAppetiteToolName)
            ? await executeInteractiveAppetiteTool(database, {
              allowedTools: allowedAppetiteTools,
              approvedApplyCalls,
              call: { name: definition.name, arguments: parameters, toolCallId: _toolCallId },
              deviceId,
              ownerId,
              randomId,
              signal,
            })
          : await executeInteractiveInternalTool(database, {
            allowedTools: allowedInternalTools,
            baseUrl,
            call: { name: definition.name, arguments: parameters },
            ...(internalToolDependencies ? { dependencies: internalToolDependencies } : {}),
            ownerId,
            randomId,
            signal,
          });
      if (signal?.aborted) throw new Error('interactive_agent_cancelled');
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(result) }],
        details: result,
      };
    },
  })) as AgentTool[];
}

function textFromAssistant(message: AssistantMessage) {
  return message.content
    .filter((item) => item.type === 'text')
    .map((item) => item.text)
    .join('');
}

function stableToolResult(message: ToolResultMessage): Record<string, unknown> {
  if (message.isError) return { error: 'tool_execution_failed' };
  const text = message.content
    .filter((item) => item.type === 'text')
    .map((item) => item.text)
    .join('');
  try {
    const value: unknown = JSON.parse(text);
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      return value as Record<string, unknown>;
    }
  } catch {
    // The gateway transcript remains in memory only; persistence is strict and content-free on errors.
  }
  return { error: 'tool_result_invalid' };
}

export function agentMessagesToLocalEntries(
  messages: AgentMessage[],
  allowedToolNames: ReadonlySet<InteractiveToolName> = allToolNames,
): AgentEntryContent[] {
  const entries: AgentEntryContent[] = [];
  for (const message of messages) {
    if (message.role === 'assistant') {
      const text = textFromAssistant(message);
      if (text) entries.push({ type: 'assistant', text });
      for (const item of message.content) {
        if (item.type !== 'toolCall') continue;
        if (!allowedToolNames.has(item.name as InteractiveToolName)) {
          throw new Error('interactive_agent_tool_invalid');
        }
        entries.push({
          type: 'tool_call',
          toolCallId: item.id,
          toolName: item.name as InteractiveToolName,
          arguments: item.arguments,
        });
      }
      continue;
    }
    if (message.role === 'toolResult') {
      if (!allowedToolNames.has(message.toolName as InteractiveToolName)) {
        throw new Error('interactive_agent_tool_invalid');
      }
      entries.push({
        type: 'tool_result',
        toolCallId: message.toolCallId,
        toolName: message.toolName as InteractiveToolName,
        result: stableToolResult(message),
      });
    }
  }
  return entries;
}

export interface RunInteractiveAgentHostOptions {
  baseUrl: string;
  claim: InteractiveAgentTurnClaim;
  database: SyncStoreDatabase;
  entries: LocalAgentEntry[];
  fetch?: InteractiveGatewayFetch;
  approvedSendDependencies?: InteractiveApprovedSendDependencies;
  internalToolDependencies?: InteractiveInternalToolDependencies;
  onStreamText?(text: string): void;
  ownerId: string;
  randomId(): string;
  requestApproval?: RequestInteractiveSendApproval;
  requestAppetiteApproval?: RequestInteractiveAppetiteApproval;
  signal?: AbortSignal;
}

export interface InteractiveAgentHostResult {
  entries: AgentEntryContent[];
  finalText: string;
}

/** Runs one locally hosted, versioned pi turn through the content-free server gateway. */
export async function runInteractiveAgentHost({
  baseUrl,
  claim,
  database,
  entries,
  fetch,
  approvedSendDependencies,
  internalToolDependencies,
  onStreamText,
  ownerId,
  randomId,
  requestApproval,
  requestAppetiteApproval,
  signal,
}: RunInteractiveAgentHostOptions): Promise<InteractiveAgentHostResult> {
  if (
    claim.runtimeVersion !== INTERACTIVE_AGENT_RUNTIME_VERSION
    || claim.schemaVersion < 1
    || claim.schemaVersion > INTERACTIVE_AGENT_SCHEMA_VERSION
  ) {
    throw new Error('interactive_agent_contract_mismatch');
  }
  let contract: InteractiveAgentContract;
  try {
    contract = interactiveAgentContractForSchema(claim.schemaVersion);
  } catch {
    throw new Error('interactive_agent_contract_mismatch');
  }
  if (claim.policyVersion !== contract.policyVersion) {
    throw new Error('interactive_agent_contract_mismatch');
  }
  const requiresApproval = contract.tools.some((tool) => tool.name === 'send_reply');
  if (requiresApproval && (!requestApproval || !approvedSendDependencies)) {
    throw new Error('interactive_agent_contract_mismatch');
  }
  const approvedSend = requestApproval && approvedSendDependencies
    ? createInteractiveApprovedSendCoordinator({
      baseUrl,
      database,
      dependencies: approvedSendDependencies,
      ownerId,
      randomId,
      requestApproval,
      turnToken: claim.turnToken,
    })
    : null;
  const allowedToolNames = new Set<InteractiveToolName>(
    contract.tools.map((tool) => tool.name),
  );
  const approvedApplyCalls = new Set<string>();
  const model = gatewayModel(baseUrl, claim.modelAlias);
  const initialMessages = boundedInteractiveContext(
    localEntriesToAgentMessages(entries, model, allowedToolNames),
  );
  if (initialMessages.at(-1)?.role !== 'user') {
    throw new Error('interactive_agent_prompt_missing');
  }
  const agent = new Agent({
    initialState: {
      systemPrompt: contract.systemPrompt,
      model,
      thinkingLevel: 'off',
      tools: createTools(
        baseUrl,
        contract,
        database,
        ownerId,
        randomId,
        async (toolCallId, parameters, executionSignal) => {
          if (!approvedSend) throw new Error('interactive_agent_approval_missing');
          return approvedSend.execute(toolCallId, parameters, executionSignal);
        },
        internalToolDependencies,
        approvedApplyCalls,
        claim.deviceId,
      ),
      messages: initialMessages,
    },
    streamFn: createInteractiveGatewayStreamFn({
      baseUrl,
      contract,
      turnToken: claim.turnToken,
      ...(fetch ? { fetch } : {}),
    }),
    transformContext: async (messages) => boundedInteractiveContext(messages),
    toolExecution: 'sequential',
    beforeToolCall: async ({ args, toolCall }, toolSignal) => {
      if (!allowedToolNames.has(toolCall.name as InteractiveToolName)) {
        return { block: true, reason: 'Only tools registered for this session are allowed' };
      }
      if (toolCall.name === 'apply_appetite_change') {
        const preferenceVersion = Number((args as Record<string, unknown>).preference_version);
        if (!requestAppetiteApproval || !Number.isInteger(preferenceVersion)) {
          return { block: true, reason: 'Explicit appetite confirmation is unavailable' };
        }
        const approved = await requestAppetiteApproval({
          preferenceVersion,
          toolCallId: toolCall.id,
        }, toolSignal);
        if (!approved) return { block: true, reason: 'The user did not apply this appetite change' };
        approvedApplyCalls.add(toolCall.id);
        return undefined;
      }
      if (toolCall.name !== 'send_reply') return undefined;
      if (!approvedSend) {
        return { block: true, reason: 'Explicit approval is unavailable' };
      }
      const approved = await approvedSend.prepare(toolCall.id, args, toolSignal);
      return approved
        ? undefined
        : { block: true, reason: 'The user denied this external action' };
    },
  });
  const initialMessageCount = agent.state.messages.length;
  const unsubscribe = agent.subscribe((event) => {
    if (event.type === 'message_update' && event.message.role === 'assistant') {
      onStreamText?.(textFromAssistant(event.message));
    }
  });
  const abort = () => agent.abort();
  if (signal?.aborted) throw new Error('interactive_agent_cancelled');
  signal?.addEventListener('abort', abort, { once: true });
  try {
    await agent.continue();
  } finally {
    unsubscribe();
    signal?.removeEventListener('abort', abort);
  }
  if (signal?.aborted) throw new Error('interactive_agent_cancelled');
  if (agent.state.errorMessage) throw new Error('interactive_agent_request_failed');
  const newMessages = agent.state.messages.slice(initialMessageCount);
  const finalAssistant = [...newMessages]
    .reverse()
    .find((message): message is AssistantMessage => message.role === 'assistant');
  const finalText = finalAssistant ? textFromAssistant(finalAssistant) : '';
  if (!finalText) throw new Error('interactive_agent_result_missing');
  return {
    entries: agentMessagesToLocalEntries(newMessages, allowedToolNames),
    finalText,
  };
}
