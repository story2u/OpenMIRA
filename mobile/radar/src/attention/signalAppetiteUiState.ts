import type { SignalAppetiteStoreExecutor } from './signalAppetiteStore';

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function owner(value: string) {
  if (!uuidPattern.test(value)) throw new Error('invalid_attention_ui_owner');
  return value.toLowerCase();
}

export async function hasSeenTeachingOnboarding(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
) {
  const row = await database.getFirstAsync<{ teaching_onboarding_seen: number }>(
    'SELECT teaching_onboarding_seen FROM signal_appetite_ui_state WHERE owner_id = ?',
    owner(ownerId),
  );
  return row?.teaching_onboarding_seen === 1;
}

export async function setTeachingOnboardingSeen(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  seen: boolean,
) {
  await database.runAsync(
    `INSERT INTO signal_appetite_ui_state (owner_id, teaching_onboarding_seen, updated_at)
     VALUES (?, ?, ?)
     ON CONFLICT(owner_id) DO UPDATE SET
       teaching_onboarding_seen = excluded.teaching_onboarding_seen,
       updated_at = excluded.updated_at`,
    owner(ownerId),
    seen ? 1 : 0,
    new Date().toISOString(),
  );
}
