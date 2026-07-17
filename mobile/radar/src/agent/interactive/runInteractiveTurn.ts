import type { InteractiveAgentTurn, InteractiveAgentTurnClaim } from '@story2u/radar-contracts/interactive-agent';
import { randomUUID } from 'expo-crypto';

import {
  claimInteractiveAgentTurn,
  completeInteractiveAgentTurn,
  decideInteractiveAgentAction,
  failInteractiveAgentTurn,
  heartbeatInteractiveAgentTurn,
  sendApprovedInteractiveAgentReply,
} from '../../api/client';
import type { SyncStoreDatabase } from '../../sync/syncStore';
import type { RequestInteractiveSendApproval } from './approvedSend';
import type { InteractiveApprovedSendDependencies } from './approvedSend';
import {
  appendAgentEntries,
  appendAgentEntry,
  readAgentEntries,
  type AgentEntryContent,
  type AgentSessionStoreDatabase,
} from './sessionStore';

const heartbeatIntervalMilliseconds = 45_000;

interface HostResult {
  entries: AgentEntryContent[];
  finalText: string;
}

interface RunHostOptions {
  baseUrl: string;
  approvedSendDependencies?: InteractiveApprovedSendDependencies;
  claim: InteractiveAgentTurnClaim;
  database: SyncStoreDatabase;
  entries: Awaited<ReturnType<typeof readAgentEntries>>;
  onStreamText?(text: string): void;
  ownerId: string;
  randomId(): string;
  requestApproval?: RequestInteractiveSendApproval;
  signal?: AbortSignal;
}

export interface InteractiveTurnDependencies {
  claim(
    baseUrl: string,
    localSessionId: string,
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<InteractiveAgentTurnClaim>;
  complete(
    baseUrl: string,
    turnId: string,
    turnToken: string,
    expectedLockVersion: number,
  ): Promise<InteractiveAgentTurn>;
  fail(
    baseUrl: string,
    turnId: string,
    turnToken: string,
    expectedLockVersion: number,
    failureCode: string,
  ): Promise<InteractiveAgentTurn>;
  heartbeat(
    baseUrl: string,
    turnId: string,
    turnToken: string,
    expectedLockVersion: number,
  ): Promise<InteractiveAgentTurn>;
  randomId(): string;
  runHost(options: RunHostOptions): Promise<HostResult>;
}

const installedDependencies: InteractiveTurnDependencies = {
  claim: claimInteractiveAgentTurn,
  complete: completeInteractiveAgentTurn,
  fail: failInteractiveAgentTurn,
  heartbeat: heartbeatInteractiveAgentTurn,
  randomId: randomUUID,
  runHost: async (options) => (
    await import('./host')
  ).runInteractiveAgentHost(options),
};

export interface RunInteractiveTurnOptions {
  baseUrl: string;
  database: AgentSessionStoreDatabase & SyncStoreDatabase;
  dependencies?: InteractiveTurnDependencies;
  onStreamText?(text: string): void;
  ownerId: string;
  requestApproval?: RequestInteractiveSendApproval;
  sessionId: string;
  signal?: AbortSignal;
  text: string;
}

export interface InteractiveTurnResult {
  entries: AgentEntryContent[];
  finalText: string;
}

function stableFailureCode(
  error: unknown,
  externalSignal: AbortSignal | undefined,
  lifecycleFailed: boolean,
) {
  if (externalSignal?.aborted) return 'interactive_agent_cancelled';
  if (lifecycleFailed) return 'interactive_agent_lease_lost';
  if (error instanceof Error && error.message === 'interactive_agent_contract_mismatch') {
    return 'interactive_agent_contract_mismatch';
  }
  return 'interactive_agent_failed';
}

/** Coordinates local persistence with a content-free server lease and quota reservation. */
export async function runInteractiveTurn({
  baseUrl,
  database,
  dependencies = installedDependencies,
  onStreamText,
  ownerId,
  requestApproval,
  sessionId,
  signal,
  text,
}: RunInteractiveTurnOptions): Promise<InteractiveTurnResult> {
  if (signal?.aborted) throw new Error('interactive_agent_cancelled');
  await appendAgentEntry(database, {
    ownerId,
    sessionId,
    content: { type: 'user', text },
  });

  let claim: InteractiveAgentTurnClaim | undefined;
  let lockVersion = 0;
  let heartbeatTimer: ReturnType<typeof setTimeout> | undefined;
  let heartbeatPromise: Promise<void> = Promise.resolve();
  let lifecycleFailed = false;
  let finished = false;
  const hostController = new AbortController();
  const abortHost = () => hostController.abort();
  signal?.addEventListener('abort', abortHost, { once: true });

  const heartbeat = async () => {
    if (!claim || finished) return;
    const updated = await dependencies.heartbeat(
      baseUrl,
      claim.id,
      claim.turnToken,
      lockVersion,
    );
    lockVersion = updated.lockVersion;
  };
  const scheduleHeartbeat = () => {
    if (finished) return;
    heartbeatTimer = setTimeout(() => {
      heartbeatPromise = heartbeat()
        .catch(() => {
          lifecycleFailed = true;
          hostController.abort();
        })
        .finally(scheduleHeartbeat);
    }, heartbeatIntervalMilliseconds);
  };
  const stopHeartbeats = async () => {
    finished = true;
    if (heartbeatTimer) clearTimeout(heartbeatTimer);
    await heartbeatPromise;
  };

  try {
    claim = await dependencies.claim(
      baseUrl,
      sessionId,
      dependencies.randomId(),
      signal,
    );
    lockVersion = claim.lockVersion;
    await heartbeat();
    scheduleHeartbeat();

    const entries = await readAgentEntries(database, ownerId, sessionId, { limit: 500 });
    const result = await dependencies.runHost({
      baseUrl,
      approvedSendDependencies: {
        decide: decideInteractiveAgentAction,
        execute: sendApprovedInteractiveAgentReply,
      },
      claim,
      database,
      entries,
      onStreamText,
      ownerId,
      randomId: dependencies.randomId,
      requestApproval,
      signal: hostController.signal,
    });
    if (signal?.aborted || lifecycleFailed) {
      throw new Error(lifecycleFailed
        ? 'interactive_agent_lease_lost'
        : 'interactive_agent_cancelled');
    }
    await stopHeartbeats();

    // Persist the real local result before marking the content-free server turn complete.
    // A crash can therefore never manufacture a completed local answer.
    await appendAgentEntries(database, {
      ownerId,
      sessionId,
      contents: result.entries,
    });
    await dependencies.complete(
      baseUrl,
      claim.id,
      claim.turnToken,
      lockVersion,
    );
    return result;
  } catch (error) {
    await stopHeartbeats();
    const failureCode = stableFailureCode(error, signal, lifecycleFailed);
    if (claim) {
      await dependencies.fail(
        baseUrl,
        claim.id,
        claim.turnToken,
        lockVersion,
        failureCode,
      ).catch(() => undefined);
    }
    await appendAgentEntry(database, {
      ownerId,
      sessionId,
      content: { type: 'error', code: failureCode },
    }).catch(() => undefined);
    throw new Error(failureCode);
  } finally {
    signal?.removeEventListener('abort', abortHost);
  }
}
