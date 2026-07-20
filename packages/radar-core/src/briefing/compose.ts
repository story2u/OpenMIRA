import type {
  AttentionSnapshot,
  Briefing,
  BriefingCategoryCount,
  BriefingItem,
  BriefingItemPriority,
  BriefingType,
  DecisionLike,
  QuietItemSummary,
} from './model';

/** 低于该置信度且未被收起的决策视为"需要用户判断"的边界消息。 */
export const NEEDS_JUDGMENT_CONFIDENCE = 0.55;

const PRIORITY_ORDER: Readonly<Record<BriefingItemPriority, number>> = {
  action_required: 0,
  worth_attention: 1,
  needs_judgment: 2,
  later: 3,
};

/** 决策的主分类：优先取证据里的概念标签，落不到已知概念时归入 other。 */
export function categoryOfDecision(decision: DecisionLike): string {
  const conceptEvidence = decision.evidence.find(
    (entry) => entry.kind === 'preference' || entry.kind === 'message_signal',
  );
  return conceptEvidence?.label ?? 'other';
}

function priorityOfDecision(decision: DecisionLike): BriefingItemPriority {
  if (decision.decision === 'immediate') return 'action_required';
  if (decision.confidence < NEEDS_JUDGMENT_CONFIDENCE && decision.decision !== 'suppress') {
    return 'needs_judgment';
  }
  if (decision.decision === 'inbox') return 'worth_attention';
  return 'later';
}

function countByCategory(decisions: readonly DecisionLike[]): BriefingCategoryCount[] {
  const counts = new Map<string, number>();
  for (const decision of decisions) {
    const category = categoryOfDecision(decision);
    counts.set(category, (counts.get(category) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([category, count]) => ({ category, count }))
    .sort((left, right) => right.count - left.count || left.category.localeCompare(right.category));
}

export interface ComposeSnapshotInput {
  id: string;
  generatedAt: string;
  decisions: readonly DecisionLike[];
}

export function composeAttentionSnapshot(input: ComposeSnapshotInput): AttentionSnapshot {
  const byMode = { immediate: 0, inbox: 0, digest: 0, suppress: 0 };
  let localProcessed = 0;
  let deepAnalyzed = 0;
  let needsUserInputCount = 0;
  for (const decision of input.decisions) {
    byMode[decision.decision] += 1;
    if (decision.evaluator === 'cloud_agent') deepAnalyzed += 1;
    else localProcessed += 1;
    if (priorityOfDecision(decision) === 'needs_judgment') needsUserInputCount += 1;
  }
  return {
    id: input.id,
    generatedAt: input.generatedAt,
    totalProcessed: input.decisions.length,
    localProcessed,
    deepAnalyzed,
    immediateCount: byMode.immediate,
    inboxCount: byMode.inbox,
    digestCount: byMode.digest,
    suppressedCount: byMode.suppress,
    needsUserInputCount,
    categoryCounts: countByCategory(input.decisions),
  };
}

export interface ComposeBriefingInput {
  id: string;
  type: BriefingType;
  title: string;
  coveredFrom: string;
  coveredTo: string;
  generatedAt: string;
  decisions: readonly DecisionLike[];
  /** 此前简报已覆盖的消息 id：增量简报绝不重复展示。 */
  previouslyIncludedIds: ReadonlySet<string>;
  /** 用户已处理的实体 id：晚间摘要必须排除。 */
  handledIds: ReadonlySet<string>;
  opportunityIdByMessageId?: ReadonlyMap<string, string>;
  createItemId: (index: number) => string;
}

export interface ComposedBriefing {
  briefing: Briefing;
  items: readonly BriefingItem[];
}

/**
 * 纯函数组合简报：只做结构化计算与增量去重。summary 一律为 null，
 * 云端语言整理由调用方在持久化前补充；失败保持 null 走本地兜底渲染。
 */
export function composeBriefing(input: ComposeBriefingInput): ComposedBriefing {
  const fresh: DecisionLike[] = [];
  const excludedHandled: string[] = [];
  const seen = new Set<string>();
  for (const decision of input.decisions) {
    if (seen.has(decision.messageId)) continue;
    seen.add(decision.messageId);
    if (decision.decidedAt < input.coveredFrom || decision.decidedAt > input.coveredTo) continue;
    if (input.previouslyIncludedIds.has(decision.messageId)) continue;
    if (input.handledIds.has(decision.messageId)) {
      excludedHandled.push(decision.messageId);
      continue;
    }
    fresh.push(decision);
  }

  const byMode = { immediate: 0, inbox: 0, digest: 0, suppress: 0 };
  for (const decision of fresh) byMode[decision.decision] += 1;

  const visible = fresh.filter((decision) => decision.decision !== 'suppress');
  const ordered = [...visible].sort(
    (left, right) =>
      PRIORITY_ORDER[priorityOfDecision(left)] - PRIORITY_ORDER[priorityOfDecision(right)] ||
      right.decidedAt.localeCompare(left.decidedAt),
  );

  const includedOpportunityIds: string[] = [];
  const items: BriefingItem[] = ordered.map((decision, index) => {
    const opportunityId = input.opportunityIdByMessageId?.get(decision.messageId) ?? null;
    if (opportunityId) includedOpportunityIds.push(opportunityId);
    const priority = priorityOfDecision(decision);
    return {
      id: input.createItemId(index),
      briefingId: input.id,
      itemType: opportunityId ? 'opportunity' : 'message',
      entityId: opportunityId ?? decision.messageId,
      priority,
      reasonSummary: decision.reasonSummary,
      actionRequired: priority === 'action_required',
      handled: false,
      order: index,
    };
  });

  const briefing: Briefing = {
    id: input.id,
    type: input.type,
    title: input.title,
    summary: null,
    coveredFrom: input.coveredFrom,
    coveredTo: input.coveredTo,
    generatedAt: input.generatedAt,
    generatedBy: 'local',
    status: 'ready',
    totalMessages: fresh.length,
    immediateCount: byMode.immediate,
    inboxCount: byMode.inbox,
    digestCount: byMode.digest,
    suppressedCount: byMode.suppress,
    includedMessageIds: fresh.map((decision) => decision.messageId),
    includedOpportunityIds,
    excludedHandledIds: excludedHandled,
    categorySummaries: countByCategory(fresh),
    evidenceRefs: visible.map((decision) => ({ entityType: 'decision', entityId: decision.messageId })),
  };
  return { briefing, items };
}

export interface QuietSummaryInput {
  decisions: readonly DecisionLike[];
  sourceOfMessage?: (messageId: string) => string | null;
  sampleLimit?: number;
}

export function summarizeQuietItems(input: QuietSummaryInput): QuietItemSummary[] {
  const sampleLimit = input.sampleLimit ?? 5;
  const groups = new Map<string, { count: number; sources: Set<string>; reasons: Map<string, number>; samples: string[] }>();
  for (const decision of input.decisions) {
    if (decision.decision !== 'suppress') continue;
    const category = categoryOfDecision(decision);
    const group = groups.get(category) ?? { count: 0, sources: new Set<string>(), reasons: new Map<string, number>(), samples: [] };
    group.count += 1;
    const source = input.sourceOfMessage?.(decision.messageId);
    if (source) group.sources.add(source);
    group.reasons.set(decision.reasonSummary, (group.reasons.get(decision.reasonSummary) ?? 0) + 1);
    if (group.samples.length < sampleLimit) group.samples.push(decision.messageId);
    groups.set(category, group);
  }
  return [...groups.entries()]
    .map(([category, group]) => ({
      category,
      count: group.count,
      sourceCount: group.sources.size,
      topReason: [...group.reasons.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? '',
      samples: group.samples,
    }))
    .sort((left, right) => right.count - left.count || left.category.localeCompare(right.category));
}
