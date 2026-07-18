import { describe, expect, it } from 'vitest';

import {
  radarMigrations,
  runRadarMigrations,
  type MigrationDatabase,
  type MigrationExecutor,
  type RadarMigration,
} from './migrations';

class FakeDatabase implements MigrationDatabase {
  readonly statements: string[] = [];
  readonly versions = new Map<number, string>();

  async execAsync(source: string) {
    this.statements.push(source);
  }

  async runAsync(source: string, ...params: Array<string | number | null>) {
    this.statements.push(source);
    if (source.startsWith('INSERT INTO schema_migrations')) {
      this.versions.set(Number(params[0]), String(params[1]));
    }
  }

  async getAllAsync<Row>() {
    return [...this.versions.keys()].map((version) => ({ version })) as Row[];
  }

  async withExclusiveTransactionAsync(task: (transaction: MigrationExecutor) => Promise<void>) {
    const versions = new Map(this.versions);
    const statementCount = this.statements.length;
    try {
      await task(this);
    } catch (error) {
      this.versions.clear();
      versions.forEach((name, version) => this.versions.set(version, name));
      this.statements.splice(statementCount);
      throw error;
    }
  }
}

describe('runRadarMigrations', () => {
  it('creates an owner-scoped inbox, projection and idempotent outbox exactly once', async () => {
    const database = new FakeDatabase();
    await runRadarMigrations(database);
    await runRadarMigrations(database);

    expect([...database.versions.keys()]).toEqual([1, 2, 3, 4, 5, 6, 7]);
    const schema = database.statements.join('\n');
    expect(schema).toContain('CREATE TABLE change_inbox');
    expect(schema).toContain('CREATE TABLE opportunity_projection');
    expect(schema).toContain('CREATE TABLE message_projection');
    expect(schema).toContain('CREATE TABLE setting_projection');
    expect(schema).toContain('CREATE TABLE sync_bootstrap_state');
    expect(schema).toContain('CREATE TABLE client_capability_state');
    expect(schema).toContain('CREATE TABLE command_outbox');
    expect(schema.match(/owner_id TEXT NOT NULL/g)?.length).toBeGreaterThanOrEqual(4);
    expect(schema).toContain('UNIQUE (owner_id, idempotency_key)');
    expect(schema).toContain('ADD COLUMN expires_at TEXT');
    expect(schema).toContain('CREATE TABLE analysis_run_state');
    expect(schema).toContain('input_json TEXT NOT NULL');
    expect(schema).not.toMatch(/analysis_run_state[\s\S]*run_token/i);
    expect(schema).toContain('CREATE TABLE agent_sessions');
    expect(schema).toContain('CREATE TABLE agent_entries');
    expect(schema).toContain('REFERENCES agent_sessions (owner_id, id) ON DELETE CASCADE');
    expect(schema).toContain('CREATE TABLE attention_events');
    expect(schema).toContain('CREATE TABLE signal_appetite_ui_state');
    expect(schema).toContain('CREATE TABLE attention_preferences');
    expect(schema).toContain('CREATE TABLE attention_intents');
    expect(schema).toContain('CREATE TABLE teaching_sessions');
    expect(schema).toContain('CREATE TABLE preference_examples');
    expect(schema).toContain('CREATE TABLE message_filter_decisions');
    expect(schema).toContain('CREATE TABLE shadow_evaluations');
    expect(schema).toContain('CREATE TABLE temporary_focuses');
    expect(schema).toContain('UNIQUE (owner_id, event_id)');
  });

  it('rolls back the migration marker and schema statements when a migration fails', async () => {
    const database = new FakeDatabase();
    const failing: RadarMigration = {
      version: 1,
      name: 'fails_closed',
      async up(executor) {
        await executor.execAsync('CREATE TABLE partial (id TEXT);');
        throw new Error('migration failed');
      },
    };

    await expect(runRadarMigrations(database, [failing])).rejects.toThrow('migration failed');
    expect(database.versions.size).toBe(0);
    expect(database.statements.join('\n')).not.toContain('CREATE TABLE partial');
  });

  it('rejects migration gaps before applying application schema', async () => {
    const database = new FakeDatabase();
    await expect(runRadarMigrations(database, [{ ...radarMigrations[0], version: 2 }])).rejects.toThrow(
      'contiguous',
    );
    expect(database.versions.size).toBe(0);
  });
});
