import { RadarApiError } from '@story2u/radar-api/client';
import type {
  AnalysisRun,
  AnalysisRunClaim,
  AnalysisRunLinks,
  AgentAnalysisResult,
} from '@story2u/radar-contracts/analysis-runs';

import {
  claimMessageAnalysisRun,
  claimNextMessageAnalysisRun,
  claimShadowMessageAnalysisRun,
  completeMessageAnalysisRun,
  expireMessageAnalysisRun,
  failMessageAnalysisRun,
  heartbeatMessageAnalysisRun,
  inspectMessageAnalysisLinks,
} from '../api/client';
import { initializeRadarDatabase } from '../storage/database';
import {
  deleteLocalAnalysisRun,
  readRecoverableAnalysisRuns,
  saveClaimedAnalysisRun,
  updateLocalAnalysisRun,
  type AnalysisRunStoreExecutor,
  type LocalAnalysisRun,
} from './analysisRunStore';
import {
  runDeviceAnalysis,
  type DeviceAnalysisClaim,
} from './deviceAnalysis';
import {
  analysisRunTokenStore,
  type AnalysisRunTokenStore,
} from './runTokenStorage';

const defaultHeartbeatIntervalMs = 30_000;
const maximumAttempts = 3;

export interface DeviceAnalysisRunApi {
  claim(messageId: string, signal?: AbortSignal): Promise<AnalysisRunClaim>;
  claimNext(signal?: AbortSignal): Promise<AnalysisRunClaim | null>;
  claimShadow(signal?: AbortSignal): Promise<AnalysisRunClaim | null>;
  complete(
    runId: string,
    runToken: string,
    expectedLockVersion: number,
    result: AgentAnalysisResult,
    signal?: AbortSignal,
  ): Promise<AnalysisRun>;
  expire(runId: string, signal?: AbortSignal): Promise<AnalysisRun>;
  fail(
    runId: string,
    runToken: string,
    expectedLockVersion: number,
    failureCode: string,
    signal?: AbortSignal,
  ): Promise<AnalysisRun>;
  heartbeat(
    runId: string,
    runToken: string,
    expectedLockVersion: number,
    signal?: AbortSignal,
  ): Promise<AnalysisRun>;
  inspectLinks(
    runId: string,
    runToken: string,
    signal?: AbortSignal,
  ): Promise<AnalysisRunLinks>;
}

export type DeviceAnalysisExecutor = (
  claim: DeviceAnalysisClaim,
  links: AnalysisRunLinks,
  signal: AbortSignal,
) => Promise<AgentAnalysisResult>;

export interface AnalysisRunCoordinatorOptions {
  api: DeviceAnalysisRunApi;
  database: AnalysisRunStoreExecutor;
  execute: DeviceAnalysisExecutor;
  heartbeatIntervalMs?: number;
  now?: () => number;
  tokenStore: AnalysisRunTokenStore;
}

export type DeviceAnalysisRunOutcome =
  | 'completed'
  | 'deferred'
  | 'expired'
  | 'failed';

function serverState(run: LocalAnalysisRun, response: AnalysisRun) {
  return {
    ...run,
    lockVersion: response.lockVersion,
    leaseExpiresAt: response.leaseExpiresAt,
  };
}

function isTerminalRunError(error: unknown) {
  return error instanceof RadarApiError && [401, 404, 409].includes(error.status);
}

export class AnalysisRunCoordinator {
  private readonly heartbeatIntervalMs: number;
  private readonly now: () => number;

  constructor(private readonly options: AnalysisRunCoordinatorOptions) {
    this.heartbeatIntervalMs = options.heartbeatIntervalMs ?? defaultHeartbeatIntervalMs;
    this.now = options.now ?? Date.now;
  }

  private async cleanup(run: LocalAnalysisRun) {
    await this.options.tokenStore.clear(run.runId);
    await deleteLocalAnalysisRun(this.options.database, run.ownerId, run.runId);
  }

  private async recordError(run: LocalAnalysisRun, code: string) {
    return updateLocalAnalysisRun(this.options.database, run, { lastErrorCode: code });
  }

  private async expire(run: LocalAnalysisRun, signal?: AbortSignal): Promise<DeviceAnalysisRunOutcome> {
    try {
      await this.options.api.expire(run.runId, signal);
      await this.cleanup(run);
      return 'expired';
    } catch (error) {
      if (isTerminalRunError(error)) {
        await this.cleanup(run);
        return 'expired';
      }
      await this.recordError(run, 'expire_deferred');
      return 'deferred';
    }
  }

  private async fail(
    run: LocalAnalysisRun,
    runToken: string,
    failureCode: string,
    signal?: AbortSignal,
  ): Promise<DeviceAnalysisRunOutcome> {
    try {
      await this.options.api.fail(
        run.runId,
        runToken,
        run.lockVersion,
        failureCode,
        signal,
      );
      await this.cleanup(run);
      return 'failed';
    } catch (error) {
      if (isTerminalRunError(error)) {
        await this.cleanup(run);
        return 'failed';
      }
      await this.recordError(run, 'fail_deferred');
      return 'deferred';
    }
  }

  private async execute(
    initialRun: LocalAnalysisRun,
    runToken: string,
    externalSignal?: AbortSignal,
  ): Promise<DeviceAnalysisRunOutcome> {
    if (initialRun.attemptCount >= maximumAttempts) {
      return this.fail(initialRun, runToken, 'agent_retry_exhausted', externalSignal);
    }
    const controller = new AbortController();
    const forwardAbort = () => controller.abort();
    externalSignal?.addEventListener('abort', forwardAbort, { once: true });
    let run = initialRun;
    let heartbeatFailure = false;
    let heartbeatTimer: ReturnType<typeof setTimeout> | undefined;
    let heartbeatPromise: Promise<void> = Promise.resolve();
    let heartbeatStopped = false;

    const heartbeat = async () => {
      const response = await this.options.api.heartbeat(
        run.runId,
        runToken,
        run.lockVersion,
        controller.signal,
      );
      run = await updateLocalAnalysisRun(
        this.options.database,
        serverState(run, response),
        {
          lockVersion: response.lockVersion,
          leaseExpiresAt: response.leaseExpiresAt,
          lastErrorCode: null,
        },
      );
    };
    const scheduleHeartbeat = () => {
      if (heartbeatStopped) return;
      heartbeatTimer = setTimeout(() => {
        heartbeatPromise = heartbeat()
          .catch(() => {
            heartbeatFailure = true;
            controller.abort();
          })
          .finally(scheduleHeartbeat);
      }, this.heartbeatIntervalMs);
    };
    const stopHeartbeat = async () => {
      heartbeatStopped = true;
      if (heartbeatTimer) clearTimeout(heartbeatTimer);
      await heartbeatPromise;
    };

    try {
      await heartbeat();
      scheduleHeartbeat();
      run = await updateLocalAnalysisRun(this.options.database, run, {
        phase: 'inspecting_links',
      });
      const links = await this.options.api.inspectLinks(
        run.runId,
        runToken,
        controller.signal,
      );
      run = await updateLocalAnalysisRun(this.options.database, run, {
        phase: 'running',
        attemptCount: run.attemptCount + 1,
      });
      const claim: DeviceAnalysisClaim = {
        id: run.runId,
        input: run.input,
        modelAlias: run.modelAlias,
        runToken,
        runtimeVersion: run.runtimeVersion,
        schemaVersion: run.schemaVersion,
        sourceMessageVersion: run.sourceMessageVersion,
      };
      const result = await this.options.execute(claim, links, controller.signal);
      await stopHeartbeat();
      if (heartbeatFailure || controller.signal.aborted) throw new Error('analysis_interrupted');
      run = await updateLocalAnalysisRun(this.options.database, run, {
        phase: 'completing',
      });
      const completed = await this.options.api.complete(
        run.runId,
        runToken,
        run.lockVersion,
        result,
        controller.signal,
      );
      run = serverState(run, completed);
      await this.cleanup(run);
      return 'completed';
    } catch (error) {
      await stopHeartbeat();
      if (isTerminalRunError(error)) {
        await this.cleanup(run);
        return 'expired';
      }
      if (externalSignal?.aborted || heartbeatFailure) {
        await this.recordError(run, heartbeatFailure ? 'heartbeat_failed' : 'analysis_interrupted');
        return 'deferred';
      }
      run = await this.recordError(run, 'agent_attempt_failed');
      if (run.attemptCount >= maximumAttempts) {
        return this.fail(run, runToken, 'agent_retry_exhausted');
      }
      return 'deferred';
    } finally {
      externalSignal?.removeEventListener('abort', forwardAbort);
      controller.abort();
    }
  }

  async claimAndExecute(
    ownerId: string,
    messageId: string,
    signal?: AbortSignal,
  ): Promise<DeviceAnalysisRunOutcome> {
    const claim = await this.options.api.claim(messageId, signal);
    const run = await saveClaimedAnalysisRun(this.options.database, ownerId, claim);
    try {
      await this.options.tokenStore.write(run.runId, claim.runToken);
    } catch {
      return this.fail(run, claim.runToken, 'token_storage_failed', signal);
    }
    return this.execute(run, claim.runToken, signal);
  }

  async recover(
    ownerId: string,
    executionEnabled: boolean,
    signal?: AbortSignal,
  ) {
    const outcomes: DeviceAnalysisRunOutcome[] = [];
    for (const run of await readRecoverableAnalysisRuns(this.options.database, ownerId)) {
      if (signal?.aborted) break;
      if (Date.parse(run.leaseExpiresAt) <= this.now()) {
        outcomes.push(await this.expire(run, signal));
        continue;
      }
      if (!executionEnabled) {
        outcomes.push('deferred');
        continue;
      }
      const token = await this.options.tokenStore.read(run.runId);
      if (!token) {
        await this.recordError(run, 'run_token_missing');
        outcomes.push('deferred');
        continue;
      }
      outcomes.push(await this.execute(run, token, signal));
    }
    if (executionEnabled && !signal?.aborted) {
      const primaryClaim = await this.options.api.claimNext(signal);
      if (primaryClaim) {
        const run = await saveClaimedAnalysisRun(this.options.database, ownerId, primaryClaim);
        try {
          await this.options.tokenStore.write(run.runId, primaryClaim.runToken);
          outcomes.push(await this.execute(run, primaryClaim.runToken, signal));
        } catch {
          outcomes.push(await this.fail(run, primaryClaim.runToken, 'token_storage_failed', signal));
        }
        return outcomes;
      }
      const claim = await this.options.api.claimShadow(signal);
      if (claim) {
        const run = await saveClaimedAnalysisRun(this.options.database, ownerId, claim);
        try {
          await this.options.tokenStore.write(run.runId, claim.runToken);
          outcomes.push(await this.execute(run, claim.runToken, signal));
        } catch {
          outcomes.push(await this.fail(run, claim.runToken, 'token_storage_failed', signal));
        }
      }
    }
    return outcomes;
  }
}

function installedApi(baseUrl: string): DeviceAnalysisRunApi {
  return {
    claim: (messageId, signal) => claimMessageAnalysisRun(baseUrl, messageId, signal),
    claimNext: (signal) => claimNextMessageAnalysisRun(baseUrl, signal),
    claimShadow: (signal) => claimShadowMessageAnalysisRun(baseUrl, signal),
    heartbeat: (runId, token, version, signal) => heartbeatMessageAnalysisRun(
      baseUrl,
      runId,
      token,
      version,
      signal,
    ),
    inspectLinks: (runId, token, signal) => inspectMessageAnalysisLinks(
      baseUrl,
      runId,
      token,
      signal,
    ),
    complete: (runId, token, version, result, signal) => completeMessageAnalysisRun(
      baseUrl,
      runId,
      token,
      { expectedLockVersion: version, result },
      signal,
    ),
    fail: (runId, token, version, code, signal) => failMessageAnalysisRun(
      baseUrl,
      runId,
      token,
      version,
      code,
      signal,
    ),
    expire: (runId, signal) => expireMessageAnalysisRun(baseUrl, runId, signal),
  };
}

let installedController: AbortController | null = null;
let installedTask: Promise<unknown> | null = null;

async function installedCoordinator(baseUrl: string) {
  const database = await initializeRadarDatabase();
  return new AnalysisRunCoordinator({
    api: installedApi(baseUrl),
    database,
    execute: (claim, links, signal) => runDeviceAnalysis({
      baseUrl,
      claim,
      links,
      signal,
    }),
    tokenStore: analysisRunTokenStore,
  });
}

export function pauseInstalledDeviceAnalysis() {
  installedController?.abort();
}

export async function recoverInstalledDeviceAnalysis(
  baseUrl: string,
  ownerId: string,
  executionEnabled: boolean,
) {
  if (installedTask) return installedTask;
  const controller = new AbortController();
  installedController = controller;
  installedTask = installedCoordinator(baseUrl)
    .then((coordinator) => coordinator.recover(ownerId, executionEnabled, controller.signal))
    .finally(() => {
      if (installedController === controller) installedController = null;
      installedTask = null;
    });
  return installedTask;
}

/** Explicit primary-run entry point for future server-owned message scheduling. */
export async function claimAndExecuteInstalledDeviceAnalysis(
  baseUrl: string,
  ownerId: string,
  messageId: string,
) {
  if (installedTask) return 'deferred' as const;
  const controller = new AbortController();
  installedController = controller;
  installedTask = installedCoordinator(baseUrl)
    .then((coordinator) => coordinator.claimAndExecute(ownerId, messageId, controller.signal))
    .finally(() => {
      if (installedController === controller) installedController = null;
      installedTask = null;
    });
  return installedTask as Promise<DeviceAnalysisRunOutcome>;
}
