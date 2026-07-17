import { Agent, type AgentOptions, type AgentTool } from '@earendil-works/pi-agent-core';
import type { AssistantMessage, AssistantMessageEventStream, Model } from '@earendil-works/pi-ai';
import {
  AnalysisSchema,
  type AnalysisResult,
} from '@story2u/radar-agent/analysis';

const expectedResult: AnalysisResult = {
  is_opportunity: true,
  confidence: 0.93,
  title: 'Hermes compatibility result',
  summary: 'Hermes executed the structured submit tool.',
  priority: 'high',
  trust_score: 90,
  attention_required: true,
  link_status: 'unverified',
  link_summary: null,
  risk_flags: [],
  contacts: {
    email: null,
    phone: null,
    telegram_handle: null,
    wecom_id: null,
    extraction_source: null,
  },
  actions: [],
};

function createSubmitTool(onSubmit: (result: AnalysisResult) => void): AgentTool<typeof AnalysisSchema> {
  return {
    name: 'submit_analysis',
    label: 'Submit analysis',
    description: 'Submit the fixed P0 compatibility result.',
    parameters: AnalysisSchema,
    executionMode: 'sequential',
    execute: async (_toolCallId, params) => {
      onSubmit(params as AnalysisResult);
      return {
        content: [{ type: 'text', text: 'Analysis accepted.' }],
        details: {},
        terminate: true,
      };
    },
  };
}

export interface PiHermesSpikeResult {
  callCount: number;
  messageCount: number;
  submitted: AnalysisResult;
}

const model: Model<'radar-fixed'> = {
  id: 'radar-fixed-1',
  name: 'Radar fixed stream',
  api: 'radar-fixed',
  provider: 'radar-hermes-spike',
  baseUrl: 'local://radar-hermes-spike',
  reasoning: false,
  input: ['text'],
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
  contextWindow: 4096,
  maxTokens: 1024,
};

function fixedToolCallStream(): AssistantMessageEventStream {
  const message: AssistantMessage = {
    role: 'assistant',
    content: [
      {
        type: 'toolCall',
        id: 'radar-fixed-tool-call',
        name: 'submit_analysis',
        arguments: expectedResult,
      },
    ],
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
    stopReason: 'toolUse',
    timestamp: Date.now(),
  };
  const response = {
    async *[Symbol.asyncIterator]() {
      yield { type: 'done' as const, reason: 'toolUse' as const, message };
    },
    result: async () => message,
  };
  return response as unknown as AssistantMessageEventStream;
}

/** Uses pi's real Agent/stream/tool pipeline with a deterministic local provider. */
export async function runPiHermesSpike(): Promise<PiHermesSpikeResult> {
  let submitted: AnalysisResult | undefined;
  let callCount = 0;
  const streamFn: NonNullable<AgentOptions['streamFn']> = () => {
    callCount += 1;
    return fixedToolCallStream();
  };
  const agent = new Agent({
    initialState: {
      systemPrompt: 'Call submit_analysis exactly once.',
      model,
      thinkingLevel: 'off',
      tools: [createSubmitTool((result) => {
        submitted = result;
      })],
      messages: [],
    },
    streamFn,
    toolExecution: 'sequential',
  });

  await agent.prompt('Run the deterministic compatibility check.');
  if (agent.state.errorMessage) throw new Error(agent.state.errorMessage);
  if (!submitted) throw new Error('pi did not execute submit_analysis');

  return {
    callCount,
    messageCount: agent.state.messages.length,
    submitted,
  };
}
