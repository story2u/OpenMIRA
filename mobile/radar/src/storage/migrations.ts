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
