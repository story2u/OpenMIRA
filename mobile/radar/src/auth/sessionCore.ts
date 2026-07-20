export type SessionState<User> =
  | { status: 'anonymous' }
  | { status: 'authenticated'; user: User }
  | { status: 'requires-login'; reason: 'expired' | 'migration-failed' };

export interface SessionDependencies<User> {
  clearToken(): Promise<void>;
  isUnauthorized(error: unknown): boolean;
  loadUser(token: string): Promise<User>;
  migrateToken(): Promise<unknown>;
  readToken(): Promise<string | null>;
}

export interface SessionTokenStore {
  clear(): Promise<void>;
  read(): Promise<string | null>;
  write(token: string): Promise<void>;
}

export interface ClearableTokenStore {
  clear(): Promise<void>;
}

export type SessionPersistenceStage =
  | 'current-clear'
  | 'current-read'
  | 'current-write'
  | 'legacy-clear'
  | 'readback-mismatch'
  | 'token-validation';

export class SessionPersistenceError extends Error {
  constructor(
    readonly stage: SessionPersistenceStage,
    cause?: unknown,
  ) {
    super(`Session token persistence failed at ${stage}`);
    this.name = 'SessionPersistenceError';
    this.cause = cause;
  }
}

function persistenceError(stage: SessionPersistenceStage, cause?: unknown) {
  return new SessionPersistenceError(stage, cause);
}

export async function persistSessionToken(store: SessionTokenStore, token: string) {
  if (
    typeof token !== 'string'
    || token.length < 16
    || token.length > 16_384
    || /[\r\n]/.test(token)
  ) {
    throw persistenceError('token-validation');
  }
  try {
    await store.write(token);
  } catch (error) {
    throw persistenceError('current-write', error);
  }
  let stored: string | null;
  try {
    stored = await store.read();
  } catch (error) {
    throw persistenceError('current-read', error);
  }
  if (stored !== token) {
    try {
      await store.clear();
    } catch (error) {
      throw persistenceError('current-clear', error);
    }
    throw persistenceError('readback-mismatch');
  }
}

export async function persistReplacingLegacyToken(
  current: SessionTokenStore,
  legacy: ClearableTokenStore,
  token: string,
) {
  // Retire the old credential before accepting the replacement. If this fails,
  // the new token is never written and cannot appear after a failed login.
  try {
    await legacy.clear();
  } catch (error) {
    throw persistenceError('legacy-clear', error);
  }
  await persistSessionToken(current, token);
}

export async function clearSessionTokens(
  current: ClearableTokenStore,
  legacy: ClearableTokenStore,
  ...additional: readonly ClearableTokenStore[]
) {
  // Retire legacy first so a partial logout can never repopulate current on next bootstrap.
  await legacy.clear();
  await current.clear();
  for (const store of additional) await store.clear();
}

export async function endSession(dependencies: {
  clearLocalData(): Promise<void>;
  clearToken(): Promise<void>;
}) {
  await dependencies.clearToken();
  try {
    await dependencies.clearLocalData();
    return { localDataCleared: true } as const;
  } catch {
    return { localDataCleared: false } as const;
  }
}

export async function restoreSession<User>(
  dependencies: SessionDependencies<User>,
): Promise<SessionState<User>> {
  try {
    await dependencies.migrateToken();
  } catch {
    return { status: 'requires-login', reason: 'migration-failed' };
  }

  const token = await dependencies.readToken();
  if (!token) return { status: 'anonymous' };

  try {
    return { status: 'authenticated', user: await dependencies.loadUser(token) };
  } catch (error) {
    if (!dependencies.isUnauthorized(error)) throw error;
    await dependencies.clearToken();
    return { status: 'requires-login', reason: 'expired' };
  }
}
