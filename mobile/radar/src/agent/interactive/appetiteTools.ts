import {
  ApplyAppetiteChangeParameters,
  CapturePreferenceExampleParameters,
  ComparePreferenceVersionsParameters,
  CorrectMessageDecisionParameters,
  CreateTemporaryFocusParameters,
  ExplainMessageDecisionParameters,
  INTERACTIVE_SIGNAL_APPETITE_TOOLS,
  InspectSignalAppetiteParameters,
  ListSuppressedSamplesParameters,
  ProposeAppetiteChangeParameters,
  SimulateAppetiteParameters,
  StartShadowModeParameters,
  StartTeachingSessionParameters,
  SummarizeTeachingSessionParameters,
  UndoPreferenceChangeParameters,
  UpdateAttentionScheduleParameters,
  type InteractiveAppetiteToolName,
} from '@story2u/radar-agent/interactive';
import type { MessageFilterDecision, PreferenceExample } from '@story2u/radar-core/attention/model';
import type { TSchema } from 'typebox';
import { Value } from 'typebox/value';

import {
  applyPreferenceVersion,
  comparePreferenceVersions,
  createTemporaryFocus,
  proposeAppetiteFromTeaching,
  proposeScheduleUpdate,
  simulatePreferenceVersion,
  startPreferenceShadow,
  undoPreferenceChange,
} from '../../attention/appetiteService';
import {
  appendSignalAppetiteEvent,
  readActivePreference,
  readMessageFilterDecisions,
  readPreferenceIntents,
  readPreferenceVersion,
  type SignalAppetiteStoreDatabase,
} from '../../attention/signalAppetiteStore';
import {
  captureTeachingExample,
  completeTeachingSession,
  startTeachingSession,
} from '../../attention/teachingService';

const schemas: Readonly<Record<InteractiveAppetiteToolName, TSchema>> = Object.freeze({
  inspect_signal_appetite: InspectSignalAppetiteParameters,
  start_teaching_session: StartTeachingSessionParameters,
  capture_preference_example: CapturePreferenceExampleParameters,
  summarize_teaching_session: SummarizeTeachingSessionParameters,
  propose_appetite_change: ProposeAppetiteChangeParameters,
  simulate_appetite: SimulateAppetiteParameters,
  apply_appetite_change: ApplyAppetiteChangeParameters,
  start_shadow_mode: StartShadowModeParameters,
  explain_message_decision: ExplainMessageDecisionParameters,
  list_suppressed_samples: ListSuppressedSamplesParameters,
  correct_message_decision: CorrectMessageDecisionParameters,
  create_temporary_focus: CreateTemporaryFocusParameters,
  update_attention_schedule: UpdateAttentionScheduleParameters,
  undo_preference_change: UndoPreferenceChangeParameters,
  compare_preference_versions: ComparePreferenceVersionsParameters,
});
const knownTools = new Set<InteractiveAppetiteToolName>(
  INTERACTIVE_SIGNAL_APPETITE_TOOLS.map((tool) => tool.name as InteractiveAppetiteToolName),
);

export interface InteractiveAppetiteToolCall {
  arguments: unknown;
  name: string;
  toolCallId: string;
}

export class InteractiveAppetiteToolError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = 'InteractiveAppetiteToolError';
  }
}

async function nextVersion(
  database: SignalAppetiteStoreDatabase,
  ownerId: string,
  aggregateId: string,
) {
  const row = await database.getFirstAsync<{ maximum: number | null }>(
    `SELECT MAX(aggregate_version) AS maximum FROM attention_events
     WHERE owner_id = ? AND aggregate_id = ?`,
    ownerId,
    aggregateId,
  );
  return (row?.maximum ?? 0) + 1;
}

async function inspect(database: SignalAppetiteStoreDatabase, ownerId: string, now: Date) {
  const active = await readActivePreference(database, ownerId);
  const intents = active
    ? await readPreferenceIntents(database, ownerId, active.id, active.version)
    : [];
  const stats = await database.getAllAsync<{ decision: string; total: number }>(
    `SELECT decision, COUNT(*) AS total FROM message_filter_decisions
     WHERE owner_id = ? GROUP BY decision`,
    ownerId,
  );
  const focuses = await database.getAllAsync<{
    id: string; concept: string; delivery_mode: string; created_at: string; expires_at: string;
  }>(
    `SELECT id, concept, delivery_mode, created_at, expires_at FROM temporary_focuses
     WHERE owner_id = ? AND expired_at IS NULL AND expires_at > ?
     ORDER BY expires_at LIMIT 20`,
    ownerId,
    now.toISOString(),
  );
  return {
    activePreference: active,
    intents,
    schedule: active?.schedule ?? [],
    temporaryFocuses: focuses.map((focus) => ({
      id: focus.id,
      concept: focus.concept,
      deliveryMode: focus.delivery_mode,
      createdAt: focus.created_at,
      expiresAt: focus.expires_at,
    })),
    statistics: Object.fromEntries(stats.map((row) => [row.decision, row.total])),
  };
}

async function latestActiveSession(database: SignalAppetiteStoreDatabase, ownerId: string) {
  return database.getFirstAsync<{ id: string }>(
    `SELECT id FROM teaching_sessions WHERE owner_id = ? AND status = 'active'
     ORDER BY started_at DESC, id DESC LIMIT 1`,
    ownerId,
  );
}

async function quietZoneSamples(
  database: SignalAppetiteStoreDatabase,
  ownerId: string,
  limit: number,
) {
  const rows = await database.getAllAsync<{
    message_id: string;
    decision: MessageFilterDecision['decision'];
    confidence: number;
    reason_summary: string;
    evaluator: MessageFilterDecision['evaluator'];
    payload_json: string;
  }>(
    `SELECT decision.message_id, decision.decision, decision.confidence,
      decision.reason_summary, decision.evaluator, message.payload_json
     FROM message_filter_decisions AS decision
     JOIN message_projection AS message
       ON message.owner_id = decision.owner_id AND message.id = decision.message_id
     WHERE decision.owner_id = ? AND decision.decision = 'suppress'
       AND message.deleted_at IS NULL
     ORDER BY decision.decided_at DESC LIMIT ?`,
    ownerId,
    limit,
  );
  return rows.map((row) => {
    const payload = JSON.parse(row.payload_json) as {
      senderName?: string; content?: string; sentAt?: string;
    };
    return {
      messageId: row.message_id,
      senderName: String(payload.senderName ?? ''),
      body: String(payload.content ?? '').slice(0, 2_000),
      sentAt: String(payload.sentAt ?? ''),
      decision: row.decision,
      confidence: row.confidence,
      reasonSummary: row.reason_summary,
      evaluator: row.evaluator,
    };
  });
}

async function correctDecision(
  database: SignalAppetiteStoreDatabase,
  input: {
    ownerId: string;
    deviceId: string;
    messageId: string;
    corrected: MessageFilterDecision['decision'];
    reason?: string;
    now: Date;
    randomId(): string;
  },
) {
  const active = await readActivePreference(database, input.ownerId);
  if (!active) throw new InteractiveAppetiteToolError('active_preference_not_found');
  const previous = (await readMessageFilterDecisions(database, input.ownerId, { limit: 500 }))
    .find((decision) => decision.messageId === input.messageId);
  if (!previous) throw new InteractiveAppetiteToolError('message_decision_not_found');
  const sessionId = input.randomId();
  await appendSignalAppetiteEvent(database, {
    eventId: input.randomId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: sessionId, aggregateVersion: 1, schemaVersion: 1,
    occurredAt: input.now.toISOString(), type: 'TeachingSessionStarted',
    payload: { sessionId, targetCount: 1 },
  });
  const label: PreferenceExample['label'] = input.corrected === 'immediate' || input.corrected === 'inbox'
    ? 'positive'
    : 'negative';
  const example: PreferenceExample = {
    id: input.randomId(), messageId: input.messageId, label,
    selectedReasons: ['decision_correction'],
    freeformReason: input.reason?.trim().slice(0, 1_000) || null,
    capturedAt: input.now.toISOString(), teachingSessionId: sessionId, revertedAt: null,
  };
  const correctedDecision: MessageFilterDecision = {
    ...previous,
    preferenceVersion: active.version,
    decision: input.corrected,
    confidence: 1,
    reasonSummary: input.reason?.trim().slice(0, 1_000) || 'Corrected by you',
    evidence: [{ kind: 'message_signal', label: 'user_correction' }],
    evaluator: 'deterministic',
    decidedAt: input.now.toISOString(),
  };
  await appendSignalAppetiteEvent(database, {
    eventId: input.randomId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: input.messageId,
    aggregateVersion: await nextVersion(database, input.ownerId, input.messageId),
    schemaVersion: 1, occurredAt: input.now.toISOString(), type: 'MessageDecisionCorrected',
    payload: { previousDecision: previous, correctedDecision, example },
  });
  await appendSignalAppetiteEvent(database, {
    eventId: input.randomId(), ownerId: input.ownerId, deviceId: input.deviceId,
    aggregateId: sessionId, aggregateVersion: 2, schemaVersion: 1,
    occurredAt: input.now.toISOString(), type: 'TeachingSessionCompleted',
    payload: {
      sessionId, completedAt: input.now.toISOString(),
      summary: label === 'positive'
        ? { increase: ['decision_correction'], reduce: [] }
        : { increase: [], reduce: ['decision_correction'] },
    },
  });
  return correctedDecision;
}

export async function executeInteractiveAppetiteTool(
  database: SignalAppetiteStoreDatabase,
  options: {
    allowedTools: ReadonlySet<InteractiveAppetiteToolName>;
    approvedApplyCalls: ReadonlySet<string>;
    call: InteractiveAppetiteToolCall;
    deviceId: string;
    now?: Date;
    ownerId: string;
    randomId(): string;
    signal?: AbortSignal;
  },
): Promise<Record<string, unknown>> {
  const { call, ownerId, deviceId, randomId, signal } = options;
  if (!knownTools.has(call.name as InteractiveAppetiteToolName)) {
    throw new InteractiveAppetiteToolError('unknown_tool');
  }
  const tool = call.name as InteractiveAppetiteToolName;
  if (!options.allowedTools.has(tool)) throw new InteractiveAppetiteToolError('tool_not_authorized');
  if (!Value.Check(schemas[tool], call.arguments)) {
    throw new InteractiveAppetiteToolError('invalid_tool_arguments');
  }
  if (signal?.aborted) throw new InteractiveAppetiteToolError('interactive_agent_cancelled');
  const args = call.arguments as Record<string, unknown>;
  const now = options.now ?? new Date();
  const context = { ownerId, deviceId, now, createId: randomId };

  switch (tool) {
    case 'inspect_signal_appetite':
      return inspect(database, ownerId, now);
    case 'start_teaching_session': {
      const session = await startTeachingSession(database, {
        ...context, targetCount: args.target_count as number | undefined,
      });
      return {
        teachingSessionId: session.sessionId,
        cardCount: session.cards.length,
        categories: session.cards.map((card) => card.category),
        state: 'ready',
      };
    }
    case 'capture_preference_example': {
      const session = await latestActiveSession(database, ownerId);
      if (!session) throw new InteractiveAppetiteToolError('teaching_session_not_active');
      const example = await captureTeachingExample(database, {
        ...context,
        sessionId: session.id,
        messageId: String(args.message_id),
        label: args.label as PreferenceExample['label'],
        reasons: args.reasons as string[] | undefined,
        freeformReason: args.freeform_reason as string | undefined,
      });
      return { exampleId: example.id, label: example.label, state: 'captured', activeChanged: false };
    }
    case 'summarize_teaching_session':
      return { ...await completeTeachingSession(database, {
        ...context, sessionId: String(args.teaching_session_id),
      }) };
    case 'propose_appetite_change': {
      const proposal = await proposeAppetiteFromTeaching(database, {
        ...context, sessionId: String(args.teaching_session_id),
      });
      return { preference: proposal.preference, intents: proposal.intents, state: 'candidate' };
    }
    case 'simulate_appetite':
      return { ...await simulatePreferenceVersion(database, {
        ...context, version: Number(args.preference_version),
      }) };
    case 'apply_appetite_change': {
      if (!options.approvedApplyCalls.has(call.toolCallId)) {
        throw new InteractiveAppetiteToolError('preference_confirmation_required');
      }
      const applied = await applyPreferenceVersion(database, {
        ...context, version: Number(args.preference_version), confirmed: true,
      });
      return {
        state: 'active',
        preferenceVersion: applied.preference?.version,
        decisionCount: applied.decisions.length,
      };
    }
    case 'start_shadow_mode':
      return startPreferenceShadow(database, {
        ...context,
        candidateVersion: Number(args.preference_version),
        durationHours: args.duration_hours as number | undefined,
      });
    case 'explain_message_decision': {
      const decision = (await readMessageFilterDecisions(database, ownerId, { limit: 500 }))
        .find((item) => item.messageId === args.message_id);
      if (!decision) throw new InteractiveAppetiteToolError('message_decision_not_found');
      return { ...decision };
    }
    case 'list_suppressed_samples':
      return { samples: await quietZoneSamples(database, ownerId, Number(args.limit ?? 10)) };
    case 'correct_message_decision':
      return { ...await correctDecision(database, {
        ownerId, deviceId, messageId: String(args.message_id),
        corrected: args.decision as MessageFilterDecision['decision'],
        reason: args.reason as string | undefined, now, randomId,
      }) };
    case 'create_temporary_focus':
      return { ...await createTemporaryFocus(database, {
        ...context,
        concept: String(args.concept),
        durationHours: Number(args.duration_hours),
        deliveryMode: args.delivery_mode as 'immediate' | 'inbox' | 'digest' | undefined,
      }) };
    case 'update_attention_schedule':
      return proposeScheduleUpdate(database, { ...context, instruction: String(args.instruction) });
    case 'undo_preference_change':
      return { state: 'active', preference: await undoPreferenceChange(database, context) };
    case 'compare_preference_versions':
      return comparePreferenceVersions(database, {
        ownerId,
        fromVersion: Number(args.from_version),
        toVersion: Number(args.to_version),
      });
  }
}
