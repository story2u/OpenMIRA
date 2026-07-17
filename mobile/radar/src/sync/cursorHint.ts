import type { LocalSyncState } from './syncStore';

export type CursorHintSyncResult<Result> =
  | { status: 'already-current'; cursor: number }
  | { status: 'synchronized'; result: Result };

export interface CursorHintSyncDependencies<Result> {
  readLocalState(): Promise<LocalSyncState | null>;
  synchronize(): Promise<Result>;
}

function requireCursor(cursor: number) {
  if (!Number.isSafeInteger(cursor) || cursor <= 0) {
    throw new Error('Push cursor hint must be a positive safe integer.');
  }
}

export async function synchronizeForCursorHint<Result>(
  cursor: number,
  dependencies: CursorHintSyncDependencies<Result>,
): Promise<CursorHintSyncResult<Result>> {
  requireCursor(cursor);
  let state: LocalSyncState | null;
  try {
    state = await dependencies.readLocalState();
  } catch {
    // A malformed or unavailable projection must enter the normal recovery path.
    return { status: 'synchronized', result: await dependencies.synchronize() };
  }
  if (state?.phase === 'ready' && state.cursor >= cursor) {
    return { status: 'already-current', cursor: state.cursor };
  }
  return { status: 'synchronized', result: await dependencies.synchronize() };
}
