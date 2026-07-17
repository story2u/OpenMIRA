import type { SyncStoreExecutor } from '../sync/syncStore';

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function requireOwnerId(ownerId: string) {
  if (!uuidPattern.test(ownerId)) throw new Error('invalid capability owner');
}

export async function readStoredSyncCapability(
  database: SyncStoreExecutor,
  ownerId: string,
) {
  requireOwnerId(ownerId);
  const row = await database.getFirstAsync<{ sync_available: number }>(
    'SELECT sync_available FROM client_capability_state WHERE owner_id = ?',
    ownerId,
  );
  return row?.sync_available === 1;
}

export async function writeStoredSyncCapability(
  database: SyncStoreExecutor,
  ownerId: string,
  available: boolean,
) {
  requireOwnerId(ownerId);
  await database.runAsync(
    `INSERT INTO client_capability_state (owner_id, sync_available, updated_at)
     VALUES (?, ?, ?)
     ON CONFLICT(owner_id) DO UPDATE SET
       sync_available = excluded.sync_available,
       updated_at = excluded.updated_at`,
    ownerId,
    available ? 1 : 0,
    new Date().toISOString(),
  );
}
