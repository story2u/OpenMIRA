import type {
  AppetiteSimulationSummary,
  ShadowEvaluation,
  TemporaryFocus,
} from '@story2u/radar-core/attention/model';

import {
  readActivePreference,
  readMessageFilterDecisions,
  readPreferenceVersion,
  readPreferenceIntents,
  type SignalAppetiteStoreExecutor,
} from '../../attention/signalAppetiteStore';
import { buildIntentMapModel } from './intent-map-model';

interface FocusRow {
  id: string;
  concept: string;
  delivery_mode: TemporaryFocus['deliveryMode'];
  created_at: string;
  expires_at: string;
  expired_at: string | null;
}

interface ShadowRow {
  id: string;
  old_version: number;
  candidate_version: number;
  started_at: string;
  ends_at: string;
  diff_summary_json: string;
  status: ShadowEvaluation['status'];
}

export async function readIntentMapModel(
  database: SignalAppetiteStoreExecutor,
  ownerId: string,
) {
  const preference = await readActivePreference(database, ownerId);
  const [intents, decisions, focusRows, shadowRow] = await Promise.all([
    preference
      ? readPreferenceIntents(database, ownerId, preference.id, preference.version)
      : Promise.resolve([]),
    readMessageFilterDecisions(database, ownerId, { limit: 500 }),
    database.getAllAsync<FocusRow>(
      `SELECT * FROM temporary_focuses
       WHERE owner_id = ? AND expired_at IS NULL AND expires_at > ?
       ORDER BY expires_at, id LIMIT 8`,
      ownerId,
      new Date().toISOString(),
    ),
    database.getFirstAsync<ShadowRow>(
      `SELECT * FROM shadow_evaluations
       WHERE owner_id = ? AND status = 'running' AND ends_at > ?
       ORDER BY started_at DESC LIMIT 1`,
      ownerId,
      new Date().toISOString(),
    ),
  ]);
  const temporaryFocuses: TemporaryFocus[] = focusRows.map((row) => ({
    id: row.id,
    concept: row.concept,
    deliveryMode: row.delivery_mode,
    createdAt: row.created_at,
    expiresAt: row.expires_at,
    expiredAt: row.expired_at,
  }));
  const shadow: ShadowEvaluation | null = shadowRow ? {
    id: shadowRow.id,
    oldVersion: shadowRow.old_version,
    candidateVersion: shadowRow.candidate_version,
    startedAt: shadowRow.started_at,
    endsAt: shadowRow.ends_at,
    diffSummary: JSON.parse(shadowRow.diff_summary_json) as AppetiteSimulationSummary,
    status: shadowRow.status,
  } : null;
  const model = buildIntentMapModel({ preference, intents, decisions, temporaryFocuses, shadow });
  if (!shadow) return model;
  const candidatePreference = await readPreferenceVersion(database, ownerId, shadow.candidateVersion);
  if (!candidatePreference) return model;
  const candidateIntents = await readPreferenceIntents(
    database,
    ownerId,
    candidatePreference.id,
    candidatePreference.version,
  );
  const candidateModel = buildIntentMapModel({
    preference: candidatePreference,
    intents: candidateIntents,
    decisions,
    temporaryFocuses,
    shadow,
  });
  return {
    ...model,
    candidate: {
      nodes: candidateModel.nodes,
      edges: candidateModel.edges,
      preference: candidatePreference,
      stats: {
        immediate: shadow.diffSummary.immediateCount,
        inbox: shadow.diffSummary.inboxCount,
        digest: shadow.diffSummary.digestCount,
        suppress: shadow.diffSummary.suppressCount,
        total: shadow.diffSummary.originalCount,
      },
    },
  };
}
