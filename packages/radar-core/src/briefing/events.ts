import type {
  AttentionSnapshot,
  Briefing,
  BriefingItem,
  BriefingScheduleEntry,
  BriefingType,
} from './model';

export const BRIEFING_EVENT_SCHEMA_VERSION = 1;

interface BriefingEventBase<Type extends string, Payload> {
  eventId: string;
  ownerId: string;
  deviceId: string;
  sequence: number;
  aggregateId: string;
  aggregateVersion: number;
  schemaVersion: typeof BRIEFING_EVENT_SCHEMA_VERSION;
  occurredAt: string;
  type: Type;
  payload: Payload;
}

export type BriefingEvent =
  | BriefingEventBase<'BriefingScheduled', {
    briefingId: string;
    briefingType: BriefingType;
    scheduledFor: string;
  }>
  | BriefingEventBase<'BriefingGenerationStarted', {
    briefingId: string;
    briefingType: BriefingType;
  }>
  | BriefingEventBase<'BriefingGenerated', {
    briefing: Briefing;
    items: readonly BriefingItem[];
  }>
  | BriefingEventBase<'BriefingOpened', {
    briefingId: string;
  }>
  | BriefingEventBase<'BriefingItemHandled', {
    briefingId: string;
    itemId: string;
    entityId: string;
  }>
  | BriefingEventBase<'BriefingDismissed', {
    briefingId: string;
  }>
  | BriefingEventBase<'AttentionSnapshotUpdated', {
    snapshot: AttentionSnapshot;
  }>
  | BriefingEventBase<'QuietItemAdded', {
    messageId: string;
    category: string;
    reason: string;
  }>
  | BriefingEventBase<'QuietItemRestored', {
    messageId: string;
  }>
  | BriefingEventBase<'BriefingScheduleUpdated', {
    entries: readonly BriefingScheduleEntry[];
  }>;

export type BriefingEventType = BriefingEvent['type'];

export const BRIEFING_EVENT_TYPES: readonly BriefingEventType[] = Object.freeze([
  'BriefingScheduled',
  'BriefingGenerationStarted',
  'BriefingGenerated',
  'BriefingOpened',
  'BriefingItemHandled',
  'BriefingDismissed',
  'AttentionSnapshotUpdated',
  'QuietItemAdded',
  'QuietItemRestored',
  'BriefingScheduleUpdated',
]);
