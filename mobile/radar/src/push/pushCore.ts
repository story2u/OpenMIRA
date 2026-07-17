import type {
  PushEnvironment,
  PushProvider,
  PushRegistrationRequest,
} from '@story2u/radar-contracts/devices';

export type PushEnrollmentState = 'disabled' | 'registering' | 'active' | 'denied' | 'error';

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

export function extractCursorHint(value: unknown): number | null {
  const candidate = record(value);
  if (!candidate) return null;
  if (typeof candidate.dataString === 'string') {
    try {
      const parsed = extractCursorHint(JSON.parse(candidate.dataString));
      if (parsed !== null) return parsed;
    } catch {
      return null;
    }
  }
  for (const nested of ['radar', 'data', 'notification']) {
    const parsed = extractCursorHint(candidate[nested]);
    if (parsed !== null) return parsed;
  }
  if (candidate.type !== 'sync_cursor' || String(candidate.schemaVersion) !== '1') return null;
  const cursor = typeof candidate.cursor === 'string'
    ? Number(candidate.cursor)
    : candidate.cursor;
  return typeof cursor === 'number' && Number.isSafeInteger(cursor) && cursor > 0
    ? cursor
    : null;
}

export function buildPushRegistration(
  platform: 'ios' | 'android',
  development: boolean,
  tokenType: string,
  token: unknown,
): PushRegistrationRequest {
  const provider: PushProvider = platform === 'ios' ? 'apns' : 'fcm';
  const environment: PushEnvironment = development ? 'sandbox' : 'production';
  if (tokenType !== platform || typeof token !== 'string' || token.length < 16) {
    throw new Error('Native push token does not match the current platform.');
  }
  return { provider, environment, token };
}
