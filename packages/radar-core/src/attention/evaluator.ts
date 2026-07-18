import type {
  AppetiteSimulationSummary,
  AttentionIntent,
  AttentionScheduleWindow,
  DeliveryMode,
  MessageFilterDecision,
  TemporaryFocus,
} from './model';

export interface AppetiteMessage {
  id: string;
  sourceKey: string;
  sourceTrusted: boolean;
  senderImportant: boolean;
  duplicate: boolean;
  explicitAdvertisingSource: boolean;
  hardExcluded: boolean;
  needsReply: boolean;
  topicKeys: readonly string[];
  semanticScores: Readonly<Record<string, number>>;
  sentAt: string;
}

export interface AppetiteEvaluationInput {
  message: AppetiteMessage;
  preferenceVersion: number;
  intents: readonly AttentionIntent[];
  schedule: readonly AttentionScheduleWindow[];
  temporaryFocuses: readonly TemporaryFocus[];
  cloudAvailable: boolean;
  now: string;
}

const deliveryRank: Readonly<Record<DeliveryMode, number>> = {
  suppress: 0,
  digest: 1,
  inbox: 2,
  immediate: 3,
};

function activeSchedule(
  windows: readonly AttentionScheduleWindow[],
  timestamp: string,
) {
  const date = new Date(timestamp);
  const day = date.getUTCDay();
  const minute = date.getUTCHours() * 60 + date.getUTCMinutes();
  return windows.find((window) => (
    window.days.includes(day)
    && (window.startMinute <= window.endMinute
      ? minute >= window.startMinute && minute < window.endMinute
      : minute >= window.startMinute || minute < window.endMinute)
  ));
}

function decision(
  input: AppetiteEvaluationInput,
  mode: DeliveryMode,
  confidence: number,
  reasonSummary: string,
  evaluator: MessageFilterDecision['evaluator'],
  evidence: MessageFilterDecision['evidence'],
): MessageFilterDecision {
  return {
    messageId: input.message.id,
    preferenceVersion: input.preferenceVersion,
    decision: mode,
    confidence,
    reasonSummary,
    evaluator,
    evidence,
    decidedAt: input.now,
    expiresAt: null,
  };
}

export function evaluateMessage(input: AppetiteEvaluationInput): MessageFilterDecision {
  const { message } = input;
  const activeFocus = input.temporaryFocuses.find((focus) => (
    !focus.expiredAt
    && focus.expiresAt > input.now
    && (message.topicKeys.includes(focus.concept) || (message.semanticScores[focus.concept] ?? 0) >= 0.62)
  ));
  if (activeFocus) {
    return decision(input, activeFocus.deliveryMode, 0.96, `Temporary focus: ${activeFocus.concept}`,
      'deterministic', [{ kind: 'temporary_focus', label: activeFocus.concept, referenceId: activeFocus.id }]);
  }
  if (message.senderImportant || message.needsReply) {
    return decision(input, 'immediate', 0.95, message.needsReply ? 'Needs your reply' : 'Important sender',
      'deterministic', [{ kind: 'message_signal', label: message.needsReply ? 'needs_reply' : 'important_sender' }]);
  }
  if (message.hardExcluded || message.explicitAdvertisingSource) {
    return decision(input, 'suppress', 0.98, message.hardExcluded ? 'Source excluded by you' : 'Known advertising source',
      'deterministic', [{ kind: 'source', label: message.sourceKey }]);
  }
  if (message.duplicate) {
    return decision(input, 'suppress', 0.94, 'Duplicate message', 'deterministic',
      [{ kind: 'message_signal', label: 'duplicate' }]);
  }

  const schedule = activeSchedule(input.schedule, message.sentAt);
  const activeIntentIds = schedule ? new Set(schedule.activeIntentIds) : null;
  let best: { intent: AttentionIntent; score: number } | null = null;
  for (const intent of input.intents) {
    if (intent.validFrom && intent.validFrom > input.now) continue;
    if (intent.validUntil && intent.validUntil <= input.now) continue;
    if (activeIntentIds && !activeIntentIds.has(intent.id)) continue;
    const semantic = Math.max(
      message.topicKeys.includes(intent.concept) ? 1 : 0,
      message.semanticScores[intent.concept] ?? 0,
    );
    const score = semantic * Math.abs(intent.weight) * intent.confidence;
    if (!best || score > best.score) best = { intent, score };
  }

  if (!best || best.score < 0.34) {
    const fallback = schedule?.fallbackDeliveryMode ?? 'inbox';
    const safeFallback = fallback === 'suppress' && !input.cloudAvailable ? 'digest' : fallback;
    return decision(input, safeFallback, 0.42, 'No strong match in the current appetite',
      'on_device_model', schedule ? [{ kind: 'schedule', label: schedule.label }] : []);
  }
  if (best.score < 0.62 && !input.cloudAvailable) {
    return decision(input, best.intent.intentType === 'reduce' ? 'digest' : 'inbox', best.score,
      'Kept for review because deeper analysis is unavailable', 'on_device_model',
      [{ kind: 'preference', label: best.intent.concept, referenceId: best.intent.id }]);
  }
  const mode = best.intent.intentType === 'reduce'
    ? (deliveryRank[best.intent.deliveryMode] < deliveryRank.digest ? 'suppress' : best.intent.deliveryMode)
    : best.intent.deliveryMode;
  return decision(input, mode, Math.min(0.93, 0.45 + best.score * 0.5),
    `${best.intent.intentType === 'reduce' ? 'Reduced' : 'Kept'}: ${best.intent.concept}`,
    best.score < 0.62 ? 'cloud_agent' : 'on_device_model',
    [{ kind: 'preference', label: best.intent.concept, referenceId: best.intent.id }]);
}

export function simulateAppetite(
  messages: readonly AppetiteMessage[],
  evaluateCandidate: (message: AppetiteMessage) => MessageFilterDecision,
  previousDecisions: ReadonlyMap<string, MessageFilterDecision> = new Map(),
): AppetiteSimulationSummary {
  const counts: Record<DeliveryMode, number> = { immediate: 0, inbox: 0, digest: 0, suppress: 0 };
  const newlyRetained: string[] = [];
  const newlySuppressed: string[] = [];
  const boundary: string[] = [];
  const sourceChanges = new Map<string, number>();
  for (const message of messages) {
    const next = evaluateCandidate(message);
    counts[next.decision] += 1;
    const previous = previousDecisions.get(message.id);
    if (previous && previous.decision !== next.decision) {
      sourceChanges.set(message.sourceKey, (sourceChanges.get(message.sourceKey) ?? 0) + 1);
      if (deliveryRank[next.decision] >= deliveryRank.inbox && deliveryRank[previous.decision] < deliveryRank.inbox) {
        newlyRetained.push(message.id);
      }
      if (next.decision === 'suppress' && previous.decision !== 'suppress') {
        newlySuppressed.push(message.id);
      }
    }
    if (next.confidence < 0.62) boundary.push(message.id);
  }
  return {
    originalCount: messages.length,
    immediateCount: counts.immediate,
    inboxCount: counts.inbox,
    digestCount: counts.digest,
    suppressCount: counts.suppress,
    newlyRetainedMessageIds: newlyRetained.slice(0, 5),
    newlySuppressedMessageIds: newlySuppressed.slice(0, 5),
    boundaryMessageIds: boundary.slice(0, 5),
    largestChangeSources: [...sourceChanges.entries()]
      .sort(([leftKey, left], [rightKey, right]) => right - left || leftKey.localeCompare(rightKey))
      .slice(0, 5)
      .map(([source]) => source),
    riskSummary: boundary.length > 0
      ? `${boundary.length} boundary messages need review`
      : 'No high-risk boundary changes detected',
  };
}
