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

export async function persistSessionToken(store: SessionTokenStore, token: string) {
  if (token.length < 16 || token.length > 16_384 || /[\r\n]/.test(token)) {
    throw new Error('Invalid access token');
  }
  await store.write(token);
  if ((await store.read()) !== token) {
    await store.clear();
    throw new Error('Secure token write could not be verified');
  }
}

export async function persistReplacingLegacyToken(
  current: SessionTokenStore,
  legacy: ClearableTokenStore,
  token: string,
) {
  // Retire the old credential before accepting the replacement. If this fails,
  // the new token is never written and cannot appear after a failed login.
  await legacy.clear();
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
