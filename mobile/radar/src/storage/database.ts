import { openDatabaseAsync, type SQLiteDatabase } from 'expo-sqlite';

import { analysisRunTokenStore } from '../agent/runTokenStorage';
import { runRadarMigrations } from './migrations';
import { clearLocalUserDataInDatabase } from './userData';

const databaseName = 'radar.db';
let databasePromise: Promise<SQLiteDatabase> | null = null;

export async function initializeRadarDatabase() {
  if (!databasePromise) {
    databasePromise = openDatabaseAsync(databaseName)
      .then(async (database) => {
        await runRadarMigrations(database);
        return database;
      })
      .catch((error) => {
        databasePromise = null;
        throw error;
      });
  }
  return databasePromise;
}

export async function clearLocalUserData(ownerId: string) {
  if (!ownerId) throw new Error('ownerId is required to clear local data');
  const database = await initializeRadarDatabase();
  const analysisRuns = await database.getAllAsync<{ run_id: string }>(
    'SELECT run_id FROM analysis_run_state WHERE owner_id = ?',
    ownerId,
  );
  await clearLocalUserDataInDatabase(database, ownerId);
  const cleared = await Promise.allSettled(
    analysisRuns.map((run) => analysisRunTokenStore.clear(run.run_id)),
  );
  if (cleared.some((result) => result.status === 'rejected')) {
    throw new Error('analysis run tokens could not be cleared');
  }
}
