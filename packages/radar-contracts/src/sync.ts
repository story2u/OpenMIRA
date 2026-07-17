import type { OpportunityDetail } from './opportunities';
import type { components } from './openapi';
import type {
  DetectionSettings,
  NotificationSettings,
  WorkSchedule,
} from './settings';
import type { ReplyTemplate } from './templates';

export type SyncAggregateType = components['schemas']['SyncAggregateType'];
export type SyncOperation = components['schemas']['SyncOperation'];
export type SyncAckRequest = components['schemas']['SyncAckRequest'];
export type SyncAck = components['schemas']['SyncAckRead'];

export interface SyncMessagePayload {
  id: string;
  opportunityId: string | null;
  senderName: string;
  content: string;
  isFromContact: boolean;
  sentAt: string;
  source: components['schemas']['MessageSource'] | null;
}

export type SyncPayload =
  | OpportunityDetail
  | SyncMessagePayload
  | DetectionSettings
  | WorkSchedule
  | NotificationSettings
  | ReplyTemplate;

export interface SyncSnapshotItem<Payload extends SyncPayload = SyncPayload> {
  aggregateType: SyncAggregateType;
  aggregateId: string;
  aggregateVersion: number;
  schemaVersion: 1;
  payload: Payload;
}

export interface SyncBootstrap {
  watermarkCursor: number;
  items: SyncSnapshotItem[];
  nextPageToken: string | null;
  hasMore: boolean;
}

interface SyncChangeBase {
  eventId: string;
  cursor: number;
  aggregateType: SyncAggregateType;
  aggregateId: string;
  aggregateVersion: number;
  schemaVersion: 1;
  createdAt: string;
}

export type SyncChange =
  | (SyncChangeBase & { operation: 'upsert'; payload: SyncPayload })
  | (SyncChangeBase & { operation: 'delete'; payload: null });

export interface SyncChanges {
  changes: SyncChange[];
  nextCursor: number;
  serverCursor: number;
  hasMore: boolean;
  resetRequired: boolean;
  resetReason: 'cursor_expired' | 'cursor_ahead' | null;
}

export interface SyncBootstrapQuery {
  limit?: number;
  pageToken?: string;
}

export interface SyncChangesQuery {
  after: number;
  limit?: number;
}
