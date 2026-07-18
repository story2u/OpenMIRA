export type DeliveryMode = 'immediate' | 'inbox' | 'digest' | 'suppress';
export type DecisionEvaluator = 'deterministic' | 'on_device_model' | 'cloud_agent';
export type PreferenceExampleLabel = 'positive' | 'negative' | 'skipped' | 'boundary';
export type AttentionIntentType = 'include' | 'reduce' | 'context';
export type AttentionPreferenceStatus = 'candidate' | 'active' | 'superseded' | 'reverted';
export type TeachingSessionStatus = 'active' | 'summarized' | 'completed' | 'abandoned';
export type ShadowEvaluationStatus = 'running' | 'completed' | 'applied' | 'abandoned';

export interface AttentionScheduleWindow {
  id: string;
  days: readonly number[];
  startMinute: number;
  endMinute: number;
  label: string;
  activeIntentIds: readonly string[];
  fallbackDeliveryMode: Exclude<DeliveryMode, 'immediate'>;
}

export interface AttentionPreference {
  id: string;
  title: string;
  naturalLanguageSummary: string;
  scope: 'all_messages' | 'opportunities' | 'jobs';
  status: AttentionPreferenceStatus;
  confidence: number;
  version: number;
  activeFrom: string | null;
  activeUntil: string | null;
  schedule: readonly AttentionScheduleWindow[];
  createdAt: string;
  updatedAt: string;
}

export interface AttentionIntent {
  id: string;
  preferenceId: string;
  concept: string;
  intentType: AttentionIntentType;
  weight: number;
  deliveryMode: DeliveryMode;
  confidence: number;
  userConfirmed: boolean;
  source: 'teaching' | 'conversation' | 'correction' | 'temporary_focus';
  validFrom: string | null;
  validUntil: string | null;
}

export interface PreferenceExample {
  id: string;
  messageId: string;
  label: PreferenceExampleLabel;
  selectedReasons: readonly string[];
  freeformReason: string | null;
  capturedAt: string;
  teachingSessionId: string;
  revertedAt: string | null;
}

export interface MessageDecisionEvidence {
  kind: 'preference' | 'source' | 'message_signal' | 'schedule' | 'temporary_focus';
  label: string;
  referenceId?: string;
}

export interface MessageFilterDecision {
  messageId: string;
  preferenceVersion: number;
  decision: DeliveryMode;
  confidence: number;
  reasonSummary: string;
  evidence: readonly MessageDecisionEvidence[];
  evaluator: DecisionEvaluator;
  decidedAt: string;
  expiresAt: string | null;
}

export interface TeachingSession {
  id: string;
  startedAt: string;
  completedAt: string | null;
  presentedCount: number;
  positiveCount: number;
  negativeCount: number;
  skippedCount: number;
  status: TeachingSessionStatus;
  proposedPreferenceVersion: number | null;
  summary: {
    increase: readonly string[];
    reduce: readonly string[];
  } | null;
}

export interface AppetiteSimulationSummary {
  originalCount: number;
  immediateCount: number;
  inboxCount: number;
  digestCount: number;
  suppressCount: number;
  newlyRetainedMessageIds: readonly string[];
  newlySuppressedMessageIds: readonly string[];
  boundaryMessageIds: readonly string[];
  largestChangeSources: readonly string[];
  riskSummary: string;
}

export interface ShadowEvaluation {
  id: string;
  oldVersion: number;
  candidateVersion: number;
  startedAt: string;
  endsAt: string;
  diffSummary: AppetiteSimulationSummary;
  status: ShadowEvaluationStatus;
}

export interface TemporaryFocus {
  id: string;
  concept: string;
  deliveryMode: DeliveryMode;
  createdAt: string;
  expiresAt: string;
  expiredAt: string | null;
}
