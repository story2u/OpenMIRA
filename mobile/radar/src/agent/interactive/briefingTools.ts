import {
  GetAttentionSnapshotParameters,
  GetQuietSummaryParameters,
  INTERACTIVE_BRIEFING_TOOLS,
  ListCategoryItemsParameters,
  ListPriorityItemsParameters,
  SummarizeTimeWindowParameters,
  UpdateBriefScheduleParameters,
  type InteractiveBriefingToolName,
} from '@story2u/radar-agent/interactive';
import type { BriefingType } from '@story2u/radar-core/briefing/model';
import type { TSchema } from 'typebox';
import { Value } from 'typebox/value';

import {
  generateBriefing,
  getAttentionSnapshot,
  getQuietSummary,
  listBriefings,
  updateBriefingSchedule,
} from '../../briefing/briefingService';
import {
  readBriefingItems,
  readCategoryDecisionRefs,
  type BriefingStoreDatabase,
} from '../../briefing/briefingStore';

const schemas: Readonly<Record<InteractiveBriefingToolName, TSchema>> = Object.freeze({
  summarize_time_window: SummarizeTimeWindowParameters,
  get_attention_snapshot: GetAttentionSnapshotParameters,
  list_priority_items: ListPriorityItemsParameters,
  list_category_items: ListCategoryItemsParameters,
  get_quiet_summary: GetQuietSummaryParameters,
  update_brief_schedule: UpdateBriefScheduleParameters,
});
const knownTools = new Set<InteractiveBriefingToolName>(
  INTERACTIVE_BRIEFING_TOOLS.map((tool) => tool.name as InteractiveBriefingToolName),
);

export interface InteractiveBriefingToolCall {
  arguments: unknown;
  name: string;
  toolCallId: string;
}

export class InteractiveBriefingToolError extends Error {
  constructor(code: string) {
    super(code);
    this.name = 'InteractiveBriefingToolError';
  }
}

export async function executeInteractiveBriefingTool(
  database: BriefingStoreDatabase,
  options: {
    allowedTools: ReadonlySet<InteractiveBriefingToolName>;
    call: InteractiveBriefingToolCall;
    deviceId: string;
    now?: Date;
    ownerId: string;
    randomId(): string;
    signal?: AbortSignal;
  },
): Promise<Record<string, unknown>> {
  const { call, ownerId, deviceId, randomId, signal } = options;
  if (!knownTools.has(call.name as InteractiveBriefingToolName)) {
    throw new InteractiveBriefingToolError('unknown_tool');
  }
  const tool = call.name as InteractiveBriefingToolName;
  if (!options.allowedTools.has(tool)) throw new InteractiveBriefingToolError('tool_not_authorized');
  if (!Value.Check(schemas[tool], call.arguments)) {
    throw new InteractiveBriefingToolError('invalid_tool_arguments');
  }
  if (signal?.aborted) throw new InteractiveBriefingToolError('interactive_agent_cancelled');
  const args = call.arguments as Record<string, unknown>;
  const now = options.now ?? new Date();
  const context = { ownerId, deviceId, now: () => now, createId: randomId };

  switch (tool) {
    case 'summarize_time_window': {
      // Agent 只拿结构化结果并据此向用户叙述；持久化 summary 的 L2 语言整理由简报调度器负责。
      const result = await generateBriefing(database, context, {
        type: args.briefing_type as BriefingType,
      });
      return {
        briefingId: result.briefing.id,
        briefingType: result.briefing.type,
        coveredFrom: result.briefing.coveredFrom,
        coveredTo: result.briefing.coveredTo,
        totalMessages: result.briefing.totalMessages,
        immediateCount: result.briefing.immediateCount,
        inboxCount: result.briefing.inboxCount,
        digestCount: result.briefing.digestCount,
        suppressedCount: result.briefing.suppressedCount,
        excludedHandledCount: result.briefing.excludedHandledIds.length,
        categorySummaries: result.briefing.categorySummaries,
        items: result.items.map((item) => ({
          itemId: item.id,
          entityId: item.entityId,
          itemType: item.itemType,
          priority: item.priority,
          reasonSummary: item.reasonSummary,
        })),
        state: 'generated',
      };
    }
    case 'get_attention_snapshot': {
      const snapshot = await getAttentionSnapshot(database, context);
      return { ...snapshot, state: 'ok' };
    }
    case 'list_priority_items': {
      const briefings = await listBriefings(database, context, { limit: 10 });
      const latest = briefings.find((briefing) => briefing.status === 'ready');
      if (!latest) return { items: [], state: 'no_ready_briefing' };
      const limit = (args.limit as number | undefined) ?? 20;
      const priority = args.priority as string | undefined;
      const items = (await readBriefingItems(database, ownerId, latest.id))
        .filter((item) => (priority ? item.priority === priority : true))
        .slice(0, limit)
        .map((item) => ({
          itemId: item.id,
          briefingId: latest.id,
          entityId: item.entityId,
          itemType: item.itemType,
          priority: item.priority,
          reasonSummary: item.reasonSummary,
          handled: item.handled,
        }));
      return { briefingId: latest.id, items, state: 'ok' };
    }
    case 'list_category_items': {
      const dayStart = new Date(now);
      dayStart.setHours(0, 0, 0, 0);
      const refs = await readCategoryDecisionRefs(database, ownerId, {
        category: args.category as string,
        decidedFrom: dayStart.toISOString(),
        limit: (args.limit as number | undefined) ?? 20,
        offset: (args.offset as number | undefined) ?? 0,
      });
      return { category: args.category, items: refs, state: 'ok' };
    }
    case 'get_quiet_summary': {
      const summaries = await getQuietSummary(database, context, {
        sinceIso: args.since as string | undefined,
      });
      return { categories: summaries, state: 'ok' };
    }
    case 'update_brief_schedule': {
      const entries = (args.entries as Array<{
        briefing_type: 'morning' | 'midday' | 'evening';
        minute_of_day: number;
        days: number[];
        enabled: boolean;
      }>).map((entry) => ({
        briefingType: entry.briefing_type,
        minuteOfDay: entry.minute_of_day,
        days: entry.days,
        enabled: entry.enabled,
      }));
      await updateBriefingSchedule(database, context, entries);
      return { entries, state: 'updated' };
    }
  }
}
