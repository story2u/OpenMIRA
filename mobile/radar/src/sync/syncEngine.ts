import type {
  SyncAckRequest,
  SyncBootstrap,
  SyncBootstrapQuery,
  SyncChanges,
  SyncChangesQuery,
} from '@story2u/radar-contracts/sync';
import { RadarApiError } from '@story2u/radar-api/client';

import {
  applyBootstrapPage,
  applyChangePage,
  readLocalBootstrapState,
  readLocalSyncState,
  type SyncStoreDatabase,
} from './syncStore';

const pageSize = 500;
const maxPagesPerRun = 10_000;

export interface SyncTransport {
  acknowledge(input: SyncAckRequest, signal?: AbortSignal): Promise<unknown>;
  bootstrap(query: SyncBootstrapQuery, signal?: AbortSignal): Promise<SyncBootstrap>;
  changes(query: SyncChangesQuery, signal?: AbortSignal): Promise<SyncChanges>;
}

export interface SyncRunResult {
  acknowledged: boolean;
  bootstrapPages: number;
  changeCount: number;
  changePages: number;
  cursor: number;
  resetCount: number;
}

function requireNotAborted(signal?: AbortSignal) {
  if (signal?.aborted) throw new DOMException('Sync aborted', 'AbortError');
}

async function bootstrapOwner(
  database: SyncStoreDatabase,
  transport: SyncTransport,
  ownerId: string,
  signal: AbortSignal | undefined,
  resume: boolean,
) {
  let pageToken: string | undefined;
  let restart = true;
  if (resume) {
    const stored = await readLocalBootstrapState(database, ownerId);
    if (stored?.nextPageToken) {
      pageToken = stored.nextPageToken;
      restart = false;
    }
  }

  let pages = 0;
  while (pages < maxPagesPerRun) {
    requireNotAborted(signal);
    const page = await transport.bootstrap(
      { limit: pageSize, ...(pageToken ? { pageToken } : {}) },
      signal,
    );
    requireNotAborted(signal);
    await applyBootstrapPage(database, ownerId, page, { restart });
    pages += 1;
    restart = false;
    if (!page.hasMore) return pages;
    if (!page.nextPageToken) throw new Error('Sync bootstrap continuation is missing.');
    pageToken = page.nextPageToken;
  }
  throw new Error('Sync bootstrap page budget exceeded.');
}

export async function runOwnerSync(
  database: SyncStoreDatabase,
  transport: SyncTransport,
  ownerId: string,
  signal?: AbortSignal,
): Promise<SyncRunResult> {
  requireNotAborted(signal);
  let state = await readLocalSyncState(database, ownerId);
  let bootstrapPages = 0;
  let resetCount = 0;
  if (!state || state.phase !== 'ready') {
    const resume = state?.phase === 'bootstrapping';
    try {
      bootstrapPages += await bootstrapOwner(
        database,
        transport,
        ownerId,
        signal,
        resume,
      );
    } catch (error) {
      if (!resume || !(error instanceof RadarApiError) || error.status !== 422) throw error;
      bootstrapPages += await bootstrapOwner(
        database,
        transport,
        ownerId,
        signal,
        false,
      );
    }
    state = await readLocalSyncState(database, ownerId);
  }
  if (!state || state.phase !== 'ready') throw new Error('Sync bootstrap did not become ready.');

  let cursor = state.cursor;
  let changeCount = 0;
  let changePages = 0;
  while (changePages < maxPagesPerRun) {
    requireNotAborted(signal);
    const page = await transport.changes({ after: cursor, limit: pageSize }, signal);
    requireNotAborted(signal);
    if (page.resetRequired) {
      if (resetCount >= 1) throw new Error('Sync stream repeatedly requested reset.');
      resetCount += 1;
      bootstrapPages += await bootstrapOwner(database, transport, ownerId, signal, false);
      const resetState = await readLocalSyncState(database, ownerId);
      if (!resetState || resetState.phase !== 'ready') {
        throw new Error('Sync reset did not become ready.');
      }
      cursor = resetState.cursor;
      continue;
    }
    await applyChangePage(database, ownerId, cursor, page);
    cursor = page.nextCursor;
    changeCount += page.changes.length;
    changePages += 1;
    if (!page.hasMore) break;
  }
  if (changePages >= maxPagesPerRun) throw new Error('Sync change page budget exceeded.');

  let acknowledged = false;
  try {
    await transport.acknowledge({ cursor }, signal);
    acknowledged = true;
  } catch {
    // Ack is observability only. Durable local data stays ready and will be acked next run.
  }
  return { acknowledged, bootstrapPages, changeCount, changePages, cursor, resetCount };
}
