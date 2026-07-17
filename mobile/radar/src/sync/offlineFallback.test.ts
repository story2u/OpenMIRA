import { RadarApiError } from '@story2u/radar-api/client';
import { describe, expect, it, vi } from 'vitest';

import { readWithOfflineFallback } from './offlineFallback';

describe('readWithOfflineFallback', () => {
  it('uses the local projection only for enabled network or server failures', async () => {
    const local = vi.fn(async () => 'local');

    await expect(readWithOfflineFallback(true, async () => {
      throw new TypeError('network unavailable');
    }, local)).resolves.toBe('local');
    await expect(readWithOfflineFallback(true, async () => {
      throw new RadarApiError('unavailable', 503, null);
    }, local)).resolves.toBe('local');
    expect(local).toHaveBeenCalledTimes(2);
  });

  it('fails closed for disabled rollout, auth, validation, schema and abort errors', async () => {
    const local = vi.fn(async () => 'local');
    const failures = [
      new TypeError('network unavailable'),
      new RadarApiError('unauthorized', 401, null),
      new RadarApiError('invalid', 422, null),
      new Error('strict schema mismatch'),
      Object.assign(new Error('cancelled'), { name: 'AbortError' }),
    ];

    await expect(readWithOfflineFallback(false, async () => {
      throw failures[0];
    }, local)).rejects.toBe(failures[0]);
    for (const failure of failures.slice(1)) {
      await expect(readWithOfflineFallback(true, async () => {
        throw failure;
      }, local)).rejects.toBe(failure);
    }
    expect(local).not.toHaveBeenCalled();
  });
});
