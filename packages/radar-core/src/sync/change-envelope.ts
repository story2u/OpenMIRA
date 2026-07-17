export type SyncOperation = 'upsert' | 'delete';

export interface SyncChangeEnvelope<Payload = unknown> {
  eventId: string;
  cursor: number;
  aggregateType: string;
  aggregateId: string;
  aggregateVersion: number;
  operation: SyncOperation;
  schemaVersion: number;
  createdAt: string;
  payload: Payload | null;
}

export function changeIdentity(change: Pick<SyncChangeEnvelope, 'aggregateType' | 'aggregateId'>) {
  return `${change.aggregateType}:${change.aggregateId}`;
}
