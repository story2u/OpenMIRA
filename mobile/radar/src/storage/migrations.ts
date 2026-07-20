export interface MigrationExecutor {
  execAsync(source: string): Promise<void>;
  runAsync(source: string, ...params: Array<string | number | null>): Promise<unknown>;
}

export interface MigrationDatabase extends MigrationExecutor {
  getAllAsync<Row>(source: string): Promise<Row[]>;
  withExclusiveTransactionAsync(task: (transaction: MigrationExecutor) => Promise<void>): Promise<void>;
}

export interface RadarMigration {
  name: string;
  up(executor: MigrationExecutor): Promise<void>;
  version: number;
}

const initialSchema = `
  CREATE TABLE sync_state (
    owner_id TEXT NOT NULL,
    stream TEXT NOT NULL,
    cursor INTEGER NOT NULL DEFAULT 0 CHECK (cursor >= 0),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, stream)
  ) WITHOUT ROWID;

  CREATE TABLE change_inbox (
    owner_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    cursor INTEGER NOT NULL CHECK (cursor > 0),
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version >= 0),
    operation TEXT NOT NULL CHECK (operation IN ('upsert', 'delete')),
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    received_at TEXT NOT NULL,
    applied_at TEXT,
    PRIMARY KEY (owner_id, event_id),
    UNIQUE (owner_id, cursor)
  ) WITHOUT ROWID;

  CREATE INDEX change_inbox_unapplied_idx
    ON change_inbox (owner_id, cursor)
    WHERE applied_at IS NULL;

  CREATE TABLE opportunity_projection (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version >= 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE TABLE command_outbox (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    command_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    expected_version INTEGER CHECK (expected_version IS NULL OR expected_version >= 0),
    idempotency_key TEXT NOT NULL,
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TEXT,
    last_error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, id),
    UNIQUE (owner_id, idempotency_key)
  ) WITHOUT ROWID;

  CREATE INDEX command_outbox_pending_idx
    ON command_outbox (owner_id, next_attempt_at, created_at)
    WHERE status IN ('pending', 'failed');
`;

const syncProjectionSchema = `
  ALTER TABLE sync_state
    ADD COLUMN phase TEXT NOT NULL DEFAULT 'ready'
    CHECK (phase IN ('ready', 'bootstrapping', 'error'));
  ALTER TABLE sync_state
    ADD COLUMN last_error_code TEXT;

  ALTER TABLE opportunity_projection ADD COLUMN platform TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN frontend_status TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN source_type TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN created_at TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN last_message_at TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN trust_score INTEGER;
  ALTER TABLE opportunity_projection ADD COLUMN sop_stage TEXT;
  ALTER TABLE opportunity_projection ADD COLUMN confidence_score REAL;
  ALTER TABLE opportunity_projection ADD COLUMN attention_required INTEGER;
  ALTER TABLE opportunity_projection ADD COLUMN archived_at TEXT;

  CREATE INDEX opportunity_projection_dashboard_idx
    ON opportunity_projection (
      owner_id,
      deleted_at,
      archived_at,
      frontend_status,
      created_at DESC,
      id
    );

  CREATE TABLE message_projection (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    opportunity_id TEXT,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version >= 0),
    sent_at TEXT NOT NULL,
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE INDEX message_projection_opportunity_sent_idx
    ON message_projection (owner_id, opportunity_id, sent_at, id)
    WHERE deleted_at IS NULL;

  CREATE TABLE setting_projection (
    owner_id TEXT NOT NULL,
    setting_type TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version >= 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    PRIMARY KEY (owner_id, setting_type)
  ) WITHOUT ROWID;

  CREATE TABLE sync_bootstrap_state (
    owner_id TEXT PRIMARY KEY NOT NULL,
    watermark_cursor INTEGER NOT NULL CHECK (watermark_cursor >= 0),
    next_page_token TEXT,
    updated_at TEXT NOT NULL
  ) WITHOUT ROWID;

  CREATE TABLE client_capability_state (
    owner_id TEXT PRIMARY KEY NOT NULL,
    sync_available INTEGER NOT NULL DEFAULT 0 CHECK (sync_available IN (0, 1)),
    updated_at TEXT NOT NULL
  ) WITHOUT ROWID;
`;

const internalCommandSchema = `
  ALTER TABLE command_outbox ADD COLUMN expires_at TEXT;
  CREATE INDEX command_outbox_owner_aggregate_idx
    ON command_outbox (owner_id, aggregate_type, aggregate_id, status);
`;

const deviceAnalysisRunSchema = `
  CREATE TABLE analysis_run_state (
    owner_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    phase TEXT NOT NULL
      CHECK (phase IN ('claimed', 'inspecting_links', 'running', 'completing')),
    source_message_version INTEGER NOT NULL CHECK (source_message_version > 0),
    lock_version INTEGER NOT NULL CHECK (lock_version > 0),
    lease_expires_at TEXT NOT NULL,
    runtime_version TEXT NOT NULL CHECK (length(runtime_version) BETWEEN 1 AND 64),
    schema_version INTEGER NOT NULL CHECK (schema_version BETWEEN 1 AND 100),
    model_alias TEXT NOT NULL CHECK (length(model_alias) BETWEEN 1 AND 64),
    policy_version TEXT NOT NULL CHECK (length(policy_version) BETWEEN 1 AND 64),
    input_json TEXT NOT NULL CHECK (json_valid(input_json) AND length(input_json) <= 65536),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 3),
    last_error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, run_id)
  ) WITHOUT ROWID;

  CREATE INDEX analysis_run_state_recovery_idx
    ON analysis_run_state (owner_id, lease_expires_at, updated_at);
`;

const interactiveAgentSessionSchema = `
  CREATE TABLE agent_sessions (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    opportunity_id TEXT,
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE INDEX agent_sessions_recent_idx
    ON agent_sessions (owner_id, updated_at DESC, id);

  CREATE INDEX agent_sessions_expiry_idx
    ON agent_sessions (owner_id, expires_at);

  CREATE TABLE agent_entries (
    owner_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    seq INTEGER NOT NULL CHECK (seq > 0),
    entry_type TEXT NOT NULL
      CHECK (entry_type IN ('user', 'assistant', 'tool_call', 'tool_result', 'error')),
    content_json TEXT NOT NULL CHECK (json_valid(content_json) AND length(content_json) <= 65536),
    created_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, session_id, seq),
    FOREIGN KEY (owner_id, session_id)
      REFERENCES agent_sessions (owner_id, id) ON DELETE CASCADE
  ) WITHOUT ROWID;
`;

const signalAppetiteSchema = `
  CREATE TABLE attention_events (
    local_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
      'TeachingSessionStarted',
      'TeachingCardPresented',
      'PreferenceExampleCaptured',
      'PreferenceExampleReverted',
      'TeachingSessionCompleted',
      'PreferenceChangeProposed',
      'PreferenceSimulationCompleted',
      'PreferenceShadowStarted',
      'PreferenceApplied',
      'PreferenceReverted',
      'MessageFilterDecisionMade',
      'MessageDecisionCorrected',
      'IntentMapUpdated',
      'TemporaryFocusCreated',
      'TemporaryFocusExpired'
    )),
    aggregate_id TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version > 0),
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json) AND length(payload_json) <= 65536),
    occurred_at TEXT NOT NULL,
    sync_status TEXT NOT NULL DEFAULT 'pending'
      CHECK (sync_status IN ('pending', 'synced', 'error')),
    server_cursor INTEGER CHECK (server_cursor IS NULL OR server_cursor > 0),
    UNIQUE (owner_id, event_id)
  );

  CREATE INDEX attention_events_owner_sequence_idx
    ON attention_events (owner_id, local_sequence);
  CREATE INDEX attention_events_pending_idx
    ON attention_events (owner_id, local_sequence)
    WHERE sync_status IN ('pending', 'error');

  CREATE TABLE attention_preferences (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    natural_language_summary TEXT NOT NULL CHECK (length(natural_language_summary) <= 4000),
    scope TEXT NOT NULL CHECK (scope IN ('all_messages', 'opportunities', 'jobs')),
    status TEXT NOT NULL CHECK (status IN ('candidate', 'active', 'superseded', 'reverted')),
    confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    active_from TEXT,
    active_until TEXT,
    schedule_json TEXT NOT NULL CHECK (json_valid(schedule_json)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, id, version)
  ) WITHOUT ROWID;

  CREATE UNIQUE INDEX attention_preferences_one_active_idx
    ON attention_preferences (owner_id) WHERE status = 'active';

  CREATE TABLE attention_intents (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    preference_id TEXT NOT NULL,
    preference_version INTEGER NOT NULL CHECK (preference_version > 0),
    concept TEXT NOT NULL CHECK (length(concept) BETWEEN 1 AND 120),
    intent_type TEXT NOT NULL CHECK (intent_type IN ('include', 'reduce', 'context')),
    weight REAL NOT NULL CHECK (weight BETWEEN -1 AND 1),
    delivery_mode TEXT NOT NULL CHECK (delivery_mode IN ('immediate', 'inbox', 'digest', 'suppress')),
    confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    user_confirmed INTEGER NOT NULL CHECK (user_confirmed IN (0, 1)),
    source TEXT NOT NULL CHECK (source IN ('teaching', 'conversation', 'correction', 'temporary_focus')),
    valid_from TEXT,
    valid_until TEXT,
    PRIMARY KEY (owner_id, id, preference_version),
    FOREIGN KEY (owner_id, preference_id, preference_version)
      REFERENCES attention_preferences (owner_id, id, version) ON DELETE CASCADE
  ) WITHOUT ROWID;

  CREATE INDEX attention_intents_preference_idx
    ON attention_intents (owner_id, preference_id, preference_version);

  CREATE TABLE teaching_sessions (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    target_count INTEGER NOT NULL CHECK (target_count BETWEEN 1 AND 15),
    presented_count INTEGER NOT NULL DEFAULT 0 CHECK (presented_count >= 0),
    positive_count INTEGER NOT NULL DEFAULT 0 CHECK (positive_count >= 0),
    negative_count INTEGER NOT NULL DEFAULT 0 CHECK (negative_count >= 0),
    skipped_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_count >= 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'summarized', 'completed', 'abandoned')),
    proposed_preference_version INTEGER,
    summary_json TEXT CHECK (summary_json IS NULL OR json_valid(summary_json)),
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE TABLE preference_examples (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    label TEXT NOT NULL CHECK (label IN ('positive', 'negative', 'skipped', 'boundary')),
    selected_reasons_json TEXT NOT NULL CHECK (json_valid(selected_reasons_json)),
    freeform_reason TEXT CHECK (freeform_reason IS NULL OR length(freeform_reason) <= 1000),
    captured_at TEXT NOT NULL,
    teaching_session_id TEXT NOT NULL,
    reverted_at TEXT,
    PRIMARY KEY (owner_id, id),
    FOREIGN KEY (owner_id, teaching_session_id)
      REFERENCES teaching_sessions (owner_id, id) ON DELETE CASCADE
  ) WITHOUT ROWID;

  CREATE INDEX preference_examples_session_idx
    ON preference_examples (owner_id, teaching_session_id, captured_at, id);
  CREATE INDEX preference_examples_message_idx
    ON preference_examples (owner_id, message_id, captured_at DESC);

  CREATE TABLE message_filter_decisions (
    owner_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    preference_version INTEGER NOT NULL CHECK (preference_version > 0),
    decision TEXT NOT NULL CHECK (decision IN ('immediate', 'inbox', 'digest', 'suppress')),
    confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    reason_summary TEXT NOT NULL CHECK (length(reason_summary) BETWEEN 1 AND 1000),
    evidence_json TEXT NOT NULL CHECK (json_valid(evidence_json)),
    evaluator TEXT NOT NULL CHECK (evaluator IN ('deterministic', 'on_device_model', 'cloud_agent')),
    decided_at TEXT NOT NULL,
    expires_at TEXT,
    PRIMARY KEY (owner_id, message_id)
  ) WITHOUT ROWID;

  CREATE INDEX message_filter_decisions_delivery_idx
    ON message_filter_decisions (owner_id, decision, decided_at DESC);

  CREATE TABLE shadow_evaluations (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    old_version INTEGER NOT NULL CHECK (old_version > 0),
    candidate_version INTEGER NOT NULL CHECK (candidate_version > 0),
    started_at TEXT NOT NULL,
    ends_at TEXT NOT NULL,
    diff_summary_json TEXT NOT NULL CHECK (json_valid(diff_summary_json)),
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'applied', 'abandoned')),
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE TABLE temporary_focuses (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    concept TEXT NOT NULL CHECK (length(concept) BETWEEN 1 AND 120),
    delivery_mode TEXT NOT NULL CHECK (delivery_mode IN ('immediate', 'inbox', 'digest', 'suppress')),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    expired_at TEXT,
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;
`;

const signalAppetiteUiSchema = `
  CREATE TABLE signal_appetite_ui_state (
    owner_id TEXT NOT NULL PRIMARY KEY,
    teaching_onboarding_seen INTEGER NOT NULL DEFAULT 0
      CHECK (teaching_onboarding_seen IN (0, 1)),
    updated_at TEXT NOT NULL
  ) WITHOUT ROWID;
`;

const briefingSchema = `
  CREATE TABLE briefing_events (
    local_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
      'BriefingScheduled',
      'BriefingGenerationStarted',
      'BriefingGenerated',
      'BriefingOpened',
      'BriefingItemHandled',
      'BriefingDismissed',
      'AttentionSnapshotUpdated',
      'QuietItemAdded',
      'QuietItemRestored',
      'BriefingScheduleUpdated'
    )),
    aggregate_id TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL CHECK (aggregate_version > 0),
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json) AND length(payload_json) <= 262144),
    occurred_at TEXT NOT NULL,
    sync_status TEXT NOT NULL DEFAULT 'pending'
      CHECK (sync_status IN ('pending', 'synced', 'error')),
    server_cursor INTEGER CHECK (server_cursor IS NULL OR server_cursor > 0),
    UNIQUE (owner_id, event_id)
  );

  CREATE INDEX briefing_events_owner_sequence_idx
    ON briefing_events (owner_id, local_sequence);
  CREATE INDEX briefing_events_pending_idx
    ON briefing_events (owner_id, local_sequence)
    WHERE sync_status IN ('pending', 'error');

  CREATE TABLE briefings (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    briefing_type TEXT NOT NULL CHECK (briefing_type IN ('morning', 'midday', 'evening', 'ad_hoc', 'urgent')),
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    summary TEXT CHECK (summary IS NULL OR length(summary) <= 4000),
    covered_from TEXT NOT NULL,
    covered_to TEXT NOT NULL,
    generated_at TEXT NOT NULL,
    generated_by TEXT NOT NULL CHECK (generated_by IN ('local', 'cloud')),
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'generating', 'ready', 'dismissed')),
    total_messages INTEGER NOT NULL CHECK (total_messages >= 0),
    immediate_count INTEGER NOT NULL CHECK (immediate_count >= 0),
    inbox_count INTEGER NOT NULL CHECK (inbox_count >= 0),
    digest_count INTEGER NOT NULL CHECK (digest_count >= 0),
    suppressed_count INTEGER NOT NULL CHECK (suppressed_count >= 0),
    included_message_ids_json TEXT NOT NULL CHECK (json_valid(included_message_ids_json)),
    included_opportunity_ids_json TEXT NOT NULL CHECK (json_valid(included_opportunity_ids_json)),
    excluded_handled_ids_json TEXT NOT NULL CHECK (json_valid(excluded_handled_ids_json)),
    category_summaries_json TEXT NOT NULL CHECK (json_valid(category_summaries_json)),
    evidence_refs_json TEXT NOT NULL CHECK (json_valid(evidence_refs_json)),
    PRIMARY KEY (owner_id, id)
  ) WITHOUT ROWID;

  CREATE INDEX briefings_owner_covered_idx
    ON briefings (owner_id, covered_to DESC);

  CREATE TABLE briefing_items (
    owner_id TEXT NOT NULL,
    id TEXT NOT NULL,
    briefing_id TEXT NOT NULL,
    item_type TEXT NOT NULL CHECK (item_type IN ('message', 'opportunity')),
    entity_id TEXT NOT NULL,
    priority TEXT NOT NULL CHECK (priority IN ('action_required', 'worth_attention', 'needs_judgment', 'later')),
    reason_summary TEXT NOT NULL CHECK (length(reason_summary) <= 1000),
    action_required INTEGER NOT NULL CHECK (action_required IN (0, 1)),
    handled INTEGER NOT NULL DEFAULT 0 CHECK (handled IN (0, 1)),
    order_index INTEGER NOT NULL CHECK (order_index >= 0),
    PRIMARY KEY (owner_id, id),
    FOREIGN KEY (owner_id, briefing_id) REFERENCES briefings (owner_id, id) ON DELETE CASCADE
  ) WITHOUT ROWID;

  CREATE INDEX briefing_items_briefing_idx
    ON briefing_items (owner_id, briefing_id, order_index);

  CREATE TABLE briefing_schedules (
    owner_id TEXT NOT NULL,
    briefing_type TEXT NOT NULL CHECK (briefing_type IN ('morning', 'midday', 'evening')),
    minute_of_day INTEGER NOT NULL CHECK (minute_of_day BETWEEN 0 AND 1439),
    days_json TEXT NOT NULL CHECK (json_valid(days_json)),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, briefing_type)
  ) WITHOUT ROWID;
`;

export const radarMigrations: readonly RadarMigration[] = Object.freeze([
  {
    version: 1,
    name: 'initial_owner_scoped_sync_store',
    up: (executor) => executor.execAsync(initialSchema),
  },
  {
    version: 2,
    name: 'message_settings_and_resumable_bootstrap',
    up: (executor) => executor.execAsync(syncProjectionSchema),
  },
  {
    version: 3,
    name: 'internal_status_command_expiry',
    up: (executor) => executor.execAsync(internalCommandSchema),
  },
  {
    version: 4,
    name: 'device_analysis_run_recovery',
    up: (executor) => executor.execAsync(deviceAnalysisRunSchema),
  },
  {
    version: 5,
    name: 'interactive_agent_local_sessions',
    up: (executor) => executor.execAsync(interactiveAgentSessionSchema),
  },
  {
    version: 6,
    name: 'signal_appetite_event_log_and_projections',
    up: (executor) => executor.execAsync(signalAppetiteSchema),
  },
  {
    version: 7,
    name: 'signal_appetite_local_ui_state',
    up: (executor) => executor.execAsync(signalAppetiteUiSchema),
  },
  {
    version: 8,
    name: 'mira_briefing_event_log_and_projections',
    up: (executor) => executor.execAsync(briefingSchema),
  },
]);

function validateMigrations(migrations: readonly RadarMigration[]) {
  // Hermes in the current RN 0.86 release does not expose Array.prototype.toSorted yet.
  const sorted = Array.from(migrations).sort((left, right) => left.version - right.version);
  sorted.forEach((migration, index) => {
    if (migration.version !== index + 1) {
      throw new Error(`Database migrations must be contiguous from version 1; found ${migration.version}`);
    }
  });
  return sorted;
}

export async function runRadarMigrations(
  database: MigrationDatabase,
  migrations: readonly RadarMigration[] = radarMigrations,
) {
  await database.execAsync(`
    PRAGMA foreign_keys = ON;
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS schema_migrations (
      version INTEGER PRIMARY KEY NOT NULL,
      name TEXT NOT NULL,
      applied_at TEXT NOT NULL
    );
  `);

  const appliedRows = await database.getAllAsync<{ version: number }>(
    'SELECT version FROM schema_migrations ORDER BY version',
  );
  const applied = new Set(appliedRows.map((row) => row.version));

  for (const migration of validateMigrations(migrations)) {
    if (applied.has(migration.version)) continue;
    await database.withExclusiveTransactionAsync(async (transaction) => {
      await migration.up(transaction);
      await transaction.runAsync(
        'INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)',
        migration.version,
        migration.name,
        new Date().toISOString(),
      );
    });
  }
}
