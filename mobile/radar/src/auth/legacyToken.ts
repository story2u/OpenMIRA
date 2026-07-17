import type { LegacyTokenStore } from './tokenMigrationCore';

// TypeScript/web fallback. Metro resolves the platform-specific implementation on native.
export const legacyTokenStore: LegacyTokenStore = {
  read: async () => null,
  clear: async () => undefined,
};
