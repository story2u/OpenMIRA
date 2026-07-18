import { describe, expect, it } from 'vitest';

import type { SignalAppetiteEvent } from './events';
import { foldSignalAppetite } from './fold';
import type { AttentionPreference, PreferenceExample } from './model';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const deviceId = '11234567-89ab-cdef-0123-456789abcdef';
const sessionId = '21234567-89ab-cdef-0123-456789abcdef';
const preferenceId = '31234567-89ab-cdef-0123-456789abcdef';

function event<Type extends SignalAppetiteEvent['type']>(
  type: Type,
  sequence: number,
  payload: Extract<SignalAppetiteEvent, { type: Type }>['payload'],
): Extract<SignalAppetiteEvent, { type: Type }> {
  return {
    eventId: `41234567-89ab-4def-8123-${sequence.toString().padStart(12, '0')}`,
    ownerId,
    deviceId,
    sequence,
    aggregateId: sessionId,
    aggregateVersion: sequence,
    schemaVersion: 1,
    occurredAt: `2026-07-18T10:00:${sequence.toString().padStart(2, '0')}.000Z`,
    type,
    payload,
  } as Extract<SignalAppetiteEvent, { type: Type }>;
}

function example(label: PreferenceExample['label']): PreferenceExample {
  return {
    id: '51234567-89ab-cdef-0123-456789abcdef',
    messageId: '61234567-89ab-cdef-0123-456789abcdef',
    label,
    selectedReasons: ['needs_reply'],
    freeformReason: null,
    capturedAt: '2026-07-18T10:00:03.000Z',
    teachingSessionId: sessionId,
    revertedAt: null,
  };
}

function preference(version: number): AttentionPreference {
  return {
    id: preferenceId,
    title: 'Current appetite',
    naturalLanguageSummary: 'Keep messages that need a reply.',
    scope: 'all_messages',
    status: 'candidate',
    confidence: 0.8,
    version,
    activeFrom: null,
    activeUntil: null,
    schedule: [],
    createdAt: '2026-07-18T10:00:00.000Z',
    updatedAt: '2026-07-18T10:00:00.000Z',
  };
}

describe('foldSignalAppetite', () => {
  it('captures and reverts examples without applying a permanent preference', () => {
    const started = event('TeachingSessionStarted', 1, { sessionId, targetCount: 8 });
    const captured = event('PreferenceExampleCaptured', 2, { example: example('positive') });
    const reverted = event('PreferenceExampleReverted', 3, {
      exampleId: captured.payload.example.id,
      revertedAt: '2026-07-18T10:00:04.000Z',
    });
    const state = foldSignalAppetite([reverted, captured, started, captured]);

    expect(state.activePreferenceVersion).toBeNull();
    expect(state.examples.get(captured.payload.example.id)?.revertedAt).not.toBeNull();
    expect(state.sessions.get(sessionId)).toMatchObject({ positiveCount: 0, status: 'active' });
    expect(state.appliedEventIds.size).toBe(3);
  });

  it('updates reasons on a repeated captured example without double-counting it', () => {
    const started = event('TeachingSessionStarted', 1, { sessionId, targetCount: 8 });
    const original = example('positive');
    const captured = event('PreferenceExampleCaptured', 2, { example: original });
    const annotated = event('PreferenceExampleCaptured', 3, {
      example: { ...original, selectedReasons: ['needs_reply', 'deadline'] },
    });

    const state = foldSignalAppetite([annotated, captured, started]);

    expect(state.sessions.get(sessionId)?.positiveCount).toBe(1);
    expect(state.examples.get(original.id)?.selectedReasons).toEqual(['needs_reply', 'deadline']);
  });

  it('requires a proposed version before apply and supports auditable rollback', () => {
    const ignoredApply = event('PreferenceApplied', 1, {
      preferenceId,
      version: 2,
      appliedAt: '2026-07-18T10:00:01.000Z',
    });
    const proposedV1 = event('PreferenceChangeProposed', 2, {
      preference: preference(1),
      intents: [],
      teachingSessionId: null,
    });
    const appliedV1 = event('PreferenceApplied', 3, {
      preferenceId,
      version: 1,
      appliedAt: '2026-07-18T10:00:03.000Z',
    });
    const proposedV2 = event('PreferenceChangeProposed', 4, {
      preference: preference(2),
      intents: [],
      teachingSessionId: null,
    });
    const appliedV2 = event('PreferenceApplied', 5, {
      preferenceId,
      version: 2,
      appliedAt: '2026-07-18T10:00:05.000Z',
    });
    const reverted = event('PreferenceReverted', 6, {
      preferenceId,
      fromVersion: 2,
      toVersion: 1,
      revertedAt: '2026-07-18T10:00:06.000Z',
    });
    const state = foldSignalAppetite([
      ignoredApply,
      proposedV1,
      appliedV1,
      proposedV2,
      appliedV2,
      reverted,
    ]);

    expect(state.activePreferenceVersion).toBe(1);
    expect(state.preferences.get(1)?.status).toBe('active');
    expect(state.preferences.get(2)?.status).toBe('reverted');
  });
});
