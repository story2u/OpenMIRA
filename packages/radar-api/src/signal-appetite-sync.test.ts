import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createSignalAppetiteSyncApi,
  decodeSignalAppetiteEventsPage,
} from './signal-appetite-sync';

const event = {
  eventId: '01234567-89ab-cdef-0123-456789abcdef',
  eventType: 'TeachingSessionStarted' as const,
  aggregateId: '11234567-89ab-cdef-0123-456789abcdef',
  aggregateVersion: 1,
  schemaVersion: 1 as const,
  occurredAt: '2026-07-18T12:00:00Z',
  payload: { sessionId: '11234567-89ab-cdef-0123-456789abcdef', targetCount: 8 },
};
const synced = {
  ...event,
  ownerId: '21234567-89ab-cdef-0123-456789abcdef',
  deviceId: '31234567-89ab-cdef-0123-456789abcdef',
  cursor: 1,
  serverReceivedAt: '2026-07-18T12:00:01Z',
};

describe('Signal Appetite sync API', () => {
  it('appends bounded content-free events', async () => {
    const fetch = vi.fn(async () => new Response(JSON.stringify({
      events: [synced], serverCursor: 1,
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    const api = createSignalAppetiteSyncApi(createRadarApiClient({
      baseUrl: 'https://api.example.test', fetch, getAccessToken: () => 'token',
    }));

    await expect(api.append({ events: [event] })).resolves.toMatchObject({ serverCursor: 1 });
    expect(fetch).toHaveBeenCalledWith(
      'https://api.example.test/api/v1/sync/signal-appetite/events',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(() => api.append({
      events: [{ ...event, payload: { messageBody: 'private' } }],
    })).toThrow('forbidden');
  });

  it('rejects unordered pages and invalid cursor metadata', () => {
    const page = { events: [synced], nextCursor: 1, serverCursor: 1, hasMore: false };
    expect(decodeSignalAppetiteEventsPage(page)).toEqual(page);
    expect(() => decodeSignalAppetiteEventsPage({
      ...page,
      events: [{ ...synced, cursor: 2 }],
    })).toThrow();
    expect(() => decodeSignalAppetiteEventsPage({
      ...page,
      events: [synced, synced],
    })).toThrow();
  });
});
