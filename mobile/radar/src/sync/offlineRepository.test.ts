import { DatabaseSync } from 'node:sqlite';

import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { SyncBootstrap, SyncSnapshotItem } from '@story2u/radar-contracts/sync';
import { beforeEach, describe, expect, it } from 'vitest';

import { runRadarMigrations, type MigrationDatabase } from '../storage/migrations';
import {
  LocalProjectionCorruptError,
  OfflineProjectionUnavailableError,
  readOfflineDashboard,
  readOfflineMessagePage,
  readOfflineOpportunityDetail,
  readOfflineProjectionWithRecovery,
  readOfflineSettings,
  searchOfflineOpportunities,
} from './offlineRepository';
import {
  InteractiveReadToolError,
  executeInteractiveReadOnlyTool,
} from '../agent/interactive/readOnlyTools';
import {
  applyBootstrapPage,
  readLocalSyncState,
  type SyncStoreDatabase,
  type SyncStoreExecutor,
} from './syncStore';

const ownerId = '01234567-89ab-cdef-0123-456789abcdef';
const otherOwnerId = '41234567-89ab-cdef-0123-456789abcdef';
const opportunityId = '11234567-89ab-cdef-0123-456789abcdef';
const secondOpportunityId = '21234567-89ab-cdef-0123-456789abcdef';
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

function opportunity(
  id: string,
  overrides: Partial<OpportunityDetail> = {},
): OpportunityDetail {
  return {
    id,
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

function item(
  aggregateType: SyncSnapshotItem['aggregateType'],
  aggregateId: string,
  payload: SyncSnapshotItem['payload'],
): SyncSnapshotItem {
  return { aggregateType, aggregateId, aggregateVersion: 1, schemaVersion: 1, payload };
}

function snapshot(options: { partial?: boolean } = {}): SyncBootstrap {
  const items: SyncSnapshotItem[] = [
    item('user_detection_preference', ownerId, {
      keywords: ['quote'],
      aiSemanticsEnabled: true,
    }),
    item('user_work_schedule', ownerId, {
      timezone: 'Asia/Shanghai',
      slots: [],
      autoReplyOutsideHours: true,
      isDefault: true,
    }),
    item('user_notification_preference', ownerId, {
      newOpportunityEnabled: true,
      aiRepliedEnabled: true,
      dailyDigestEnabled: false,
      urgentOnly: false,
    }),
    item('opportunity', opportunityId, opportunity(opportunityId)),
    item('opportunity', secondOpportunityId, opportunity(secondOpportunityId, {
      platform: 'wecom',
      status: 'replied',
      internalStatus: 'following',
      trustScore: 65,
      attentionRequired: false,
      matchedKeywords: ['integration'],
      createdAt: '2026-07-16T10:00:00Z',
      updatedAt: '2026-07-16T10:01:00Z',
    })),
    item('message', messageId, {
      id: messageId,
      opportunityId,
      senderName: 'Customer',
      content: 'Please quote',
      isFromContact: true,
      sentAt: '2026-07-17T10:00:00Z',
      source: null,
    }),
  ];
  return {
    watermarkCursor: 10,
    items,
    hasMore: options.partial ?? false,
    nextPageToken: options.partial ? 'next.page.token' : null,
  };
}

describe('offline projection repository', () => {
  let database: TestDatabase;

  beforeEach(async () => {
    database = new TestDatabase();
    await runRadarMigrations(database as unknown as MigrationDatabase);
    await applyBootstrapPage(database, ownerId, snapshot(), { restart: true });
  });

  it('serves bounded dashboard filters and aggregate metadata from ready projections', async () => {
    const dashboard = await readOfflineDashboard(database, ownerId, {
      status: 'pending',
      platform: 'telegram',
      trust_levels: ['trusted'],
      keywords: ['quote'],
      sort: 'trust',
      limit: 20,
      offset: 0,
    });

    expect(dashboard.items.map((entry) => entry.id)).toEqual([opportunityId]);
    expect(dashboard.items[0].opportunityType).toBe('business');
    expect(dashboard.total).toBe(1);
    expect(dashboard.pendingCount).toBe(1);
    expect(dashboard.attentionItems?.map((entry) => entry.id)).toEqual([opportunityId]);
    expect(dashboard.keywordOptions).toEqual(['integration', 'quote']);
  });

  it('serves detail, chronological messages and fail-closed offline settings', async () => {
    const detail = await readOfflineOpportunityDetail(database, ownerId, opportunityId);
    const messages = await readOfflineMessagePage(database, ownerId, opportunityId);
    const settings = await readOfflineSettings(database, ownerId);

    expect(detail?.id).toBe(opportunityId);
    expect(detail?.opportunityType).toBe('business');
    expect(messages).toMatchObject({ total: 1, limit: 20, offset: 0 });
    expect(messages.items[0]).toMatchObject({ id: messageId, content: 'Please quote' });
    expect(settings.detection.keywords).toEqual(['quote']);
    expect(settings.capabilities).toEqual({
      pushAvailable: false,
      wecomUserBindingAvailable: false,
    });
  });

  it('backfills missing opportunityType for pre-upgrade offline projections', async () => {
    const row = await database.getFirstAsync<{ payload_json: string }>(
      'SELECT payload_json FROM opportunity_projection WHERE owner_id = ? AND id = ?',
      ownerId,
      opportunityId,
    );
    const payload = JSON.parse(row!.payload_json);
    delete payload.opportunityType;
    await database.runAsync(
      'UPDATE opportunity_projection SET payload_json = ? WHERE owner_id = ? AND id = ?',
      JSON.stringify(payload),
      ownerId,
      opportunityId,
    );

    const detail = await readOfflineOpportunityDetail(database, ownerId, opportunityId);

    expect(detail?.opportunityType).toBe('business');
  });

  it('serves owner-bound bounded search and the three read-only Agent tools', async () => {
    const allowed = new Set([
      'search_opportunities',
      'get_opportunity',
      'get_messages',
    ] as const);
    const search = await searchOfflineOpportunities(database, ownerId, 'INTEGRATION', 10);
    const toolSearch = await executeInteractiveReadOnlyTool(database, ownerId, allowed, {
      name: 'search_opportunities',
      arguments: { query: 'integration', limit: 10 },
    });
    const toolDetail = await executeInteractiveReadOnlyTool(database, ownerId, allowed, {
      name: 'get_opportunity',
      arguments: { opportunity_id: opportunityId },
    });
    const toolMessages = await executeInteractiveReadOnlyTool(database, ownerId, allowed, {
      name: 'get_messages',
      arguments: { opportunity_id: opportunityId, limit: 20, offset: 0 },
    });

    expect(search.map((entry) => entry.id)).toEqual([secondOpportunityId]);
    expect(toolSearch).toMatchObject({ count: 1 });
    expect(toolDetail).toMatchObject({ opportunity: { id: opportunityId } });
    expect(toolMessages).toMatchObject({
      total: 1,
      hasMore: false,
      messages: [{ id: messageId, content: 'Please quote' }],
    });
    expect(JSON.stringify(toolDetail)).not.toContain(ownerId);
  });

  it('fails closed for unknown, unauthorized, invalid and cross-owner tool calls', async () => {
    const all = new Set([
      'search_opportunities',
      'get_opportunity',
      'get_messages',
    ] as const);
    await expect(executeInteractiveReadOnlyTool(database, ownerId, all, {
      name: 'send_reply',
      arguments: { opportunity_id: opportunityId, text: 'Do not send' },
    })).rejects.toMatchObject({ code: 'unknown_tool' });
    await expect(executeInteractiveReadOnlyTool(database, ownerId, new Set(), {
      name: 'get_opportunity',
      arguments: { opportunity_id: opportunityId },
    })).rejects.toMatchObject({ code: 'tool_not_authorized' });
    await expect(executeInteractiveReadOnlyTool(database, ownerId, all, {
      name: 'get_opportunity',
      arguments: { opportunity_id: opportunityId, owner_id: ownerId },
    })).rejects.toMatchObject({ code: 'invalid_tool_arguments' });
    await expect(executeInteractiveReadOnlyTool(database, otherOwnerId, all, {
      name: 'get_opportunity',
      arguments: { opportunity_id: opportunityId },
    })).rejects.toBeInstanceOf(InteractiveReadToolError);
  });

  it('does not expose a partial bootstrap or another owner', async () => {
    await applyBootstrapPage(database, ownerId, snapshot({ partial: true }), { restart: true });

    await expect(readOfflineDashboard(database, ownerId)).rejects.toBeInstanceOf(
      OfflineProjectionUnavailableError,
    );
    await expect(readOfflineDashboard(
      database,
      '41234567-89ab-cdef-0123-456789abcdef',
    )).rejects.toBeInstanceOf(OfflineProjectionUnavailableError);
  });

  it('detects valid-JSON projection corruption instead of returning partial data', async () => {
    database.raw.prepare(
      'UPDATE opportunity_projection SET payload_json = ? WHERE owner_id = ? AND id = ?',
    ).run('{}', ownerId, opportunityId);

    await expect(readOfflineProjectionWithRecovery(
      database,
      ownerId,
      () => readOfflineOpportunityDetail(database, ownerId, opportunityId),
    )).rejects.toBeInstanceOf(LocalProjectionCorruptError);
    await expect(readLocalSyncState(database, ownerId)).resolves.toMatchObject({
      phase: 'error',
      lastErrorCode: 'projection_corrupt',
    });
  });
});
