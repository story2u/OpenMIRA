export type TokenMigrationStatus = 'already-migrated' | 'migrated' | 'no-legacy-token';

export interface TokenReader {
  read(): Promise<string | null>;
}

export interface LegacyTokenStore extends TokenReader {
  clear(): Promise<void>;
}

export interface CurrentTokenStore extends TokenReader {
  write(token: string): Promise<void>;
}

export interface TokenMigrationDependencies {
  current: CurrentTokenStore;
  legacy: LegacyTokenStore;
}

/**
 * Copies a legacy token into the new vault without creating a logout window.
 * The legacy value is deleted only after the new vault returns the exact value.
 */
export async function migrateLegacyToken({
  current,
  legacy,
}: TokenMigrationDependencies): Promise<TokenMigrationStatus> {
  if (await current.read()) return 'already-migrated';

  const token = await legacy.read();
  if (!token) return 'no-legacy-token';

  await current.write(token);
  if ((await current.read()) !== token) {
    throw new Error('New token vault did not return the migrated token');
  }

  await legacy.clear();
  return 'migrated';
}
