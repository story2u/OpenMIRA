import { RadarApiError } from '@story2u/radar-api/client';

export function isOfflineReadFailure(error: unknown) {
  if (error instanceof Error && error.name === 'AbortError') return false;
  if (error instanceof RadarApiError) return error.status >= 500;
  return error instanceof TypeError;
}

export async function readWithOfflineFallback<T>(
  enabled: boolean,
  readOnline: () => Promise<T>,
  readOffline: () => Promise<T>,
): Promise<T> {
  try {
    return await readOnline();
  } catch (error) {
    if (!enabled || !isOfflineReadFailure(error)) throw error;
    return readOffline();
  }
}
