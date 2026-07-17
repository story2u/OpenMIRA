import { describe, expect, it, vi } from 'vitest';

import { migrateLegacyToken } from './tokenMigrationCore';

function stores(currentValue: string | null, legacyValue: string | null) {
  let current = currentValue;
  let legacy = legacyValue;
  const clear = vi.fn(async () => {
    legacy = null;
  });
  const write = vi.fn(async (value: string) => {
    current = value;
  });

  return {
    dependencies: {
      current: { read: async () => current, write },
      legacy: { read: async () => legacy, clear },
    },
    clear,
    write,
  };
}

describe('migrateLegacyToken', () => {
  it('copies, verifies, and only then clears a legacy token', async () => {
    const fixture = stores(null, 'legacy-token');

    await expect(migrateLegacyToken(fixture.dependencies)).resolves.toBe('migrated');
    expect(fixture.write).toHaveBeenCalledWith('legacy-token');
    expect(fixture.clear).toHaveBeenCalledOnce();
  });

  it('does not touch the legacy store when a current token exists', async () => {
    const fixture = stores('current-token', 'legacy-token');

    await expect(migrateLegacyToken(fixture.dependencies)).resolves.toBe('already-migrated');
    expect(fixture.write).not.toHaveBeenCalled();
    expect(fixture.clear).not.toHaveBeenCalled();
  });

  it('does nothing on a fresh install', async () => {
    const fixture = stores(null, null);

    await expect(migrateLegacyToken(fixture.dependencies)).resolves.toBe('no-legacy-token');
    expect(fixture.write).not.toHaveBeenCalled();
    expect(fixture.clear).not.toHaveBeenCalled();
  });

  it('preserves the legacy token when write verification fails', async () => {
    const clear = vi.fn(async () => undefined);
    const dependencies = {
      current: { read: async () => null, write: async () => undefined },
      legacy: { read: async () => 'legacy-token', clear },
    };

    await expect(migrateLegacyToken(dependencies)).rejects.toThrow('did not return');
    expect(clear).not.toHaveBeenCalled();
  });
});
