import type { SignalAppetiteEvent } from './events';
import type {
  AppetiteSimulationSummary,
  AttentionIntent,
  AttentionPreference,
  MessageFilterDecision,
  PreferenceExample,
  ShadowEvaluation,
  TeachingSession,
  TemporaryFocus,
} from './model';

export interface SignalAppetiteState {
  activePreferenceVersion: number | null;
  preferences: ReadonlyMap<number, AttentionPreference>;
  intentsByPreferenceVersion: ReadonlyMap<number, readonly AttentionIntent[]>;
  examples: ReadonlyMap<string, PreferenceExample>;
  sessions: ReadonlyMap<string, TeachingSession>;
  decisions: ReadonlyMap<string, MessageFilterDecision>;
  simulations: ReadonlyMap<number, AppetiteSimulationSummary>;
  shadows: ReadonlyMap<string, ShadowEvaluation>;
  temporaryFocuses: ReadonlyMap<string, TemporaryFocus>;
  appliedEventIds: ReadonlySet<string>;
}

function emptyState(): SignalAppetiteState {
  return {
    activePreferenceVersion: null,
    preferences: new Map(),
    intentsByPreferenceVersion: new Map(),
    examples: new Map(),
    sessions: new Map(),
    decisions: new Map(),
    simulations: new Map(),
    shadows: new Map(),
    temporaryFocuses: new Map(),
    appliedEventIds: new Set(),
  };
}

function compareEvents(left: SignalAppetiteEvent, right: SignalAppetiteEvent) {
  return left.sequence - right.sequence || left.eventId.localeCompare(right.eventId);
}

export function foldSignalAppetite(events: readonly SignalAppetiteEvent[]): SignalAppetiteState {
  const state = emptyState();
  const preferences = state.preferences as Map<number, AttentionPreference>;
  const intents = state.intentsByPreferenceVersion as Map<number, readonly AttentionIntent[]>;
  const examples = state.examples as Map<string, PreferenceExample>;
  const sessions = state.sessions as Map<string, TeachingSession>;
  const decisions = state.decisions as Map<string, MessageFilterDecision>;
  const simulations = state.simulations as Map<number, AppetiteSimulationSummary>;
  const shadows = state.shadows as Map<string, ShadowEvaluation>;
  const temporaryFocuses = state.temporaryFocuses as Map<string, TemporaryFocus>;
  const appliedEventIds = state.appliedEventIds as Set<string>;

  for (const event of [...events].sort(compareEvents)) {
    if (appliedEventIds.has(event.eventId)) continue;
    appliedEventIds.add(event.eventId);

    switch (event.type) {
      case 'TeachingSessionStarted':
        sessions.set(event.payload.sessionId, {
          id: event.payload.sessionId,
          startedAt: event.occurredAt,
          completedAt: null,
          presentedCount: 0,
          positiveCount: 0,
          negativeCount: 0,
          skippedCount: 0,
          status: 'active',
          proposedPreferenceVersion: null,
          summary: null,
        });
        break;
      case 'TeachingCardPresented': {
        const session = sessions.get(event.payload.sessionId);
        if (session) sessions.set(session.id, { ...session, presentedCount: session.presentedCount + 1 });
        break;
      }
      case 'PreferenceExampleCaptured': {
        const previous = examples.get(event.payload.example.id);
        examples.set(event.payload.example.id, event.payload.example);
        if (!previous || previous.revertedAt) {
          const session = sessions.get(event.payload.example.teachingSessionId);
          if (session) {
            const label = event.payload.example.label;
            sessions.set(session.id, {
              ...session,
              positiveCount: session.positiveCount + (label === 'positive' ? 1 : 0),
              negativeCount: session.negativeCount + (label === 'negative' ? 1 : 0),
              skippedCount: session.skippedCount + (label === 'skipped' ? 1 : 0),
            });
          }
        }
        break;
      }
      case 'PreferenceExampleReverted': {
        const example = examples.get(event.payload.exampleId);
        if (example && !example.revertedAt) {
          examples.set(example.id, { ...example, revertedAt: event.payload.revertedAt });
          const session = sessions.get(example.teachingSessionId);
          if (session) {
            sessions.set(session.id, {
              ...session,
              positiveCount: Math.max(0, session.positiveCount - (example.label === 'positive' ? 1 : 0)),
              negativeCount: Math.max(0, session.negativeCount - (example.label === 'negative' ? 1 : 0)),
              skippedCount: Math.max(0, session.skippedCount - (example.label === 'skipped' ? 1 : 0)),
            });
          }
        }
        break;
      }
      case 'TeachingSessionCompleted': {
        const session = sessions.get(event.payload.sessionId);
        if (session) sessions.set(session.id, {
          ...session,
          completedAt: event.payload.completedAt,
          status: 'summarized',
          summary: event.payload.summary,
        });
        break;
      }
      case 'PreferenceChangeProposed': {
        const preference = { ...event.payload.preference, status: 'candidate' as const };
        preferences.set(preference.version, preference);
        intents.set(preference.version, event.payload.intents);
        if (event.payload.teachingSessionId) {
          const session = sessions.get(event.payload.teachingSessionId);
          if (session) sessions.set(session.id, {
            ...session,
            proposedPreferenceVersion: preference.version,
            status: 'completed',
          });
        }
        break;
      }
      case 'PreferenceSimulationCompleted':
        simulations.set(event.payload.candidateVersion, event.payload.summary);
        break;
      case 'PreferenceShadowStarted':
        shadows.set(event.payload.shadow.id, event.payload.shadow);
        break;
      case 'PreferenceApplied': {
        const candidate = preferences.get(event.payload.version);
        if (!candidate || candidate.id !== event.payload.preferenceId) break;
        if (state.activePreferenceVersion !== null) {
          const active = preferences.get(state.activePreferenceVersion);
          if (active) preferences.set(active.version, { ...active, status: 'superseded' });
        }
        preferences.set(candidate.version, {
          ...candidate,
          status: 'active',
          activeFrom: event.payload.appliedAt,
          updatedAt: event.payload.appliedAt,
        });
        state.activePreferenceVersion = candidate.version;
        break;
      }
      case 'PreferenceReverted': {
        const from = preferences.get(event.payload.fromVersion);
        const to = preferences.get(event.payload.toVersion);
        if (!from || !to || from.id !== event.payload.preferenceId || to.id !== event.payload.preferenceId) break;
        preferences.set(from.version, { ...from, status: 'reverted', updatedAt: event.payload.revertedAt });
        preferences.set(to.version, { ...to, status: 'active', updatedAt: event.payload.revertedAt });
        state.activePreferenceVersion = to.version;
        break;
      }
      case 'MessageFilterDecisionMade':
        decisions.set(event.payload.decision.messageId, event.payload.decision);
        break;
      case 'MessageDecisionCorrected':
        decisions.set(event.payload.correctedDecision.messageId, event.payload.correctedDecision);
        examples.set(event.payload.example.id, event.payload.example);
        break;
      case 'IntentMapUpdated':
        intents.set(event.payload.version, event.payload.intents);
        break;
      case 'TemporaryFocusCreated':
        temporaryFocuses.set(event.payload.focus.id, event.payload.focus);
        break;
      case 'TemporaryFocusExpired': {
        const focus = temporaryFocuses.get(event.payload.focusId);
        if (focus) temporaryFocuses.set(focus.id, { ...focus, expiredAt: event.payload.expiredAt });
        break;
      }
    }
  }

  return state;
}
