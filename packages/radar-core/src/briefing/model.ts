import type { DecisionEvaluator, DeliveryMode } from '../attention/model';

export type BriefingType = 'morning' | 'midday' | 'evening' | 'ad_hoc' | 'urgent';
export type BriefingStatus = 'scheduled' | 'generating' | 'ready' | 'dismissed';
export type BriefingGenerator = 'local' | 'cloud';
export type BriefingItemType = 'message' | 'opportunity';
export type BriefingItemPriority = 'action_required' | 'worth_attention' | 'needs_judgment' | 'later';
export type ScheduledBriefingType = Exclude<BriefingType, 'ad_hoc' | 'urgent'>;

export interface BriefingCategoryCount {
  category: string;
  count: number;
}

export interface BriefingEvidenceRef {
  entityType: 'message' | 'opportunity' | 'decision';
  entityId: string;
}

/** 时间段简报：由结构化决策 fold 生成，模型文本只允许充当 summary 的语言整理。 */
export interface Briefing {
  id: string;
  type: BriefingType;
  title: string;
  /** 云端语言整理结果；null 表示由客户端按结构化数据渲染本地兜底文案。 */
  summary: string | null;
  coveredFrom: string;
  coveredTo: string;
  generatedAt: string;
  generatedBy: BriefingGenerator;
  status: BriefingStatus;
  totalMessages: number;
  immediateCount: number;
  inboxCount: number;
  digestCount: number;
  suppressedCount: number;
  includedMessageIds: readonly string[];
  includedOpportunityIds: readonly string[];
  excludedHandledIds: readonly string[];
  categorySummaries: readonly BriefingCategoryCount[];
  evidenceRefs: readonly BriefingEvidenceRef[];
}

export interface BriefingItem {
  id: string;
  briefingId: string;
  itemType: BriefingItemType;
  entityId: string;
  priority: BriefingItemPriority;
  reasonSummary: string;
  actionRequired: boolean;
  handled: boolean;
  order: number;
}

/** Mira 今日处理快照：全部来自决策统计，不承载模型自由文本。 */
export interface AttentionSnapshot {
  id: string;
  generatedAt: string;
  totalProcessed: number;
  localProcessed: number;
  deepAnalyzed: number;
  immediateCount: number;
  inboxCount: number;
  digestCount: number;
  suppressedCount: number;
  needsUserInputCount: number;
  categoryCounts: readonly BriefingCategoryCount[];
}

export interface QuietItemSummary {
  category: string;
  count: number;
  sourceCount: number;
  topReason: string;
  samples: readonly string[];
}

export interface BriefingScheduleEntry {
  briefingType: ScheduledBriefingType;
  minuteOfDay: number;
  days: readonly number[];
  enabled: boolean;
}

export const DEFAULT_BRIEFING_SCHEDULE: readonly BriefingScheduleEntry[] = Object.freeze([
  { briefingType: 'morning', minuteOfDay: 8 * 60 + 30, days: [1, 2, 3, 4, 5, 6, 0], enabled: true },
  { briefingType: 'midday', minuteOfDay: 12 * 60, days: [1, 2, 3, 4, 5, 6, 0], enabled: true },
  { briefingType: 'evening', minuteOfDay: 18 * 60 + 30, days: [1, 2, 3, 4, 5, 6, 0], enabled: true },
]);

export interface DecisionLike {
  messageId: string;
  decision: DeliveryMode;
  confidence: number;
  reasonSummary: string;
  evaluator: DecisionEvaluator;
  decidedAt: string;
  evidence: readonly { kind: string; label: string; referenceId?: string }[];
}
