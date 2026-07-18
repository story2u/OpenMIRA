import type { DeliveryMode, PreferenceExample } from './model';

export interface TeachingCandidate {
  messageId: string;
  sourceKey: string;
  topicKeys: readonly string[];
  confidence: number;
  currentDecision: DeliveryMode;
  candidateDecision: DeliveryMode;
  openedRecently: boolean;
  ignoredRecently: boolean;
  duplicate: boolean;
  likelyNoise: boolean;
  highValueDomain: boolean;
  allowedForTeaching: boolean;
  sensitive: boolean;
}

export interface ScoredTeachingCandidate extends TeachingCandidate {
  selectionScore: number;
  selectionReasons: readonly string[];
}

export interface TeachingSummary {
  increase: readonly string[];
  reduce: readonly string[];
  positiveCount: number;
  negativeCount: number;
  skippedCount: number;
}

function clamp(value: number, minimum = 0, maximum = 1) {
  return Math.min(maximum, Math.max(minimum, value));
}

export function scoreTeachingCandidate(candidate: TeachingCandidate): ScoredTeachingCandidate | null {
  if (!candidate.allowedForTeaching || candidate.sensitive) return null;
  const changed = candidate.currentDecision !== candidate.candidateDecision;
  const boundary = 1 - Math.abs(clamp(candidate.confidence) - 0.5) * 2;
  if (boundary < 0.12 && !changed && !candidate.openedRecently && !candidate.ignoredRecently) {
    return null;
  }

  const reasons: string[] = [];
  let score = boundary * 0.52;
  if (boundary >= 0.55) reasons.push('boundary');
  if (changed) {
    score += 0.24;
    reasons.push('decision_change');
  }
  const currentlyReduced = candidate.currentDecision === 'digest' || candidate.currentDecision === 'suppress';
  const currentlyRetained = candidate.currentDecision === 'immediate' || candidate.currentDecision === 'inbox';
  if (candidate.openedRecently && currentlyReduced) {
    score += 0.15;
    reasons.push('opened_but_reduced');
  }
  if (candidate.ignoredRecently && currentlyRetained) {
    score += 0.15;
    reasons.push('ignored_but_retained');
  }
  if (candidate.highValueDomain) {
    score += 0.06;
    reasons.push('high_value_domain');
  }
  if (candidate.duplicate || candidate.likelyNoise) {
    score += 0.03;
    reasons.push(candidate.duplicate ? 'duplicate' : 'likely_noise');
  }
  return { ...candidate, selectionScore: clamp(score), selectionReasons: reasons };
}

export function selectTeachingCards(
  candidates: readonly TeachingCandidate[],
  options: { targetCount?: number; maximumPerSource?: number } = {},
): ScoredTeachingCandidate[] {
  const targetCount = Math.min(15, Math.max(5, options.targetCount ?? 8));
  const maximumPerSource = Math.min(3, Math.max(1, options.maximumPerSource ?? 2));
  const sourceCounts = new Map<string, number>();
  const remaining = candidates
    .map(scoreTeachingCandidate)
    .filter((candidate): candidate is ScoredTeachingCandidate => Boolean(candidate))
    .sort((left, right) => (
      right.selectionScore - left.selectionScore || left.messageId.localeCompare(right.messageId)
    ));
  const selected: ScoredTeachingCandidate[] = [];

  while (remaining.length > 0 && selected.length < targetCount) {
    const previous = selected.at(-1);
    let index = remaining.findIndex((candidate) => {
      if ((sourceCounts.get(candidate.sourceKey) ?? 0) >= maximumPerSource) return false;
      if (!previous) return true;
      return !candidate.topicKeys.some((topic) => previous.topicKeys.includes(topic));
    });
    if (index < 0) {
      index = remaining.findIndex(
        (candidate) => (sourceCounts.get(candidate.sourceKey) ?? 0) < maximumPerSource,
      );
    }
    if (index < 0) break;
    const [candidate] = remaining.splice(index, 1);
    selected.push(candidate);
    sourceCounts.set(candidate.sourceKey, (sourceCounts.get(candidate.sourceKey) ?? 0) + 1);
  }
  return selected;
}

export function summarizeTeachingExamples(
  examples: readonly PreferenceExample[],
  reasonLabels: Readonly<Record<string, string>> = {},
): TeachingSummary {
  const positiveReasons = new Map<string, number>();
  const negativeReasons = new Map<string, number>();
  let positiveCount = 0;
  let negativeCount = 0;
  let skippedCount = 0;
  for (const example of examples) {
    if (example.revertedAt) continue;
    if (example.label === 'positive') positiveCount += 1;
    if (example.label === 'negative') negativeCount += 1;
    if (example.label === 'skipped') skippedCount += 1;
    const target = example.label === 'positive'
      ? positiveReasons
      : example.label === 'negative'
        ? negativeReasons
        : null;
    if (!target) continue;
    for (const reason of example.selectedReasons) {
      target.set(reason, (target.get(reason) ?? 0) + 1);
    }
  }
  const ranked = (counts: ReadonlyMap<string, number>) => [...counts.entries()]
    .sort(([leftKey, left], [rightKey, right]) => right - left || leftKey.localeCompare(rightKey))
    .slice(0, 6)
    .map(([reason]) => reasonLabels[reason] ?? reason);
  return {
    increase: ranked(positiveReasons),
    reduce: ranked(negativeReasons),
    positiveCount,
    negativeCount,
    skippedCount,
  };
}
