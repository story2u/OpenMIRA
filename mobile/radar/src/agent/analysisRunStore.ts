import { AnalysisRunInputSchema } from '@story2u/radar-api/analysis-runs';
import type { AnalysisRunClaim } from '@story2u/radar-contracts/analysis-runs';
import { Value } from 'typebox/value';

export type LocalAnalysisRunPhase =
  | 'claimed'
  | 'inspecting_links'
  | 'running'
  | 'completing';

export interface AnalysisRunStoreExecutor {
  getAllAsync<Row>(source: string, ...params: Array<string | number | null>): Promise<Row[]>;
  runAsync(source: string, ...params: Array<string | number | null>): Promise<unknown>;
}

export interface LocalAnalysisRun {
  attemptCount: number;
  createdAt: string;
  deviceId: string;
  input: AnalysisRunClaim['input'];
  lastErrorCode: string | null;
  leaseExpiresAt: string;
  lockVersion: number;
  messageId: string;
  modelAlias: string;
  ownerId: string;
  phase: LocalAnalysisRunPhase;
  policyVersion: string;
  runId: string;
  runtimeVersion: string;
  schemaVersion: number;
  sourceMessageVersion: number;
  updatedAt: string;
}

interface AnalysisRunRow {
  attempt_count: number;
  created_at: string;
  device_id: string;
  input_json: string;
  last_error_code: string | null;
  lease_expires_at: string;
  lock_version: number;
  message_id: string;
  model_alias: string;
  owner_id: string;
  phase: LocalAnalysisRunPhase;
  policy_version: string;
  run_id: string;
  runtime_version: string;
  schema_version: number;
  source_message_version: number;
  updated_at: string;
}

const uuidPattern = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const phaseValues = new Set<LocalAnalysisRunPhase>([
  'claimed',
  'inspecting_links',
  'running',
  'completing',
]);
const errorCodePattern = /^[a-z][a-z0-9_.-]{0,63}$/;

function requireUuid(value: string) {
  if (!uuidPattern.test(value)) throw new Error('invalid local analysis run identity');
}

function decodeRow(row: AnalysisRunRow): LocalAnalysisRun {
  requireUuid(row.owner_id);
  requireUuid(row.run_id);
  requireUuid(row.message_id);
  requireUuid(row.device_id);
  if (
    !phaseValues.has(row.phase)
    || !Number.isInteger(row.lock_version)
    || row.lock_version < 1
    || !Number.isInteger(row.source_message_version)
    || row.source_message_version < 1
    || !Number.isInteger(row.attempt_count)
    || row.attempt_count < 0
    || row.attempt_count > 3
    || (row.last_error_code !== null && !errorCodePattern.test(row.last_error_code))
  ) {
    throw new Error('invalid local analysis run state');
  }
  const parsedInput = Value.Parse(
    AnalysisRunInputSchema,
    JSON.parse(row.input_json),
  ) as AnalysisRunClaim['input'];
  return {
    ownerId: row.owner_id,
    runId: row.run_id,
    messageId: row.message_id,
    deviceId: row.device_id,
    phase: row.phase,
    sourceMessageVersion: row.source_message_version,
    lockVersion: row.lock_version,
    leaseExpiresAt: row.lease_expires_at,
    runtimeVersion: row.runtime_version,
    schemaVersion: row.schema_version,
    modelAlias: row.model_alias,
    policyVersion: row.policy_version,
    input: parsedInput,
    attemptCount: row.attempt_count,
    lastErrorCode: row.last_error_code,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

function rowParameters(run: LocalAnalysisRun) {
  return [
    run.phase,
    run.lockVersion,
    run.leaseExpiresAt,
    run.attemptCount,
    run.lastErrorCode,
    run.updatedAt,
    run.ownerId,
    run.runId,
  ] as const;
}

export async function saveClaimedAnalysisRun(
  database: AnalysisRunStoreExecutor,
  ownerId: string,
  claim: AnalysisRunClaim,
): Promise<LocalAnalysisRun> {
  requireUuid(ownerId);
  const now = new Date().toISOString();
  const run: LocalAnalysisRun = {
    ownerId,
    runId: claim.id,
    messageId: claim.messageId,
    deviceId: claim.deviceId,
    phase: 'claimed',
    sourceMessageVersion: claim.sourceMessageVersion,
    lockVersion: claim.lockVersion,
    leaseExpiresAt: claim.leaseExpiresAt,
    runtimeVersion: claim.runtimeVersion,
    schemaVersion: claim.schemaVersion,
    modelAlias: claim.modelAlias,
    policyVersion: claim.policyVersion,
    input: claim.input,
    attemptCount: 0,
    lastErrorCode: null,
    createdAt: now,
    updatedAt: now,
  };
  requireUuid(run.runId);
  requireUuid(run.messageId);
  requireUuid(run.deviceId);
  const inputJson = JSON.stringify(run.input);
  if (inputJson.length > 65_536) throw new Error('analysis run input is too large');
  await database.runAsync(
    `INSERT INTO analysis_run_state (
      owner_id, run_id, message_id, device_id, phase, source_message_version,
      lock_version, lease_expires_at, runtime_version, schema_version, model_alias,
      policy_version, input_json, attempt_count, last_error_code, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, ?, ?)
    ON CONFLICT(owner_id, run_id) DO UPDATE SET
      lock_version = excluded.lock_version,
      lease_expires_at = excluded.lease_expires_at,
      updated_at = excluded.updated_at
    WHERE analysis_run_state.message_id = excluded.message_id
      AND analysis_run_state.device_id = excluded.device_id
      AND analysis_run_state.source_message_version = excluded.source_message_version`,
    ownerId,
    run.runId,
    run.messageId,
    run.deviceId,
    run.phase,
    run.sourceMessageVersion,
    run.lockVersion,
    run.leaseExpiresAt,
    run.runtimeVersion,
    run.schemaVersion,
    run.modelAlias,
    run.policyVersion,
    inputJson,
    now,
    now,
  );
  return run;
}

export async function readRecoverableAnalysisRuns(
  database: AnalysisRunStoreExecutor,
  ownerId: string,
) {
  requireUuid(ownerId);
  const rows = await database.getAllAsync<AnalysisRunRow>(
    `SELECT owner_id, run_id, message_id, device_id, phase, source_message_version,
      lock_version, lease_expires_at, runtime_version, schema_version, model_alias,
      policy_version, input_json, attempt_count, last_error_code, created_at, updated_at
     FROM analysis_run_state WHERE owner_id = ?
     ORDER BY lease_expires_at, updated_at, run_id`,
    ownerId,
  );
  return rows.map(decodeRow);
}

export async function updateLocalAnalysisRun(
  database: AnalysisRunStoreExecutor,
  run: LocalAnalysisRun,
  patch: Partial<Pick<
    LocalAnalysisRun,
    | 'attemptCount'
    | 'lastErrorCode'
    | 'leaseExpiresAt'
    | 'lockVersion'
    | 'phase'
  >>,
) {
  const updated: LocalAnalysisRun = {
    ...run,
    ...patch,
    updatedAt: new Date().toISOString(),
  };
  if (
    updated.attemptCount < 0
    || updated.attemptCount > 3
    || !phaseValues.has(updated.phase)
    || (updated.lastErrorCode !== null && !errorCodePattern.test(updated.lastErrorCode))
  ) {
    throw new Error('invalid local analysis run update');
  }
  await database.runAsync(
    `UPDATE analysis_run_state SET
      phase = ?, lock_version = ?, lease_expires_at = ?, attempt_count = ?,
      last_error_code = ?, updated_at = ?
     WHERE owner_id = ? AND run_id = ?`,
    ...rowParameters(updated),
  );
  return updated;
}

export async function deleteLocalAnalysisRun(
  database: AnalysisRunStoreExecutor,
  ownerId: string,
  runId: string,
) {
  requireUuid(ownerId);
  requireUuid(runId);
  await database.runAsync(
    'DELETE FROM analysis_run_state WHERE owner_id = ? AND run_id = ?',
    ownerId,
    runId,
  );
}
