import { describe, expect, it } from 'vitest';

import { evaluateMessage, simulateAppetite, type AppetiteMessage } from './evaluator';
import type { AttentionIntent } from './model';

const intent: AttentionIntent = {
  id: 'intent-ai-jobs',
  preferenceId: 'preference',
  concept: 'ai_jobs',
  intentType: 'include',
  weight: 0.9,
  deliveryMode: 'immediate',
  confidence: 0.9,
  userConfirmed: true,
  source: 'conversation',
  validFrom: null,
  validUntil: null,
};

function message(overrides: Partial<AppetiteMessage> = {}): AppetiteMessage {
  return {
    id: 'message-1',
    sourceKey: 'group-ai',
    sourceTrusted: true,
    senderImportant: false,
    duplicate: false,
    explicitAdvertisingSource: false,
    hardExcluded: false,
    needsReply: false,
    topicKeys: [],
    semanticScores: {},
    sentAt: '2026-07-18T12:00:00.000Z',
    ...overrides,
  };
}

function evaluate(candidate: AppetiteMessage, cloudAvailable = false) {
  return evaluateMessage({
    message: candidate,
    preferenceVersion: 2,
    intents: [intent],
    schedule: [],
    temporaryFocuses: [],
    cloudAvailable,
    now: '2026-07-18T12:00:00.000Z',
  });
}

describe('three-level appetite evaluation', () => {
  it('uses deterministic safety signals before semantic evaluation', () => {
    expect(evaluate(message({ needsReply: true }))).toMatchObject({
      decision: 'immediate', evaluator: 'deterministic',
    });
    expect(evaluate(message({ duplicate: true }))).toMatchObject({
      decision: 'suppress', evaluator: 'deterministic',
    });
  });

  it('never suppresses a semantic boundary merely because cloud analysis is unavailable', () => {
    expect(evaluate(message({ semanticScores: { ai_jobs: 0.55 } }), false)).toMatchObject({
      decision: 'inbox', evaluator: 'on_device_model',
    });
  });

  it('returns bounded before/after samples and delivery statistics', () => {
    const messages = Array.from({ length: 12 }, (_, index) => message({
      id: `message-${index}`,
      sourceKey: index < 7 ? 'group-a' : 'group-b',
      semanticScores: { ai_jobs: index % 2 === 0 ? 0.9 : 0.1 },
    }));
    const previous = new Map(messages.map((item) => [item.id, {
      ...evaluate(item), decision: 'digest' as const,
    }]));
    const result = simulateAppetite(messages, (item) => evaluate(item), previous);

    expect(result.originalCount).toBe(12);
    expect(result.immediateCount + result.inboxCount + result.digestCount + result.suppressCount).toBe(12);
    expect(result.newlyRetainedMessageIds.length).toBeLessThanOrEqual(5);
    expect(result.largestChangeSources[0]).toBe('group-a');
  });
});
