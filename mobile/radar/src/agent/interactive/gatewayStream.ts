import type {
  AssistantMessage,
  AssistantMessageEvent,
  AssistantMessageEventStream,
  Context,
  Model,
  SimpleStreamOptions,
  ToolCall,
  Usage,
} from '@earendil-works/pi-ai';
import type { InteractiveAgentContract } from '@story2u/radar-agent/interactive';
import { fetch as expoFetch } from 'expo/fetch';

import { EventStream } from '../../runtime/piAiCompat';

const gatewayPath = '/api/v1/agent/interactive/gateway/v1/chat/completions';
const maximumGatewayResponseBytes = 1_000_000;
const maximumToolArgumentsChars = 65_536;

interface GatewayResponse {
  body: ReadableStream<Uint8Array> | null;
  headers: { get(name: string): string | null };
  ok: boolean;
  status: number;
}

export type InteractiveGatewayFetch = (
  input: string,
  init: RequestInit,
) => Promise<GatewayResponse>;

export interface InteractiveGatewayStreamOptions {
  baseUrl: string;
  contract: InteractiveAgentContract;
  fetch?: InteractiveGatewayFetch;
  turnToken: string;
}

interface MutableToolCall extends ToolCall {
  partialArguments: string;
}

function emptyUsage(): Usage {
  return {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
    totalTokens: 0,
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
  };
}

function createOutput(model: Model<string>): AssistantMessage {
  return {
    role: 'assistant',
    content: [],
    api: model.api,
    provider: model.provider,
    model: model.id,
    usage: emptyUsage(),
    stopReason: 'stop',
    timestamp: Date.now(),
  };
}

function createEventStream() {
  return new EventStream<AssistantMessageEvent, AssistantMessage>(
    (event) => event.type === 'done' || event.type === 'error',
    (event) => {
      if (event.type === 'done') return event.message;
      if (event.type === 'error') return event.error;
      throw new Error('assistant stream ended without a result');
    },
  );
}

function textContent(content: unknown) {
  if (typeof content === 'string') return content;
  if (!Array.isArray(content)) throw new Error('interactive_gateway_context_invalid');
  const text = content.map((item) => {
    if (!item || typeof item !== 'object' || item.type !== 'text' || typeof item.text !== 'string') {
      throw new Error('interactive_gateway_context_invalid');
    }
    return item.text;
  }).join('');
  if (!text) throw new Error('interactive_gateway_context_invalid');
  return text;
}

function gatewayMessages(
  context: Context,
  systemPrompt: string,
  toolNames: ReadonlySet<string>,
) {
  if (context.messages.length < 1 || context.messages.length > 31) {
    throw new Error('interactive_gateway_context_invalid');
  }
  return [
    { role: 'system', content: systemPrompt },
    ...context.messages.map((message) => {
      if (message.role === 'user') {
        return { role: 'user', content: textContent(message.content) };
      }
      if (message.role === 'toolResult') {
        if (!toolNames.has(message.toolName)) {
          throw new Error('interactive_gateway_tool_invalid');
        }
        return {
          role: 'tool',
          tool_call_id: message.toolCallId,
          content: textContent(message.content),
        };
      }
      const texts: string[] = [];
      const toolCalls: Array<{
        id: string;
        type: 'function';
        function: { name: string; arguments: string };
      }> = [];
      for (const item of message.content) {
        if (item.type === 'text') {
          texts.push(item.text);
          continue;
        }
        if (item.type !== 'toolCall' || !toolNames.has(item.name)) {
          throw new Error('interactive_gateway_context_invalid');
        }
        toolCalls.push({
          id: item.id,
          type: 'function',
          function: { name: item.name, arguments: JSON.stringify(item.arguments) },
        });
      }
      if (texts.length === 0 && toolCalls.length === 0) {
        throw new Error('interactive_gateway_context_invalid');
      }
      return {
        role: 'assistant',
        content: texts.length > 0 ? texts.join('') : null,
        ...(toolCalls.length > 0 ? { tool_calls: toolCalls } : {}),
      };
    }),
  ];
}

function gatewayPayload(
  model: Model<string>,
  context: Context,
  contract: InteractiveAgentContract,
  options?: SimpleStreamOptions,
) {
  if (context.systemPrompt !== contract.systemPrompt) {
    throw new Error('interactive_gateway_system_prompt_invalid');
  }
  if (
    context.tools?.length !== contract.tools.length
    || context.tools.some((tool, index) => tool.name !== contract.tools[index]?.name)
  ) {
    throw new Error('interactive_gateway_tool_invalid');
  }
  const toolNames = new Set<string>(contract.tools.map((tool) => tool.name));
  const maximumTokens = Math.max(
    1,
    Math.min(options?.maxTokens ?? model.maxTokens, model.maxTokens, 4_096),
  );
  return {
    model: model.id,
    messages: gatewayMessages(context, contract.systemPrompt, toolNames),
    stream: true,
    stream_options: { include_usage: true },
    store: false,
    tools: contract.tools.map((tool) => ({
      type: 'function',
      function: {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
        strict: false,
      },
    })),
    tool_choice: 'auto',
    parallel_tool_calls: false,
    max_completion_tokens: maximumTokens,
    ...(options?.temperature === undefined ? {} : { temperature: options.temperature }),
  };
}

function finiteTokenCount(value: unknown) {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
    ? Math.min(value, 2_147_483_647)
    : 0;
}

function applyUsage(output: AssistantMessage, value: unknown) {
  if (!value || typeof value !== 'object') return;
  const usage = value as Record<string, unknown>;
  output.usage = {
    ...emptyUsage(),
    input: finiteTokenCount(usage.prompt_tokens),
    output: finiteTokenCount(usage.completion_tokens),
    totalTokens: finiteTokenCount(usage.total_tokens),
  };
}

async function* sseData(body: ReadableStream<Uint8Array>, signal?: AbortSignal) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let receivedBytes = 0;
  let completed = false;
  try {
    while (true) {
      if (signal?.aborted) throw new Error('interactive_gateway_aborted');
      const { done, value } = await reader.read();
      if (value) {
        receivedBytes += value.byteLength;
        if (receivedBytes > maximumGatewayResponseBytes) {
          throw new Error('interactive_gateway_response_too_large');
        }
        buffer += decoder.decode(value, { stream: !done });
      } else if (done) {
        buffer += decoder.decode();
      }
      const blocks = buffer.split(/\r?\n\r?\n/);
      buffer = blocks.pop() ?? '';
      for (const block of blocks) {
        const data = block
          .split(/\r?\n/)
          .filter((line) => line.startsWith('data:'))
          .map((line) => line.slice(5).trimStart())
          .join('\n');
        if (data) yield data;
      }
      if (done) break;
    }
    if (buffer.trim()) throw new Error('interactive_gateway_stream_truncated');
    completed = true;
  } finally {
    if (!completed || signal?.aborted) await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('interactive_gateway_chunk_invalid');
  }
  return value as Record<string, unknown>;
}

function completionReason(value: unknown) {
  if (value === 'tool_calls' || value === 'function_call') return 'toolUse' as const;
  if (value === 'length') return 'length' as const;
  if (value === 'stop') return 'stop' as const;
  if (value === null || value === undefined) return null;
  throw new Error('interactive_gateway_finish_reason_invalid');
}

async function consumeGatewayStream(
  response: GatewayResponse,
  output: AssistantMessage,
  stream: EventStream<AssistantMessageEvent, AssistantMessage>,
  modelAlias: string,
  toolNames: ReadonlySet<string>,
  signal?: AbortSignal,
) {
  if (!response.ok) throw new Error(`interactive_gateway_http_${response.status}`);
  if (!response.headers.get('content-type')?.toLowerCase().includes('text/event-stream')) {
    throw new Error('interactive_gateway_content_type_invalid');
  }
  if (!response.body) throw new Error('interactive_gateway_body_missing');

  stream.push({ type: 'start', partial: output });
  let textIndex: number | null = null;
  let textValue = '';
  const calls = new Map<number, { call: MutableToolCall; contentIndex: number }>();
  let finishReason: 'toolUse' | 'length' | 'stop' | null = null;
  let sawDone = false;

  for await (const data of sseData(response.body, signal)) {
    if (data === '[DONE]') {
      if (sawDone) throw new Error('interactive_gateway_chunk_invalid');
      sawDone = true;
      continue;
    }
    if (sawDone) throw new Error('interactive_gateway_chunk_invalid');
    const chunk = record(JSON.parse(data));
    if ('error' in chunk || chunk.model !== modelAlias) {
      throw new Error('interactive_gateway_chunk_invalid');
    }
    applyUsage(output, chunk.usage);
    if (!Array.isArray(chunk.choices)) throw new Error('interactive_gateway_chunk_invalid');
    if (chunk.choices.length === 0) continue;
    const choice = record(chunk.choices[0]);
    const mappedReason = completionReason(choice.finish_reason);
    if (mappedReason) finishReason = mappedReason;
    if (!choice.delta) continue;
    const delta = record(choice.delta);
    if (typeof delta.content === 'string' && delta.content.length > 0) {
      if (textIndex === null) {
        textIndex = output.content.length;
        output.content.push({ type: 'text', text: '' });
        stream.push({ type: 'text_start', contentIndex: textIndex, partial: output });
      }
      textValue += delta.content;
      const block = output.content[textIndex];
      if (!block || block.type !== 'text') throw new Error('interactive_gateway_chunk_invalid');
      block.text = textValue;
      stream.push({
        type: 'text_delta',
        contentIndex: textIndex,
        delta: delta.content,
        partial: output,
      });
    } else if (delta.content !== undefined && delta.content !== null) {
      throw new Error('interactive_gateway_chunk_invalid');
    }
    if (delta.tool_calls === undefined) continue;
    if (!Array.isArray(delta.tool_calls)) throw new Error('interactive_gateway_tool_invalid');
    for (const rawToolCall of delta.tool_calls) {
      const toolDelta = record(rawToolCall);
      const index = toolDelta.index;
      if (
        typeof index !== 'number'
        || !Number.isInteger(index)
        || index < 0
        || index > 3
      ) {
        throw new Error('interactive_gateway_tool_invalid');
      }
      const functionDelta = toolDelta.function === undefined
        ? {}
        : record(toolDelta.function);
      let current = calls.get(index);
      if (!current) {
        const call: MutableToolCall = {
          type: 'toolCall',
          id: typeof toolDelta.id === 'string' ? toolDelta.id : '',
          name: typeof functionDelta.name === 'string' ? functionDelta.name : '',
          arguments: {},
          partialArguments: '',
        };
        current = { call, contentIndex: output.content.length };
        calls.set(index, current);
        output.content.push(call);
        stream.push({
          type: 'toolcall_start',
          contentIndex: current.contentIndex,
          partial: output,
        });
      }
      if (typeof toolDelta.id === 'string') {
        if (current.call.id && current.call.id !== toolDelta.id) {
          throw new Error('interactive_gateway_tool_invalid');
        }
        current.call.id = toolDelta.id;
      }
      if (typeof functionDelta.name === 'string') {
        if (
          !toolNames.has(functionDelta.name)
          || (current.call.name && current.call.name !== functionDelta.name)
        ) {
          throw new Error('interactive_gateway_tool_invalid');
        }
        current.call.name = functionDelta.name;
      }
      if (functionDelta.arguments !== undefined) {
        if (typeof functionDelta.arguments !== 'string') {
          throw new Error('interactive_gateway_tool_invalid');
        }
        current.call.partialArguments += functionDelta.arguments;
        if (current.call.partialArguments.length > maximumToolArgumentsChars) {
          throw new Error('interactive_gateway_tool_arguments_too_large');
        }
        stream.push({
          type: 'toolcall_delta',
          contentIndex: current.contentIndex,
          delta: functionDelta.arguments,
          partial: output,
        });
      }
    }
  }

  if (!sawDone || !finishReason) throw new Error('interactive_gateway_incomplete');
  if (textIndex !== null) {
    stream.push({
      type: 'text_end',
      contentIndex: textIndex,
      content: textValue,
      partial: output,
    });
  }
  for (const index of [...calls.keys()].sort((left, right) => left - right)) {
    const current = calls.get(index);
    if (
      !current?.call.id
      || !toolNames.has(current.call.name)
      || finishReason !== 'toolUse'
    ) {
      throw new Error('interactive_gateway_tool_invalid');
    }
    current.call.arguments = record(JSON.parse(current.call.partialArguments));
    delete (current.call as Partial<MutableToolCall>).partialArguments;
    stream.push({
      type: 'toolcall_end',
      contentIndex: current.contentIndex,
      toolCall: current.call,
      partial: output,
    });
  }
  if (finishReason === 'toolUse' && calls.size === 0) {
    throw new Error('interactive_gateway_tool_missing');
  }
  output.stopReason = finishReason;
  stream.push({ type: 'done', reason: finishReason, message: output });
}

export function createInteractiveGatewayStreamFn({
  baseUrl,
  contract,
  fetch = expoFetch as unknown as InteractiveGatewayFetch,
  turnToken,
}: InteractiveGatewayStreamOptions) {
  if (turnToken.length < 16 || turnToken.length > 16_384) {
    throw new Error('invalid interactive Agent turn token');
  }
  const toolNames = new Set<string>(contract.tools.map((tool) => tool.name));
  return (
    model: Model<string>,
    context: Context,
    options?: SimpleStreamOptions,
  ): AssistantMessageEventStream => {
    const stream = createEventStream();
    const output = createOutput(model);
    void (async () => {
      try {
        const payload = gatewayPayload(model, context, contract, options);
        const response = await fetch(`${baseUrl.replace(/\/+$/, '')}${gatewayPath}`, {
          method: 'POST',
          headers: {
            Accept: 'text/event-stream',
            Authorization: `Bearer ${turnToken}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(payload),
          signal: options?.signal,
        });
        await consumeGatewayStream(
          response,
          output,
          stream,
          model.id,
          toolNames,
          options?.signal,
        );
      } catch {
        output.content = [];
        output.stopReason = options?.signal?.aborted ? 'aborted' : 'error';
        output.errorMessage = options?.signal?.aborted
          ? 'interactive_agent_cancelled'
          : 'interactive_agent_gateway_failed';
        stream.push({
          type: 'error',
          reason: output.stopReason,
          error: output,
        });
      } finally {
        stream.end();
      }
    })();
    return stream as unknown as AssistantMessageEventStream;
  };
}
