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
import { fetch as expoFetch } from 'expo/fetch';

import { EventStream } from '../runtime/piAiCompat';

const gatewayPath = '/api/v1/agent/gateway/v1/chat/completions';
const maxGatewayResponseBytes = 256_000;
const maxToolArgumentsChars = 64_000;

interface GatewayResponse {
  body: ReadableStream<Uint8Array> | null;
  headers: { get(name: string): string | null };
  ok: boolean;
  status: number;
}

export type GatewayFetch = (
  input: string,
  init: RequestInit,
) => Promise<GatewayResponse>;

export interface GatewayStreamOptions {
  baseUrl: string;
  fetch?: GatewayFetch;
  runToken: string;
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

function userMessageText(context: Context) {
  if (context.messages.length !== 1 || context.messages[0]?.role !== 'user') {
    throw new Error('gateway_context_invalid');
  }
  const content = context.messages[0].content;
  if (typeof content === 'string') return content;
  if (content.length === 1 && content[0]?.type === 'text') return content[0].text;
  throw new Error('gateway_context_invalid');
}

function gatewayPayload(
  model: Model<string>,
  context: Context,
  options?: SimpleStreamOptions,
) {
  if (!context.systemPrompt || context.tools?.length !== 1) {
    throw new Error('gateway_context_invalid');
  }
  const tool = context.tools[0];
  if (tool.name !== 'submit_analysis') throw new Error('gateway_tool_invalid');
  const maximumTokens = Math.max(
    1,
    Math.min(options?.maxTokens ?? model.maxTokens, model.maxTokens, 4_096),
  );
  return {
    model: model.id,
    messages: [
      { role: 'system', content: context.systemPrompt },
      { role: 'user', content: userMessageText(context) },
    ],
    stream: true,
    stream_options: { include_usage: true },
    store: false,
    tools: [{
      type: 'function',
      function: {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
        strict: false,
      },
    }],
    tool_choice: {
      type: 'function',
      function: { name: 'submit_analysis' },
    },
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
  const input = finiteTokenCount(usage.prompt_tokens);
  const completion = finiteTokenCount(usage.completion_tokens);
  const total = finiteTokenCount(usage.total_tokens);
  output.usage = {
    ...emptyUsage(),
    input,
    output: completion,
    totalTokens: total,
  };
}

async function* sseData(
  body: ReadableStream<Uint8Array>,
  signal?: AbortSignal,
) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let receivedBytes = 0;
  let completed = false;
  try {
    while (true) {
      if (signal?.aborted) throw new Error('gateway_aborted');
      const { done, value } = await reader.read();
      if (value) {
        receivedBytes += value.byteLength;
        if (receivedBytes > maxGatewayResponseBytes) throw new Error('gateway_response_too_large');
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
    if (buffer.trim()) throw new Error('gateway_stream_truncated');
    completed = true;
  } finally {
    if (!completed || signal?.aborted) await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}

function objectRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('gateway_chunk_invalid');
  }
  return value as Record<string, unknown>;
}

function completionReason(value: unknown) {
  if (value === 'tool_calls' || value === 'function_call') return 'toolUse' as const;
  if (value === 'length') return 'length' as const;
  if (value === 'stop') return 'stop' as const;
  if (value === null || value === undefined) return null;
  throw new Error('gateway_finish_reason_invalid');
}

async function consumeGatewayStream(
  response: GatewayResponse,
  output: AssistantMessage,
  stream: EventStream<AssistantMessageEvent, AssistantMessage>,
  modelAlias: string,
  signal?: AbortSignal,
) {
  if (!response.ok) throw new Error(`gateway_http_${response.status}`);
  if (!response.headers.get('content-type')?.toLowerCase().includes('text/event-stream')) {
    throw new Error('gateway_content_type_invalid');
  }
  if (!response.body) throw new Error('gateway_body_missing');

  stream.push({ type: 'start', partial: output });
  let call: MutableToolCall | null = null;
  let finishReason: 'toolUse' | 'length' | 'stop' | null = null;
  let sawDone = false;

  for await (const data of sseData(response.body, signal)) {
    if (data === '[DONE]') {
      sawDone = true;
      continue;
    }
    const chunk = objectRecord(JSON.parse(data));
    if ('error' in chunk || chunk.model !== modelAlias) throw new Error('gateway_chunk_invalid');
    applyUsage(output, chunk.usage);
    const choices = chunk.choices;
    if (!Array.isArray(choices)) throw new Error('gateway_chunk_invalid');
    if (choices.length === 0) continue;
    const choice = objectRecord(choices[0]);
    const mappedReason = completionReason(choice.finish_reason);
    if (mappedReason) finishReason = mappedReason;
    if (!choice.delta) continue;
    const delta = objectRecord(choice.delta);
    if (typeof delta.content === 'string' && delta.content.length > 0) {
      throw new Error('gateway_prose_not_allowed');
    }
    if (delta.tool_calls === undefined) continue;
    if (!Array.isArray(delta.tool_calls)) throw new Error('gateway_tool_invalid');
    for (const rawToolCall of delta.tool_calls) {
      const toolDelta = objectRecord(rawToolCall);
      if (toolDelta.index !== undefined && toolDelta.index !== 0) {
        throw new Error('gateway_tool_invalid');
      }
      const functionDelta = toolDelta.function === undefined
        ? {}
        : objectRecord(toolDelta.function);
      if (!call) {
        call = {
          type: 'toolCall',
          id: typeof toolDelta.id === 'string' ? toolDelta.id : '',
          name: typeof functionDelta.name === 'string' ? functionDelta.name : '',
          arguments: {},
          partialArguments: '',
        };
        output.content.push(call);
        stream.push({ type: 'toolcall_start', contentIndex: 0, partial: output });
      }
      if (typeof toolDelta.id === 'string') {
        if (call.id && call.id !== toolDelta.id) throw new Error('gateway_tool_invalid');
        call.id = toolDelta.id;
      }
      if (typeof functionDelta.name === 'string') {
        if (call.name && call.name !== functionDelta.name) throw new Error('gateway_tool_invalid');
        call.name = functionDelta.name;
      }
      const argumentDelta = functionDelta.arguments;
      if (argumentDelta !== undefined && typeof argumentDelta !== 'string') {
        throw new Error('gateway_tool_invalid');
      }
      if (typeof argumentDelta === 'string') {
        call.partialArguments += argumentDelta;
        if (call.partialArguments.length > maxToolArgumentsChars) {
          throw new Error('gateway_tool_arguments_too_large');
        }
        stream.push({
          type: 'toolcall_delta',
          contentIndex: 0,
          delta: argumentDelta,
          partial: output,
        });
      }
    }
  }

  if (!sawDone || finishReason !== 'toolUse' || !call?.id || call.name !== 'submit_analysis') {
    throw new Error('gateway_tool_missing');
  }
  const parsedArguments = objectRecord(JSON.parse(call.partialArguments));
  call.arguments = parsedArguments;
  delete (call as Partial<MutableToolCall>).partialArguments;
  output.stopReason = 'toolUse';
  stream.push({
    type: 'toolcall_end',
    contentIndex: 0,
    toolCall: call,
    partial: output,
  });
  stream.push({ type: 'done', reason: 'toolUse', message: output });
}

/**
 * Creates the only provider transport allowed on RN: a run-token-bound stream to
 * the server gateway. Provider keys and real model identifiers never enter the app.
 */
export function createGatewayStreamFn({
  baseUrl,
  fetch = expoFetch as unknown as GatewayFetch,
  runToken,
}: GatewayStreamOptions) {
  if (runToken.length < 16 || runToken.length > 16_384) {
    throw new Error('invalid analysis run token');
  }
  return (
    model: Model<string>,
    context: Context,
    options?: SimpleStreamOptions,
  ): AssistantMessageEventStream => {
    const stream = createEventStream();
    const output = createOutput(model);
    void (async () => {
      try {
        const payload = gatewayPayload(model, context, options);
        const response = await fetch(`${baseUrl.replace(/\/+$/, '')}${gatewayPath}`, {
          method: 'POST',
          headers: {
            Accept: 'text/event-stream',
            Authorization: `Bearer ${runToken}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(payload),
          signal: options?.signal,
        });
        await consumeGatewayStream(response, output, stream, model.id, options?.signal);
      } catch {
        output.content = [];
        output.stopReason = options?.signal?.aborted ? 'aborted' : 'error';
        output.errorMessage = options?.signal?.aborted
          ? 'analysis_cancelled'
          : 'analysis_gateway_failed';
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
