import {
  acknowledgeSync,
  readSyncBootstrap,
  readSyncChanges,
  updateOpportunityStatus,
} from '../api/client';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import { randomUUID } from 'expo-crypto';
import { initializeRadarDatabase } from '../storage/database';
import {
  drainInternalCommandOutbox,
  dismissTerminalCommand,
  enqueueOpportunityStatusCommand,
  readCommandOutboxSummary,
  type DrainCommandResult,
} from './commandOutbox';
import {
  LocalSyncStateError,
  markLocalSyncError,
  readLocalSyncState,
} from './syncStore';
import { synchronizeForCursorHint } from './cursorHint';
import {
  runOwnerSync,
  type SyncRunResult,
  type SyncTransport,
} from './syncEngine';

function transportFor(baseUrl: string): SyncTransport {
  return {
    acknowledge: (input, signal) => acknowledgeSync(baseUrl, input, signal),
    bootstrap: (query, signal) => readSyncBootstrap(baseUrl, query, signal),
    changes: (query, signal) => readSyncChanges(baseUrl, query, signal),
  };
}

export interface InstalledSyncResult {
  commands: DrainCommandResult;
  sync: SyncRunResult;
}

const inFlight = new Map<string, Promise<InstalledSyncResult>>();

const commandLifetimeMilliseconds = 7 * 24 * 60 * 60 * 1_000;

export async function queueInstalledOpportunityStatus(
  ownerId: string,
  opportunityId: string,
  status: InternalOpportunityStatus,
) {
  const database = await initializeRadarDatabase();
  const commandId = randomUUID();
  await enqueueOpportunityStatusCommand(database, {
    ownerId,
    opportunityId,
    status,
    commandId,
    idempotencyKey: `status-${commandId}`,
    expiresAt: new Date(Date.now() + commandLifetimeMilliseconds).toISOString(),
  });
  return readCommandOutboxSummary(database, ownerId);
}

export async function readInstalledCommandSummary(ownerId: string) {
  return readCommandOutboxSummary(await initializeRadarDatabase(), ownerId);
}

export async function dismissInstalledTerminalCommand(ownerId: string, commandId: string) {
  return dismissTerminalCommand(await initializeRadarDatabase(), ownerId, commandId);
}

export async function synchronizeInstalledOwner(
  baseUrl: string,
  ownerId: string,
  signal?: AbortSignal,
) {
  const key = `${baseUrl}\0${ownerId}`;
  const existing = inFlight.get(key);
  if (existing) return existing;
  const promise = initializeRadarDatabase()
    .then(async (database) => {
      try {
        let sync = await runOwnerSync(database, transportFor(baseUrl), ownerId, signal);
        const commands = await drainInternalCommandOutbox(
          database,
          {
            updateOpportunityStatus: (command, commandSignal) => updateOpportunityStatus(
              baseUrl,
              command.opportunityId,
              command.status,
              {
                expectedVersion: command.expectedVersion,
                idempotencyKey: command.idempotencyKey,
                signal: commandSignal,
              },
            ),
          },
          ownerId,
          signal,
        );
        if (commands.succeededCount > 0) {
          sync = await runOwnerSync(database, transportFor(baseUrl), ownerId, signal);
        }
        return { commands, sync };
      } catch (error) {
        if (error instanceof LocalSyncStateError && !signal?.aborted) {
          await markLocalSyncError(database, ownerId, 'apply_failed');
        }
        throw error;
      }
    })
    .finally(() => {
      if (inFlight.get(key) === promise) inFlight.delete(key);
    });
  inFlight.set(key, promise);
  return promise;
}

export async function synchronizeInstalledOwnerForCursorHint(
  baseUrl: string,
  ownerId: string,
  cursor: number,
) {
  const database = await initializeRadarDatabase();
  return synchronizeForCursorHint(cursor, {
    readLocalState: () => readLocalSyncState(database, ownerId),
    synchronize: () => synchronizeInstalledOwner(baseUrl, ownerId),
  });
}
