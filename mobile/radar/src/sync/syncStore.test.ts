import { DatabaseSync } from 'node:sqlite';

import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type {
  SyncBootstrap,
  SyncChange,
  SyncMessagePayload,
  SyncSnapshotItem,
} from '@story2u/radar-contracts/sync';
import { beforeEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import { clearLocalUserDataInDatabase } from '../storage/userData';
import {
  applyBootstrapPage,
  applyChangePage,
  LocalSyncStateError,
  readLocalBootstrapState,
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '11234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '21234567-89ab-cdef-0123-456789abcdef';
const messageId = '31234567-89ab-cdef-0123-456789abcdef';

class TestDatabase implements SyncStoreDatabase {
  readonly raw = new DatabaseSync(':memory:');

  async execAsync(source: string) {
    this.raw.exec(source);
  }

  async runAsync(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).run(...params);
  }

  async getAllAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return this.raw.prepare(source).all(...params) as Row[];
  }

  async getFirstAsync<Row>(source: string, ...params: Array<string | number | null>) {
    return (this.raw.prepare(source).get(...params) as Row | undefined) ?? null;
  }

  async withExclusiveTransactionAsync(task: (transaction: SyncStoreExecutor) => Promise<void>) {
    this.raw.exec('BEGIN EXCLUSIVE');
    try {
      await task(this);
      this.raw.exec('COMMIT');
    } catch (error) {
      this.raw.exec('ROLLBACK');
      throw error;
    }
  }
}

const opportunity: OpportunityDetail = {
  id: opportunityId,
  opportunityType: 'business',
  platform: 'telegram',
  contactName: 'Customer',
  contactAvatar: '',
  summary: 'Needs a quote',
  matchedKeywords: ['quote'],
  confidenceScore: 0.9,
  status: 'pending',
  internalStatus: 'pending_human',
  priority: 'high',
  lastMessagePreview: 'Please quote',
  createdAt: '2026-07-17T10:00:00Z',
  updatedAt: '2026-07-17T10:00:01Z',
  sourceType: 'private',
  groupName: null,
  groupMemberRole: 'member',
  rawMessageLinks: [],
  linkVerification: {},
  extractedContacts: {},
  friendRequestStatus: 'n/a',
  sopStage: 'detected',
  trustScore: 72,
  agentActions: [],
  agentAnalysisStatus: 'not_requested',
  agentAnalysisError: null,
  agentAnalyzedAt: null,
  attentionRequired: true,
  archivedAt: null,
  archivedByUserId: null,
  archiveReason: null,
  aiReplyDraft: null,
  finalReply: null,
  detectionReason: 'Matched quote rule',
  assignedTo: null,
};

const message: SyncMessagePayload = {
  id: messageId,
  opportunityId,
  senderName: 'Customer',
  content: 'Please quote',
  isFromContact: true,
  sentAt: '2026-07-17T10:00:00Z',
  source: null,
};

function snapshotItem(
  aggregateType: SyncSnapshotItem['aggregateType'],
  aggregateId: string,
  aggregateVersion: number,
  payload: SyncSnapshotItem['payload'],
): SyncSnapshotItem {
  return { aggregateType, aggregateId, aggregateVersion, schemaVersion: 1, payload };
}

function bootstrap(
  items: SyncSnapshotItem[],
  options: { hasMore?: boolean; nextPageToken?: string | null; watermark?: number } = {},
): SyncBootstrap {
  return {
    watermarkCursor: options.watermark ?? 10,
    items,
    hasMore: options.hasMore ?? false,
    nextPageToken: options.nextPageToken ?? null,
  };
}

function change(
  overrides: Partial<SyncChange> & Pick<SyncChange, 'eventId' | 'cursor'>,
): SyncChange {
  const { eventId, cursor, ...remaining } = overrides;
  return {
    eventId,
    cursor,
    aggregateType: 'message',
    aggregateId: messageId,
    aggregateVersion: 2,
    operation: 'upsert',
    schemaVersion: 1,
    createdAt: '2026-07-17T10:01:00Z',
    payload: { ...message, content: 'Updated quote request' },
    ...remaining,
  } as SyncChange;
}

describe('SQLite sync store', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
  });

  it('keeps paged bootstrap hidden until the final page advances the watermark', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([
        snapshotItem('user_detection_preference', ownerId, 0, {
          keywords: [],
          aiSemanticsEnabled: true,
        }),
        snapshotItem('opportunity', opportunityId, 1, opportunity),
      ], { hasMore: true, nextPageToken: 'aaa.bbb.ccc' }),
      { restart: true },
    );

    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 0,
      phase: 'bootstrapping',
    });
    expect(await readLocalBootstrapState(database, ownerId)).toEqual({
      watermarkCursor: 10,
      nextPageToken: 'aaa.bbb.ccc',
    });

    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([snapshotItem('message', messageId, 1, message)]),
      { restart: false },
    );

    expect(await readLocalSyncState(database, ownerId)).toMatchObject({
      cursor: 10,
      phase: 'ready',
    });
    expect(await readLocalBootstrapState(database, ownerId)).toBeNull();
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS count FROM opportunity_projection WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ count: 1 });
    expect(database.raw.prepare(
      'SELECT opportunity_id, aggregate_version FROM message_projection WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ opportunity_id: opportunityId, aggregate_version: 1 });
  });

  it('applies inbox, projections and cursor atomically and idempotently', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([
        snapshotItem('opportunity', opportunityId, 1, opportunity),
        snapshotItem('message', messageId, 1, message),
      ]),
      { restart: true },
    );
    const page = {
      changes: [
        change({ eventId: '41234567-89ab-cdef-0123-456789abcdef', cursor: 11 }),
        change({
          eventId: '51234567-89ab-cdef-0123-456789abcdef',
          cursor: 12,
          aggregateType: 'opportunity',
          aggregateId: opportunityId,
          aggregateVersion: 2,
          operation: 'delete',
          payload: null,
        }),
      ],
      nextCursor: 12,
      resetRequired: false,
    };

    await applyChangePage(database, ownerId, 10, page);
    await applyChangePage(database, ownerId, 10, page);

    expect(await readLocalSyncState(database, ownerId)).toMatchObject({ cursor: 12 });
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS count FROM change_inbox WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ count: 2 });
    expect(database.raw.prepare(
      'SELECT aggregate_version, deleted_at FROM opportunity_projection WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ aggregate_version: 2 });
    expect(database.raw.prepare(
      'SELECT aggregate_version FROM message_projection WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ aggregate_version: 2 });
  });

  it('preserves pending internal commands while a reset rebuilds projections', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([snapshotItem('opportunity', opportunityId, 1, opportunity)]),
      { restart: true },
    );
    database.raw.prepare(
      `INSERT INTO command_outbox (
        owner_id, id, command_type, aggregate_type, aggregate_id,
        expected_version, idempotency_key, payload_json, status,
        attempt_count, created_at, updated_at, expires_at
      ) VALUES (?, ?, 'opportunity_status', 'opportunity', ?, 1, ?, ?,
        'pending', 0, ?, ?, ?)`,
    ).run(
      ownerId,
      '61234567-89ab-cdef-0123-456789abcdef',
      opportunityId,
      'status-command-reset',
      '{"status":"following"}',
      '2026-07-17T10:00:00Z',
      '2026-07-17T10:00:00Z',
      '2026-07-24T10:00:00Z',
    );

    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([snapshotItem('opportunity', opportunityId, 2, {
        ...opportunity,
        internalStatus: 'replied',
      })]),
      { restart: true },
    );

    expect(database.raw.prepare(
      'SELECT status, expected_version FROM command_outbox WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ status: 'pending', expected_version: 1 });
  });

  it('rolls back cursor and inbox on owner or event identity conflicts', async () => {
    await applyBootstrapPage(database, ownerId, bootstrap([]), { restart: true });
    const valid = change({
      eventId: '61234567-89ab-cdef-0123-456789abcdef',
      cursor: 11,
    });
    await applyChangePage(database, ownerId, 10, {
      changes: [valid],
      nextCursor: 11,
      resetRequired: false,
    });

    const conflicting = change({
      eventId: valid.eventId,
      cursor: 12,
      aggregateVersion: 3,
      payload: { ...message, content: 'Conflicting event identity' },
    });
    await expect(applyChangePage(database, ownerId, 11, {
      changes: [conflicting],
      nextCursor: 12,
      resetRequired: false,
    })).rejects.toMatchObject({ code: 'event_identity_conflict' });

    const foreignSetting = change({
      eventId: '71234567-89ab-cdef-0123-456789abcdef',
      cursor: 12,
      aggregateType: 'user_notification_preference',
      aggregateId: otherOwnerId,
      aggregateVersion: 1,
      payload: {
        newOpportunityEnabled: true,
        aiRepliedEnabled: true,
        dailyDigestEnabled: false,
        urgentOnly: false,
      },
    });
    await expect(applyChangePage(database, ownerId, 11, {
      changes: [foreignSetting],
      nextCursor: 12,
      resetRequired: false,
    })).rejects.toBeInstanceOf(LocalSyncStateError);

    expect(await readLocalSyncState(database, ownerId)).toMatchObject({ cursor: 11 });
    expect(database.raw.prepare(
      'SELECT COUNT(*) AS count FROM change_inbox WHERE owner_id = ?',
    ).get(ownerId)).toMatchObject({ count: 1 });
  });

  it('rejects unsupported aggregates without exposing a partial bootstrap', async () => {
    await expect(applyBootstrapPage(
      database,
      ownerId,
      bootstrap([snapshotItem('reply_template', opportunityId, 1, {
        id: opportunityId,
        title: 'Online only',
        content: 'Do not project offline',
        category: 'general',
      })]),
      { restart: true },
    )).rejects.toMatchObject({ code: 'unsupported_aggregate' });

    expect(await readLocalSyncState(database, ownerId)).toBeNull();
  });

  it('clears every table for the signed-out owner without touching the next account', async () => {
    await applyBootstrapPage(
      database,
      ownerId,
      bootstrap([snapshotItem('opportunity', opportunityId, 1, opportunity)]),
      { restart: true },
    );
    const otherOpportunityId = '41234567-89ab-cdef-0123-456789abcdef';
    await applyBootstrapPage(
      database,
      otherOwnerId,
      bootstrap([snapshotItem('opportunity', otherOpportunityId, 1, {
        ...opportunity,
        id: otherOpportunityId,
      })]),
      { restart: true },
    );
    await database.runAsync(
      `INSERT INTO client_capability_state (owner_id, sync_available, updated_at)
       VALUES (?, 1, ?)`,
      ownerId,
      '2026-07-17T10:00:00.000Z',
    );
    const sessionId = '51234567-89ab-cdef-0123-456789abcdef';
    await database.runAsync(
      `INSERT INTO agent_sessions (
        owner_id, id, opportunity_id, schema_version, title,
        created_at, updated_at, expires_at
      ) VALUES (?, ?, NULL, 1, ?, ?, ?, ?)`,
      ownerId,
      sessionId,
      'Local session',
      '2026-07-17T10:00:00.000Z',
      '2026-07-17T10:00:00.000Z',
      '2026-08-16T10:00:00.000Z',
    );
    await database.runAsync(
      `INSERT INTO agent_entries (
        owner_id, session_id, seq, entry_type, content_json, created_at
      ) VALUES (?, ?, 1, 'user', ?, ?)`,
      ownerId,
      sessionId,
      '{"type":"user","text":"local only"}',
      '2026-07-17T10:00:00.000Z',
    );

    await clearLocalUserDataInDatabase(database, ownerId);

    for (const table of [
      'agent_entries',
      'agent_sessions',
      'command_outbox',
      'message_projection',
      'setting_projection',
      'opportunity_projection',
      'change_inbox',
      'sync_bootstrap_state',
      'client_capability_state',
      'sync_state',
    ]) {
      expect(await database.getFirstAsync<{ count: number }>(
        `SELECT COUNT(*) AS count FROM ${table} WHERE owner_id = ?`,
        ownerId,
      )).toMatchObject({ count: 0 });
    }
    expect(await database.getFirstAsync<{ count: number }>(
      'SELECT COUNT(*) AS count FROM opportunity_projection WHERE owner_id = ?',
      otherOwnerId,
    )).toMatchObject({ count: 1 });
  });
});
