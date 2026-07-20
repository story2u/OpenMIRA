import type { BriefingEvent } from './events';
import type {
  AttentionSnapshot,
  Briefing,
  BriefingItem,
  BriefingScheduleEntry,
} from './model';
import { DEFAULT_BRIEFING_SCHEDULE } from './model';

export interface BriefingState {
  briefings: ReadonlyMap<string, Briefing>;
  itemsByBriefing: ReadonlyMap<string, readonly BriefingItem[]>;
  openedBriefingIds: ReadonlySet<string>;
  latestSnapshot: AttentionSnapshot | null;
  schedule: readonly BriefingScheduleEntry[];
  quietRestoredMessageIds: ReadonlySet<string>;
  appliedEventIds: ReadonlySet<string>;
}

function emptyState(): BriefingState {
  return {
    briefings: new Map(),
    itemsByBriefing: new Map(),
    openedBriefingIds: new Set(),
    latestSnapshot: null,
    schedule: DEFAULT_BRIEFING_SCHEDULE,
    quietRestoredMessageIds: new Set(),
    appliedEventIds: new Set(),
  };
}

function compareEvents(left: BriefingEvent, right: BriefingEvent) {
  return left.sequence - right.sequence || left.eventId.localeCompare(right.eventId);
}

/** 幂等 fold：同 eventId 只应用一次；乱序输入按 sequence 收敛到同一状态。 */
export function foldBriefings(events: readonly BriefingEvent[]): BriefingState {
  const state = emptyState();
  const briefings = new Map<string, Briefing>();
  const itemsByBriefing = new Map<string, BriefingItem[]>();
  const opened = new Set<string>();
  const quietRestored = new Set<string>();
  const applied = new Set<string>();
  let latestSnapshot: AttentionSnapshot | null = null;
  let schedule: readonly BriefingScheduleEntry[] = state.schedule;

  for (const event of [...events].sort(compareEvents)) {
    if (applied.has(event.eventId)) continue;
    applied.add(event.eventId);
    switch (event.type) {
      case 'BriefingGenerated': {
        briefings.set(event.payload.briefing.id, event.payload.briefing);
        itemsByBriefing.set(
          event.payload.briefing.id,
          [...event.payload.items].sort((a, b) => a.order - b.order),
        );
        break;
      }
      case 'BriefingOpened': {
        opened.add(event.payload.briefingId);
        break;
      }
      case 'BriefingItemHandled': {
        const items = itemsByBriefing.get(event.payload.briefingId);
        if (items) {
          itemsByBriefing.set(
            event.payload.briefingId,
            items.map((item) => (item.id === event.payload.itemId ? { ...item, handled: true } : item)),
          );
        }
        break;
      }
      case 'BriefingDismissed': {
        const briefing = briefings.get(event.payload.briefingId);
        if (briefing) briefings.set(briefing.id, { ...briefing, status: 'dismissed' });
        break;
      }
      case 'AttentionSnapshotUpdated': {
        if (!latestSnapshot || event.payload.snapshot.generatedAt >= latestSnapshot.generatedAt) {
          latestSnapshot = event.payload.snapshot;
        }
        break;
      }
      case 'QuietItemRestored': {
        quietRestored.add(event.payload.messageId);
        break;
      }
      case 'BriefingScheduleUpdated': {
        schedule = event.payload.entries;
        break;
      }
      case 'BriefingScheduled':
      case 'BriefingGenerationStarted':
      case 'QuietItemAdded':
        break;
    }
  }

  return {
    briefings,
    itemsByBriefing,
    openedBriefingIds: opened,
    latestSnapshot,
    schedule,
    quietRestoredMessageIds: quietRestored,
    appliedEventIds: applied,
  };
}

/** 增量简报的去重来源：给定窗口内所有 ready 简报覆盖过的消息 id 并集。 */
export function includedMessageIdsSince(state: BriefingState, sinceIso: string): Set<string> {
  const ids = new Set<string>();
  for (const briefing of state.briefings.values()) {
    if (briefing.status === 'dismissed') continue;
    if (briefing.coveredTo < sinceIso) continue;
    for (const id of briefing.includedMessageIds) ids.add(id);
  }
  return ids;
}

/** 已处理实体 id 并集：晚间摘要排除已处理项的依据。 */
export function handledEntityIds(state: BriefingState): Set<string> {
  const ids = new Set<string>();
  for (const items of state.itemsByBriefing.values()) {
    for (const item of items) if (item.handled) ids.add(item.entityId);
  }
  return ids;
}
