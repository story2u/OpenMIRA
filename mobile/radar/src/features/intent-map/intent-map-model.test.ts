import { describe, expect, it } from 'vitest';

import type { AttentionIntent } from '@story2u/radar-core/attention/model';
import { buildIntentMapModel, type IntentMapSnapshot } from './intent-map-model';

function intent(index: number, intentType: AttentionIntent['intentType'] = 'include'): AttentionIntent {
  return {
    id: `01234567-89ab-4def-8123-${index.toString().padStart(12, '0')}`,
    preferenceId: '11234567-89ab-cdef-0123-456789abcdef',
    concept: `intent_${index}`,
    intentType,
    weight: intentType === 'reduce' ? -0.7 : 0.7,
    deliveryMode: intentType === 'reduce' ? 'suppress' : 'inbox',
    confidence: 0.8,
    userConfirmed: index % 2 === 0,
    source: 'teaching', validFrom: null, validUntil: null,
  };
}

const snapshot: IntentMapSnapshot = {
  preference: null,
  intents: Array.from({ length: 35 }, (_, index) => intent(index, index > 27 ? 'reduce' : 'include')),
  temporaryFocuses: [], decisions: [], shadow: null,
};

describe('intent map model', () => {
  it('produces a deterministic, bounded layout with a fixed self node', () => {
    const first = buildIntentMapModel(snapshot);
    const second = buildIntentMapModel({ ...snapshot, intents: [...snapshot.intents].reverse() });
    expect(first.nodes).toEqual(second.nodes);
    expect(first.nodes.length).toBeLessThanOrEqual(30);
    expect(first.nodes.length).toBeGreaterThan(10);
    expect(first.nodes[0]).toMatchObject({ id: 'self', x: 180, y: 164 });
  });

  it('places reduce intents at the quiet edge and marks inferred edges as dashed data', () => {
    const model = buildIntentMapModel({ ...snapshot, intents: [intent(1, 'reduce'), intent(2)] });
    expect(model.nodes.find((node) => node.kind === 'reduce')?.y).toBeGreaterThan(280);
    expect(model.edges.find((edge) => edge.to.endsWith('000000000001'))?.confirmed).toBe(false);
  });
});
