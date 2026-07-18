import type {
  SignalAppetiteEvent,
  SignalAppetiteEventsAppendRequest,
  SignalAppetiteEventsPage,
  SignalAppetiteEventsQuery,
  SignalAppetiteEventWrite,
} from '@story2u/radar-contracts/signal-appetite-sync';

import {
  ingestSyncedSignalAppetiteEvent,
  markSignalAppetiteEventsSynced,
  readPendingSignalAppetiteEvents,
  type SignalAppetiteStoreDatabase,
  type SignalAppetiteStoreExecutor,
} from '../attention/signalAppetiteStore';

const stream = 'signal_appetite';
const pageSize = 500;

export interface SignalAppetiteSyncTransport {
  append(input: SignalAppetiteEventsAppendRequest, signal?: AbortSignal): Promise<{
    events: SignalAppetiteEvent[];
    serverCursor: number;
  }>;
  list(query: SignalAppetiteEventsQuery, signal?: AbortSignal): Promise<SignalAppetiteEventsPage>;
}

function requireNotAborted(signal?: AbortSignal) {
  if (signal?.aborted) throw new DOMException('Sync aborted', 'AbortError');
}

async function readCursor(database: SignalAppetiteStoreExecutor, ownerId: string) {
  const row = await database.getFirstAsync<{ cursor: number }>(
    'SELECT cursor FROM sync_state WHERE owner_id = ? AND stream = ?',
    ownerId,
    stream,
  );
  return row?.cursor ?? 0;
}

async function writeCursor(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
  cursor: number,
) {
  await database.runAsync(
    `INSERT INTO sync_state (owner_id, stream, cursor, updated_at, phase, last_error_code)
     VALUES (?, ?, ?, ?, 'ready', NULL)
     ON CONFLICT(owner_id, stream) DO UPDATE SET cursor = excluded.cursor,
       updated_at = excluded.updated_at, phase = 'ready', last_error_code = NULL`,
    ownerId,
    stream,
    cursor,
    new Date().toISOString(),
  );
}

export interface SignalAppetiteSyncResult {
  pushed: number;
  pulled: number;
  cursor: number;
}

export async function runSignalAppetiteSync(
  database: SignalAppetiteStoreDatabase,
  transport: SignalAppetiteSyncTransport,
  ownerId: string,
  signal?: AbortSignal,
): Promise<SignalAppetiteSyncResult> {
  requireNotAborted(signal);
  let pushed = 0;
  while (true) {
    const pending = await readPendingSignalAppetiteEvents(database, ownerId, 100);
    if (!pending.length) break;
    const response = await transport.append({
      events: pending.map((event) => ({
        eventId: event.eventId,
        eventType: event.type,
        aggregateId: event.aggregateId,
        aggregateVersion: event.aggregateVersion,
        schemaVersion: 1,
        occurredAt: event.occurredAt,
        payload: event.payload as unknown as SignalAppetiteEventWrite['payload'],
      })),
    }, signal);
    requireNotAborted(signal);
    await markSignalAppetiteEventsSynced(
      database,
      ownerId,
      new Map(response.events.map((event) => [event.eventId, event.cursor])),
    );
    pushed += pending.length;
  }

  let cursor = await readCursor(database, ownerId);
  let pulled = 0;
  while (true) {
    requireNotAborted(signal);
    const page = await transport.list({ after: cursor, limit: pageSize }, signal);
    requireNotAborted(signal);
    for (const event of page.events) {
      if (event.ownerId.toLowerCase() !== ownerId.toLowerCase()) {
        throw new Error('Signal Appetite event owner mismatch');
      }
      await ingestSyncedSignalAppetiteEvent(database, {
        ...event,
        type: event.eventType,
      });
    }
    await writeCursor(database, ownerId, page.nextCursor);
    cursor = page.nextCursor;
    pulled += page.events.length;
    if (!page.hasMore) break;
  }
  return { pushed, pulled, cursor };
}
