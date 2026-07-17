import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createSyncApi,
  decodeSyncBootstrap,
  decodeSyncChanges,
  syncBootstrapPath,
  syncChangesPath,
} from './sync';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const messageId = '11234567-89ab-cdef-0123-456789abcdef';
const eventId = '21234567-89ab-cdef-0123-456789abcdef';
const deviceId = '31234567-89ab-cdef-0123-456789abcdef';

const notificationItem = {
  aggregateType: 'user_notification_preference',
  aggregateId: ownerId,
  aggregateVersion: 0,
  schemaVersion: 1,
  payload: {
    newOpportunityEnabled: true,
    aiRepliedEnabled: true,
    dailyDigestEnabled: false,
    urgentOnly: false,
  },
};

const messagePayload = {
  id: messageId,
  opportunityId: null,
  senderName: 'Customer',
  content: 'Need a quote',
  isFromContact: true,
  sentAt: '2026-07-17T10:00:00Z',
  source: null,
};

const messageChange = {
  eventId,
  cursor: 7,
  aggregateType: 'message',
  aggregateId: messageId,
  aggregateVersion: 1,
  operation: 'upsert',
  schemaVersion: 1,
  createdAt: '2026-07-17T10:00:01Z',
  payload: messagePayload,
};

function fixture(body: unknown) {
  const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(body));
  return {
    api: createSyncApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'device-bound-access-token',
    })),
    fetch,
  };
}

describe('sync API', () => {
  it('builds bounded bootstrap and changes paths', () => {
    expect(syncBootstrapPath({ limit: 50, pageToken: 'aaa.bbb.ccc' })).toBe(
      '/api/v1/sync/bootstrap?limit=50&pageToken=aaa.bbb.ccc',
    );
    expect(syncChangesPath({ after: 42, limit: 100 })).toBe(
      '/api/v1/sync/changes?after=42&limit=100',
    );
    expect(() => syncBootstrapPath({ limit: 501 })).toThrow('limit');
    expect(() => syncBootstrapPath({ pageToken: 'unsafe token' })).toThrow('page token');
    expect(() => syncChangesPath({ after: -1 })).toThrow('cursor');
  });

  it('strictly decodes ordered bootstrap pages and continuation state', async () => {
    const body = {
      watermarkCursor: 7,
      items: [notificationItem, {
        aggregateType: 'message',
        aggregateId: messageId,
        aggregateVersion: 1,
        schemaVersion: 1,
        payload: messagePayload,
      }],
      nextPageToken: null,
      hasMore: false,
    };
    await expect(fixture(body).api.bootstrap()).resolves.toMatchObject({ watermarkCursor: 7 });

    expect(() => decodeSyncBootstrap({ ...body, hasMore: true })).toThrow('continuation');
    expect(() => decodeSyncBootstrap({
      ...body,
      items: [body.items[1], body.items[0]],
    })).toThrow('ordered');
    expect(() => decodeSyncBootstrap({
      ...body,
      items: [{ ...body.items[1], schemaVersion: 2 }],
    })).toThrow();
    expect(() => decodeSyncBootstrap({
      ...body,
      items: [{
        ...body.items[1],
        payload: { ...messagePayload, rawPayload: { secret: true } },
      }],
    })).toThrow();
  });

  it('rejects duplicate, out-of-order and inconsistent change pages', async () => {
    const body = {
      changes: [messageChange],
      nextCursor: 7,
      serverCursor: 7,
      hasMore: false,
      resetRequired: false,
      resetReason: null,
    };
    await expect(fixture(body).api.changes({ after: 0 })).resolves.toMatchObject({
      nextCursor: 7,
    });
    expect(() => decodeSyncChanges({ ...body, nextCursor: 6 })).toThrow('next cursor');
    expect(() => decodeSyncChanges({
      ...body,
      changes: [messageChange, { ...messageChange, cursor: 8 }],
      nextCursor: 8,
      serverCursor: 8,
    })).toThrow('duplicate');
    expect(() => decodeSyncChanges({
      ...body,
      changes: [{ ...messageChange, schemaVersion: 2 }],
    })).toThrow();
  });

  it('accepts explicit reset pages and rejects mixed reset data', () => {
    const reset = {
      changes: [],
      nextCursor: 0,
      serverCursor: 7,
      hasMore: false,
      resetRequired: true,
      resetReason: 'cursor_expired',
    };
    expect(decodeSyncChanges(reset)).toMatchObject({ resetRequired: true });
    expect(() => decodeSyncChanges({ ...reset, changes: [messageChange] })).toThrow('reset');
    expect(() => decodeSyncChanges({
      ...reset,
      resetRequired: false,
    })).toThrow('reset reason');
  });

  it('sends only bounded acknowledgement metadata', async () => {
    const request = fixture({
      deviceId,
      acknowledgedCursor: 7,
      acknowledgedAt: '2026-07-17T10:00:02Z',
      errorCode: null,
    });
    await request.api.acknowledge({ cursor: 7, errorCode: 'projection.retry' });

    const [url, init] = request.fetch.mock.calls[0];
    expect(url).toBe('https://api.example.test/api/v1/sync/ack');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(String(init?.body))).toEqual({
      cursor: 7,
      errorCode: 'projection.retry',
    });
    expect(() => request.api.acknowledge({ cursor: 7, errorCode: 'Unsafe Error' })).toThrow();
    expect(request.fetch).toHaveBeenCalledTimes(1);
  });
});
