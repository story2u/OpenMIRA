import { openDatabaseAsync, type SQLiteDatabase } from 'expo-sqlite';

const DATABASE_NAME = 'radar-p0-recovery.db';

async function migrate(db: SQLiteDatabase) {
  await db.execAsync(`
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS sync_changes (
      sequence INTEGER PRIMARY KEY NOT NULL,
      entity_id TEXT NOT NULL,
      title TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS opportunity_projection (
      entity_id TEXT PRIMARY KEY NOT NULL,
      sequence INTEGER NOT NULL,
      title TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS sync_meta (
      singleton INTEGER PRIMARY KEY NOT NULL CHECK (singleton = 1),
      phase TEXT NOT NULL,
      last_sequence INTEGER NOT NULL
    );
  `);
}

async function replayIfInterrupted(db: SQLiteDatabase) {
  const state = await db.getFirstAsync<{ phase: string }>(
    'SELECT phase FROM sync_meta WHERE singleton = 1',
  );
  if (state?.phase !== 'applying') return;

  await db.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync('DELETE FROM opportunity_projection');
    await transaction.execAsync(`
      INSERT INTO opportunity_projection (entity_id, sequence, title)
      SELECT change.entity_id, change.sequence, change.title
      FROM sync_changes AS change
      INNER JOIN (
        SELECT entity_id, MAX(sequence) AS sequence
        FROM sync_changes
        GROUP BY entity_id
      ) AS latest
        ON latest.entity_id = change.entity_id
        AND latest.sequence = change.sequence;
    `);
    await transaction.execAsync(`
      UPDATE sync_meta
      SET phase = 'ready', last_sequence = COALESCE((SELECT MAX(sequence) FROM sync_changes), 0)
      WHERE singleton = 1;
    `);
  });
}

export interface SQLiteRecoveryResult {
  changeCount: number;
  projectedCount: number;
  phase: string;
  lastSequence: number;
}

/**
 * Persists an interrupted projection, closes the database, then reopens and replays the change log.
 * Closing between phases provides deterministic crash-recovery coverage without killing the host app.
 */
export async function runSQLiteRecoverySpike(changeCount = 10_000): Promise<SQLiteRecoveryResult> {
  let db = await openDatabaseAsync(DATABASE_NAME);
  await migrate(db);
  await db.execAsync('DELETE FROM sync_changes; DELETE FROM opportunity_projection; DELETE FROM sync_meta;');

  const interruptedAt = Math.floor(changeCount / 2);
  await db.withExclusiveTransactionAsync(async (transaction) => {
    await transaction.runAsync(
      "INSERT INTO sync_meta (singleton, phase, last_sequence) VALUES (1, 'applying', 0)",
    );
    for (let sequence = 1; sequence <= changeCount; sequence += 1) {
      const entityId = `opportunity-${sequence}`;
      await transaction.runAsync(
        'INSERT INTO sync_changes (sequence, entity_id, title) VALUES (?, ?, ?)',
        sequence,
        entityId,
        `Opportunity ${sequence}`,
      );
      if (sequence <= interruptedAt) {
        await transaction.runAsync(
          'INSERT INTO opportunity_projection (entity_id, sequence, title) VALUES (?, ?, ?)',
          entityId,
          sequence,
          `Opportunity ${sequence}`,
        );
      }
    }
    await transaction.runAsync(
      'UPDATE sync_meta SET last_sequence = ? WHERE singleton = 1',
      interruptedAt,
    );
  });
  await db.closeAsync();

  db = await openDatabaseAsync(DATABASE_NAME);
  await migrate(db);
  await replayIfInterrupted(db);
  const counts = await db.getFirstAsync<{ count: number }>(
    'SELECT COUNT(*) AS count FROM opportunity_projection',
  );
  const state = await db.getFirstAsync<{ phase: string; last_sequence: number }>(
    'SELECT phase, last_sequence FROM sync_meta WHERE singleton = 1',
  );
  await db.closeAsync();

  return {
    changeCount,
    projectedCount: counts?.count ?? 0,
    phase: state?.phase ?? 'missing',
    lastSequence: state?.last_sequence ?? 0,
  };
}
