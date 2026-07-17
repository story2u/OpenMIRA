import { describe, expect, it, vi } from 'vitest';

import { synchronizeForCursorHint } from './cursorHint';

describe('cursor hint sync policy', () => {
  it('skips duplicate and delayed hints after the durable cursor is current', async () => {
    const synchronize = vi.fn(async () => 'unexpected');

    await expect(synchronizeForCursorHint(99, {
      readLocalState: async () => ({ cursor: 100, phase: 'ready', lastErrorCode: null }),
      synchronize,
    })).resolves.toEqual({ status: 'already-current', cursor: 100 });
    expect(synchronize).not.toHaveBeenCalled();
  });

  it.each([
    null,
    { cursor: 99, phase: 'ready' as const, lastErrorCode: null },
    { cursor: 100, phase: 'bootstrapping' as const, lastErrorCode: null },
    { cursor: 100, phase: 'error' as const, lastErrorCode: 'apply_failed' },
  ])('synchronizes an ahead hint or a projection that needs recovery: %j', async (state) => {
    const synchronize = vi.fn(async () => ({ cursor: 100 }));

    await expect(synchronizeForCursorHint(100, {
      readLocalState: async () => state,
      synchronize,
    })).resolves.toEqual({
      status: 'synchronized',
      result: { cursor: 100 },
    });
    expect(synchronize).toHaveBeenCalledOnce();
  });

  it('uses normal synchronization recovery when reading local state fails', async () => {
    const synchronize = vi.fn(async () => 'recovered');

    await expect(synchronizeForCursorHint(100, {
      readLocalState: async () => {
        throw new Error('database unavailable');
      },
      synchronize,
    })).resolves.toEqual({ status: 'synchronized', result: 'recovered' });
  });

  it.each([0, -1, Number.MAX_SAFE_INTEGER + 1, 1.5])(
    'rejects an invalid cursor without reading or synchronizing: %s',
    async (cursor) => {
      const readLocalState = vi.fn(async () => null);
      const synchronize = vi.fn(async () => 'unexpected');

      await expect(synchronizeForCursorHint(cursor, {
        readLocalState,
        synchronize,
      })).rejects.toThrow('positive safe integer');
      expect(readLocalState).not.toHaveBeenCalled();
      expect(synchronize).not.toHaveBeenCalled();
    },
  );
});
