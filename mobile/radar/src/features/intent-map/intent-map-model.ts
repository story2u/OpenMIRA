import type {
  AppetiteSimulationSummary,
  AttentionIntent,
  AttentionPreference,
  MessageFilterDecision,
  ShadowEvaluation,
  TemporaryFocus,
  AttentionScheduleWindow,
} from '@story2u/radar-core/attention/model';

export type IntentMapNodeKind = 'self' | 'core' | 'context' | 'reduce' | 'temporary';

export interface IntentMapNode {
  id: string;
  label: string;
  kind: IntentMapNodeKind;
  x: number;
  y: number;
  radius: number;
  confidence: number;
  confirmed: boolean;
  deliveryMode: AttentionIntent['deliveryMode'] | TemporaryFocus['deliveryMode'] | null;
  badge: string | null;
}

export interface IntentMapEdge {
  id: string;
  from: string;
  to: string;
  strength: number;
  confirmed: boolean;
}

export interface FilteringStats {
  immediate: number;
  inbox: number;
  digest: number;
  suppress: number;
  total: number;
}

export interface IntentMapSnapshot {
  preference: AttentionPreference | null;
  intents: readonly AttentionIntent[];
  temporaryFocuses: readonly TemporaryFocus[];
  decisions: readonly MessageFilterDecision[];
  shadow: ShadowEvaluation | null;
}

export interface IntentMapModel {
  nodes: readonly IntentMapNode[];
  edges: readonly IntentMapEdge[];
  stats: FilteringStats;
  preference: AttentionPreference | null;
  shadow: ShadowEvaluation | null;
  simulation: AppetiteSimulationSummary | null;
  candidate: {
    nodes: readonly IntentMapNode[];
    edges: readonly IntentMapEdge[];
    preference: AttentionPreference;
    stats: FilteringStats;
  } | null;
}

export function scheduleWindowAt(
  windows: readonly AttentionScheduleWindow[],
  minute: number,
  day: number,
) {
  return windows.find((window) => (
    window.days.includes(day)
    && minute >= window.startMinute
    && minute < window.endMinute
  )) ?? null;
}

function hash(value: string) {
  let result = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index);
    result = Math.imul(result, 16777619);
  }
  return result >>> 0;
}

function stats(decisions: readonly MessageFilterDecision[]): FilteringStats {
  const result = { immediate: 0, inbox: 0, digest: 0, suppress: 0, total: decisions.length };
  decisions.forEach((decision) => { result[decision.decision] += 1; });
  return result;
}

export function buildIntentMapModel(snapshot: IntentMapSnapshot): IntentMapModel {
  const self: IntentMapNode = {
    id: 'self', label: 'self', kind: 'self', x: 180, y: 164, radius: 42,
    confidence: snapshot.preference?.confidence ?? 0, confirmed: true,
    deliveryMode: null, badge: null,
  };
  const activeFocuses = snapshot.temporaryFocuses
    .filter((focus) => !focus.expiredAt)
    .sort((left, right) => left.id.localeCompare(right.id))
    .slice(0, 4);
  const intents = [...snapshot.intents]
    .sort((left, right) => (
      Math.abs(right.weight) - Math.abs(left.weight)
      || hash(left.id) - hash(right.id)
      || left.id.localeCompare(right.id)
    ));
  const primary = [
    ...intents.filter((intent) => intent.intentType === 'include').slice(0, 6),
    ...intents.filter((intent) => intent.intentType === 'context').slice(0, 12),
  ].slice(0, Math.max(0, 23 - activeFocuses.length));
  const reduced = intents.filter((intent) => intent.intentType === 'reduce').slice(0, 6);
  const primaryNodes = primary.map((intent, index): IntentMapNode => {
    const count = Math.max(1, primary.length);
    const baseAngle = -Math.PI * 0.92 + (Math.PI * 1.84 * (index + 0.5)) / count;
    const jitter = ((hash(intent.id) % 17) - 8) * 0.007;
    const ring = index < 6 ? 92 : 132;
    const angle = baseAngle + jitter;
    return {
      id: intent.id,
      label: intent.concept,
      kind: intent.intentType === 'context' ? 'context' : 'core',
      x: 180 + Math.cos(angle) * ring,
      y: 164 + Math.sin(angle) * ring * 0.82,
      radius: 20 + Math.round(Math.abs(intent.weight) * 10),
      confidence: intent.confidence,
      confirmed: intent.userConfirmed,
      deliveryMode: intent.deliveryMode,
      badge: null,
    };
  });
  const reduceNodes = reduced.map((intent, index): IntentMapNode => ({
    id: intent.id,
    label: intent.concept,
    kind: 'reduce',
    x: 42 + (index % 3) * 138,
    y: 302 + Math.floor(index / 3) * 48,
    radius: 18 + Math.round(Math.abs(intent.weight) * 6),
    confidence: intent.confidence,
    confirmed: intent.userConfirmed,
    deliveryMode: intent.deliveryMode,
    badge: null,
  }));
  const focusNodes = activeFocuses.map((focus, index): IntentMapNode => {
    const angle = -Math.PI + ((index + 1) * Math.PI) / (activeFocuses.length + 1);
    return {
      id: focus.id,
      label: focus.concept,
      kind: 'temporary',
      x: 180 + Math.cos(angle) * 74,
      y: 155 + Math.sin(angle) * 64,
      radius: 23,
      confidence: 1,
      confirmed: true,
      deliveryMode: focus.deliveryMode,
      badge: 'temporary',
    };
  });
  const nodes = [self, ...primaryNodes, ...focusNodes, ...reduceNodes].slice(0, 30);
  const edges = nodes.slice(1).map((node): IntentMapEdge => ({
    id: `self:${node.id}`,
    from: 'self',
    to: node.id,
    strength: Math.max(0.25, node.radius / 32),
    confirmed: node.confirmed,
  }));
  return {
    nodes,
    edges,
    stats: stats(snapshot.decisions),
    preference: snapshot.preference,
    shadow: snapshot.shadow,
    simulation: snapshot.shadow?.diffSummary ?? null,
    candidate: null,
  };
}
