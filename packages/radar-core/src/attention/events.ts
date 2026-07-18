import type {
  AppetiteSimulationSummary,
  AttentionIntent,
  AttentionPreference,
  MessageFilterDecision,
  PreferenceExample,
  ShadowEvaluation,
  TemporaryFocus,
} from './model';

export const SIGNAL_APPETITE_EVENT_SCHEMA_VERSION = 1;

interface SignalAppetiteEventBase<Type extends string, Payload> {
  eventId: string;
  ownerId: string;
  deviceId: string;
  sequence: number;
  aggregateId: string;
  aggregateVersion: number;
  schemaVersion: typeof SIGNAL_APPETITE_EVENT_SCHEMA_VERSION;
  occurredAt: string;
  type: Type;
  payload: Payload;
}

export type SignalAppetiteEvent =
  | SignalAppetiteEventBase<'TeachingSessionStarted', {
    sessionId: string;
    targetCount: number;
  }>
  | SignalAppetiteEventBase<'TeachingCardPresented', {
    sessionId: string;
    messageId: string;
    selectionScore: number;
  }>
  | SignalAppetiteEventBase<'PreferenceExampleCaptured', {
    example: PreferenceExample;
  }>
  | SignalAppetiteEventBase<'PreferenceExampleReverted', {
    exampleId: string;
    revertedAt: string;
  }>
  | SignalAppetiteEventBase<'TeachingSessionCompleted', {
    sessionId: string;
    completedAt: string;
    summary: { increase: readonly string[]; reduce: readonly string[] };
  }>
  | SignalAppetiteEventBase<'PreferenceChangeProposed', {
    preference: AttentionPreference;
    intents: readonly AttentionIntent[];
    teachingSessionId: string | null;
  }>
  | SignalAppetiteEventBase<'PreferenceSimulationCompleted', {
    preferenceId: string;
    candidateVersion: number;
    summary: AppetiteSimulationSummary;
  }>
  | SignalAppetiteEventBase<'PreferenceShadowStarted', {
    shadow: ShadowEvaluation;
  }>
  | SignalAppetiteEventBase<'PreferenceApplied', {
    preferenceId: string;
    version: number;
    appliedAt: string;
  }>
  | SignalAppetiteEventBase<'PreferenceReverted', {
    preferenceId: string;
    fromVersion: number;
    toVersion: number;
    revertedAt: string;
  }>
  | SignalAppetiteEventBase<'MessageFilterDecisionMade', {
    decision: MessageFilterDecision;
  }>
  | SignalAppetiteEventBase<'MessageDecisionCorrected', {
    previousDecision: MessageFilterDecision;
    correctedDecision: MessageFilterDecision;
    example: PreferenceExample;
  }>
  | SignalAppetiteEventBase<'IntentMapUpdated', {
    preferenceId: string;
    version: number;
    intents: readonly AttentionIntent[];
  }>
  | SignalAppetiteEventBase<'TemporaryFocusCreated', {
    focus: TemporaryFocus;
  }>
  | SignalAppetiteEventBase<'TemporaryFocusExpired', {
    focusId: string;
    expiredAt: string;
  }>;

export type SignalAppetiteEventType = SignalAppetiteEvent['type'];
