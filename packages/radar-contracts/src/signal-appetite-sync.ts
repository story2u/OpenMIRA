import type { components } from './openapi';

export type SignalAppetiteEventType =
  | 'TeachingSessionStarted'
  | 'TeachingCardPresented'
  | 'PreferenceExampleCaptured'
  | 'PreferenceExampleReverted'
  | 'TeachingSessionCompleted'
  | 'PreferenceChangeProposed'
  | 'PreferenceSimulationCompleted'
  | 'PreferenceShadowStarted'
  | 'PreferenceApplied'
  | 'PreferenceReverted'
  | 'MessageFilterDecisionMade'
  | 'MessageDecisionCorrected'
  | 'IntentMapUpdated'
  | 'TemporaryFocusCreated'
  | 'TemporaryFocusExpired';
export type SignalAppetiteEventWrite = components['schemas']['SignalAppetiteEventWrite'];
export type SignalAppetiteEvent = components['schemas']['SignalAppetiteEventRead'];
export type SignalAppetiteEventsAppendRequest =
  components['schemas']['SignalAppetiteEventsAppendRequest'];
export type SignalAppetiteEventsAppend =
  components['schemas']['SignalAppetiteEventsAppendRead'];
export type SignalAppetiteEventsPage =
  components['schemas']['SignalAppetiteEventsPageRead'];

export interface SignalAppetiteEventsQuery {
  after: number;
  limit?: number;
}
