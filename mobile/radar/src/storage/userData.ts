import type { SyncStoreDatabase } from '../sync/syncStore';

export async function clearLocalUserDataInDatabase(
  database: SyncStoreDatabase,
  ownerId: string,
) {
  if (!ownerId) throw new Error('ownerId is required to clear local data');
  await database.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync('DELETE FROM briefing_items WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM briefings WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM briefing_schedules WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM briefing_events WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM signal_appetite_ui_state WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM temporary_focuses WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM shadow_evaluations WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM message_filter_decisions WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM preference_examples WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM teaching_sessions WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM attention_intents WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM attention_preferences WHERE owner_id = ?', ownerId);
    await transaction.runAsync('DELETE FROM attention_events WHERE owner_id = ?', ownerId);
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
