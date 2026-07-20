import { DatabaseSync } from 'node:sqlite';

import { RadarApiError } from '@story2u/radar-api/client';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { SyncBootstrap } from '@story2u/radar-contracts/sync';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../../storage/migrations';
import { applyBootstrapPage, type SyncStoreDatabase, type SyncStoreExecutor } from '../../sync/syncStore';
import {
  executeInteractiveInternalTool,
  InteractiveInternalToolError,
  type InteractiveInternalToolDependencies,
} from './internalTools';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '11234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '21234567-89ab-cdef-0123-456789abcdef';
const now = new Date('2026-07-18T10:00:00.000Z');

class TestDatabase implements SyncStoreDatabase {
  readonly raw = new DatabaseSync(':memory:');

  async execAsync(source: string) { this.raw.exec(source); }
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

function opportunity(overrides: Partial<OpportunityDetail> = {}): OpportunityDetail {
  return {
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
    createdAt: '2026-07-18T09:00:00Z',
    updatedAt: '2026-07-18T09:01:00Z',
    sourceType: 'private',
    groupName: null,
    groupMemberRole: 'member',
    rawMessageLinks: [],
    linkVerification: {},
    extractedContacts: {},
    friendRequestStatus: 'n/a',
    sopStage: 'detected',
    trustScore: 88,
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
    ...overrides,
  };
}

function snapshot(payload = opportunity()): SyncBootstrap {
  return {
    watermarkCursor: 1,
    items: [{
      aggregateType: 'opportunity',
      aggregateId: opportunityId,
      aggregateVersion: 3,
      schemaVersion: 1,
      payload,
    }],
    hasMore: false,
    nextPageToken: null,
  };
}

function dependencies(overrides: Partial<InteractiveInternalToolDependencies> = {}) {
  return {
    claim: vi.fn(async () => opportunity({ assignedTo: ownerId })),
    ...overrides,
  } satisfies InteractiveInternalToolDependencies;
}

const randomId = () => '31234567-89ab-4def-8123-456789abcdef';

describe('interactive Agent internal tools', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    await applyBootstrapPage(database, ownerId, snapshot(), { restart: true });
  });

  it('creates a local-only draft without calling the server draft endpoint', async () => {
    const deps = dependencies();
    const result = await executeInteractiveInternalTool(database, {
      allowedTools: new Set(['draft_reply']),
      baseUrl: 'https://api.example.test',
      call: {
        name: 'draft_reply',
        arguments: { opportunity_id: opportunityId, text: '  Reviewed local draft  ' },
      },
      dependencies: deps,
      ownerId,
      randomId,
    });

    expect(result).toEqual({
      opportunity_id: opportunityId,
      draft: 'Reviewed local draft',
      state: 'local_only',
      sent: false,
    });
    expect(deps.claim).not.toHaveBeenCalled();
    expect(database.raw.prepare('SELECT COUNT(*) AS total FROM command_outbox').get())
      .toEqual({ total: 0 });
  });

  it('queues a version-bound status command and treats the same active target as idempotent', async () => {
    const deps = dependencies();
    const options = {
      allowedTools: new Set(['update_status'] as const),
      baseUrl: 'https://api.example.test',
      call: {
        name: 'update_status',
        arguments: { opportunity_id: opportunityId, status: 'following' },
      },
      dependencies: deps,
      now,
      ownerId,
      randomId,
    };
    await expect(executeInteractiveInternalTool(database, options)).resolves.toEqual({
      opportunity_id: opportunityId,
      status: 'following',
      state: 'queued',
    });
    await expect(executeInteractiveInternalTool(database, options)).resolves.toMatchObject({
      state: 'queued',
    });
    const row = database.raw.prepare(
      'SELECT expected_version, idempotency_key, payload_json, expires_at FROM command_outbox',
    ).get() as Record<string, unknown>;
    expect(row).toMatchObject({
      expected_version: 3,
      idempotency_key: 'agent-status:31234567-89ab-4def-8123-456789abcdef',
      payload_json: '{"status":"following"}',
      expires_at: '2026-07-25T10:00:00.000Z',
    });
  });

  it('claims through the authenticated API and returns only a bounded confirmation', async () => {
    const controller = new AbortController();
    const deps = dependencies();
    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(['claim_opportunity']),
      baseUrl: 'https://api.example.test',
      call: { name: 'claim_opportunity', arguments: { opportunity_id: opportunityId } },
      dependencies: deps,
      ownerId,
      randomId,
      signal: controller.signal,
    })).resolves.toEqual({
      opportunity_id: opportunityId,
      claimed: true,
      state: 'confirmed',
    });
    expect(deps.claim).toHaveBeenCalledWith(
      'https://api.example.test',
      opportunityId,
      controller.signal,
    );
  });

  it('fails closed for unauthorized, archived, corrupt, cross-owner and sanitized API errors', async () => {
    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(),
      baseUrl: 'https://api.example.test',
      call: { name: 'draft_reply', arguments: { opportunity_id: opportunityId, text: 'draft' } },
      ownerId,
      randomId,
    })).rejects.toMatchObject({ code: 'tool_not_authorized' });

    database.raw.prepare(
      'UPDATE opportunity_projection SET payload_json = ? WHERE owner_id = ? AND id = ?',
    ).run(JSON.stringify(opportunity({ archivedAt: now.toISOString() })), ownerId, opportunityId);
    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(['draft_reply']),
      baseUrl: 'https://api.example.test',
      call: { name: 'draft_reply', arguments: { opportunity_id: opportunityId, text: 'draft' } },
      ownerId,
      randomId,
    })).rejects.toMatchObject({ code: 'opportunity_archived' });

    database.raw.prepare(
      'UPDATE opportunity_projection SET payload_json = ? WHERE owner_id = ? AND id = ?',
    ).run('{}', ownerId, opportunityId);
    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(['draft_reply']),
      baseUrl: 'https://api.example.test',
      call: { name: 'draft_reply', arguments: { opportunity_id: opportunityId, text: 'draft' } },
      ownerId,
      randomId,
    })).rejects.toMatchObject({ code: 'local_projection_corrupt' });

    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(['draft_reply']),
      baseUrl: 'https://api.example.test',
      call: { name: 'draft_reply', arguments: { opportunity_id: opportunityId, text: 'draft' } },
      ownerId: otherOwnerId,
      randomId,
    })).rejects.toMatchObject({ code: 'local_projection_unavailable' });

    await applyBootstrapPage(database, ownerId, snapshot(), { restart: true });
    const secretError = new RadarApiError('provider secret body', 409, 'secret-request-id');
    await expect(executeInteractiveInternalTool(database, {
      allowedTools: new Set(['claim_opportunity']),
      baseUrl: 'https://api.example.test',
      call: { name: 'claim_opportunity', arguments: { opportunity_id: opportunityId } },
      dependencies: dependencies({ claim: vi.fn(async () => { throw secretError; }) }),
      ownerId,
      randomId,
    })).rejects.toEqual(new InteractiveInternalToolError('opportunity_conflict'));
  });
});
