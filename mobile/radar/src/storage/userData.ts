import type { SyncStoreDatabase } from '../sync/syncStore';

export async function clearLocalUserDataInDatabase(
  database: SyncStoreDatabase,
  ownerId: string,
) {
  if (!ownerId) throw new Error('ownerId is required to clear local data');
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync('DELETE FROM agent_sessions WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM analysis_run_state WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM command_outbox WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM message_projection WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM setting_projection WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM opportunity_projection WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM change_inbox WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM sync_bootstrap_state WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM client_capability_state WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM sync_state WHERE owner_id = ?', ownerId);
  });
}
