export interface SyncChange {
  sequence: number;
  entityId: string;
  operation: 'upsert' | 'delete';
  title?: string;
}

export interface OpportunityProjection {
  entityId: string;
  sequence: number;
  title: string;
}

export function foldChanges(changes: readonly SyncChange[]): Map<string, OpportunityProjection> {
  const projection = new Map<string, OpportunityProjection>();
  const versions = new Map<string, number>();

  for (const change of [...changes].sort((left, right) => left.sequence - right.sequence)) {
    const version = versions.get(change.entityId);
    if (version !== undefined && version >= change.sequence) continue;
    versions.set(change.entityId, change.sequence);

    if (change.operation === 'delete') {
      projection.delete(change.entityId);
    } else {
      projection.set(change.entityId, {
        entityId: change.entityId,
        sequence: change.sequence,
        title: change.title ?? '',
      });
    }
  }

  return projection;
}
