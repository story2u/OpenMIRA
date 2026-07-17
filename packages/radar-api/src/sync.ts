import type {
  SyncAck,
  SyncAckRequest,
  SyncBootstrap,
  SyncBootstrapQuery,
  SyncChanges,
  SyncChangesQuery,
} from '@story2u/radar-contracts/sync';
import { Type } from 'typebox';
import type { TSchema } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { ChatMessageSchema } from './messages';
import { OpportunityDetailSchema } from './opportunities';
import {
  DetectionSettingsSchema,
  NotificationSettingsSchema,
  WorkScheduleSchema,
} from './settings';
import { ReplyTemplateSchema } from './templates';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';
const pageTokenPattern = '^[A-Za-z0-9._-]+$';
const errorCodePattern = '^[a-z0-9][a-z0-9._-]*$';

const AggregateTypeSchema = Type.Union([
  Type.Literal('opportunity'),
  Type.Literal('message'),
  Type.Literal('user_detection_preference'),
  Type.Literal('user_work_schedule'),
  Type.Literal('user_notification_preference'),
  Type.Literal('reply_template'),
]);

const SyncMessagePayloadSchema = Type.Object(
  {
    ...ChatMessageSchema.properties,
    opportunityId: Type.Union([Type.String({ pattern: uuidPattern }), Type.Null()]),
  },
  { additionalProperties: false },
);

function snapshotVariant(aggregateType: string, payload: TSchema) {
  return Type.Object(
    {
      aggregateType: Type.Literal(aggregateType),
      aggregateId: Type.String({ pattern: uuidPattern }),
      aggregateVersion: Type.Integer({ minimum: 0 }),
      schemaVersion: Type.Literal(1),
      payload,
    },
    { additionalProperties: false },
  );
}

export const SyncSnapshotItemSchema = Type.Union([
  snapshotVariant('opportunity', OpportunityDetailSchema),
  snapshotVariant('message', SyncMessagePayloadSchema),
  snapshotVariant('user_detection_preference', DetectionSettingsSchema),
  snapshotVariant('user_work_schedule', WorkScheduleSchema),
  snapshotVariant('user_notification_preference', NotificationSettingsSchema),
  snapshotVariant('reply_template', ReplyTemplateSchema),
]);

const changeBase = {
  eventId: Type.String({ pattern: uuidPattern }),
  cursor: Type.Integer({ minimum: 1 }),
  aggregateId: Type.String({ pattern: uuidPattern }),
  aggregateVersion: Type.Integer({ minimum: 1 }),
  schemaVersion: Type.Literal(1),
  createdAt: Type.String({ pattern: dateTimePattern }),
};

function upsertVariant(aggregateType: string, payload: TSchema) {
  return Type.Object(
    {
      ...changeBase,
      aggregateType: Type.Literal(aggregateType),
      operation: Type.Literal('upsert'),
      payload,
    },
    { additionalProperties: false },
  );
}

export const SyncChangeSchema = Type.Union([
  upsertVariant('opportunity', OpportunityDetailSchema),
  upsertVariant('message', SyncMessagePayloadSchema),
  upsertVariant('user_detection_preference', DetectionSettingsSchema),
  upsertVariant('user_work_schedule', WorkScheduleSchema),
  upsertVariant('user_notification_preference', NotificationSettingsSchema),
  upsertVariant('reply_template', ReplyTemplateSchema),
  Type.Object(
    {
      ...changeBase,
      aggregateType: AggregateTypeSchema,
      operation: Type.Literal('delete'),
      payload: Type.Null(),
    },
    { additionalProperties: false },
  ),
]);

export const SyncBootstrapSchema = Type.Object(
  {
    watermarkCursor: Type.Integer({ minimum: 0 }),
    items: Type.Array(SyncSnapshotItemSchema, { maxItems: 500 }),
    nextPageToken: Type.Union([
      Type.String({ minLength: 1, maxLength: 2048, pattern: pageTokenPattern }),
      Type.Null(),
    ]),
    hasMore: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const SyncChangesSchema = Type.Object(
  {
    changes: Type.Array(SyncChangeSchema, { maxItems: 500 }),
    nextCursor: Type.Integer({ minimum: 0 }),
    serverCursor: Type.Integer({ minimum: 0 }),
    hasMore: Type.Boolean(),
    resetRequired: Type.Boolean(),
    resetReason: Type.Union([
      Type.Literal('cursor_expired'),
      Type.Literal('cursor_ahead'),
      Type.Null(),
    ]),
  },
  { additionalProperties: false },
);

export const SyncAckRequestSchema = Type.Object(
  {
    cursor: Type.Integer({ minimum: 0 }),
    errorCode: Type.Optional(Type.Union([
      Type.String({ minLength: 1, maxLength: 64, pattern: errorCodePattern }),
      Type.Null(),
    ])),
  },
  { additionalProperties: false },
);

export const SyncAckSchema = Type.Object(
  {
    deviceId: Type.String({ pattern: uuidPattern }),
    acknowledgedCursor: Type.Integer({ minimum: 0 }),
    acknowledgedAt: Type.String({ pattern: dateTimePattern }),
    errorCode: Type.Union([
      Type.String({ minLength: 1, maxLength: 64, pattern: errorCodePattern }),
      Type.Null(),
    ]),
  },
  { additionalProperties: false },
);

const parseBootstrap = typeboxDecoder(SyncBootstrapSchema);
const parseChanges = typeboxDecoder(SyncChangesSchema);
const parseSyncMessagePayload = typeboxDecoder(SyncMessagePayloadSchema);
const parseAckRequest = typeboxDecoder(SyncAckRequestSchema);
const parseAck = typeboxDecoder(SyncAckSchema);

export const decodeSyncMessagePayload = (value: unknown) => parseSyncMessagePayload(value);

const snapshotTypeOrder = new Map([
  ['user_detection_preference', 0],
  ['user_work_schedule', 1],
  ['user_notification_preference', 2],
  ['opportunity', 3],
  ['message', 4],
  ['reply_template', 5],
]);

export const decodeSyncBootstrap: ResponseDecoder<SyncBootstrap> = (value) => {
  const parsed = parseBootstrap(value);
  if (parsed.hasMore !== (parsed.nextPageToken !== null)) {
    throw new Error('Sync bootstrap continuation metadata is inconsistent');
  }
  const identities = new Set<string>();
  let previous: { typeOrder: number; id: string } | null = null;
  for (const item of parsed.items) {
    const identity = `${item.aggregateType}:${item.aggregateId}`;
    if (identities.has(identity)) throw new Error('Sync bootstrap contains duplicate aggregates');
    identities.add(identity);
    const current = {
      typeOrder: snapshotTypeOrder.get(item.aggregateType) ?? Number.MAX_SAFE_INTEGER,
      id: item.aggregateId.toLowerCase(),
    };
    if (
      previous &&
      (current.typeOrder < previous.typeOrder ||
        (current.typeOrder === previous.typeOrder && current.id <= previous.id))
    ) {
      throw new Error('Sync bootstrap page is not ordered');
    }
    previous = current;
  }
  return parsed as SyncBootstrap;
};

export const decodeSyncChanges: ResponseDecoder<SyncChanges> = (value) => {
  const parsed = parseChanges(value);
  if (parsed.resetRequired) {
    if (parsed.resetReason === null || parsed.changes.length > 0 || parsed.hasMore) {
      throw new Error('Sync reset metadata is inconsistent');
    }
    return parsed as SyncChanges;
  }
  if (parsed.resetReason !== null) throw new Error('Unexpected sync reset reason');
  const eventIds = new Set<string>();
  let previousCursor = 0;
  for (const change of parsed.changes) {
    if (eventIds.has(change.eventId)) throw new Error('Sync changes contain duplicate events');
    eventIds.add(change.eventId);
    if (change.cursor <= previousCursor || change.cursor > parsed.serverCursor) {
      throw new Error('Sync changes are not strictly ordered');
    }
    previousCursor = change.cursor;
  }
  if (
    parsed.changes.length > 0 &&
    parsed.nextCursor !== parsed.changes[parsed.changes.length - 1].cursor
  ) {
    throw new Error('Sync next cursor does not match the last change');
  }
  if (parsed.nextCursor > parsed.serverCursor) {
    throw new Error('Sync next cursor exceeds the stream head');
  }
  return parsed as SyncChanges;
};

function boundedInteger(value: number | undefined, minimum: number, maximum: number, label: string) {
  if (value !== undefined && (!Number.isInteger(value) || value < minimum || value > maximum)) {
    throw new Error(`Invalid ${label}`);
  }
}

export function syncBootstrapPath(query: SyncBootstrapQuery = {}) {
  boundedInteger(query.limit, 1, 500, 'sync bootstrap limit');
  if (query.pageToken && (query.pageToken.length > 2048 || !new RegExp(pageTokenPattern).test(query.pageToken))) {
    throw new Error('Invalid sync bootstrap page token');
  }
  const params = new URLSearchParams();
  params.set('limit', String(query.limit ?? 200));
  if (query.pageToken) params.set('pageToken', query.pageToken);
  return `/api/v1/sync/bootstrap?${params.toString()}`;
}

export function syncChangesPath(query: SyncChangesQuery) {
  boundedInteger(query.after, 0, Number.MAX_SAFE_INTEGER, 'sync cursor');
  boundedInteger(query.limit, 1, 500, 'sync changes limit');
  const params = new URLSearchParams();
  params.set('after', String(query.after));
  params.set('limit', String(query.limit ?? 200));
  return `/api/v1/sync/changes?${params.toString()}`;
}

export function createSyncApi(client: RadarApiClient) {
  return {
    bootstrap(
      query: SyncBootstrapQuery = {},
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SyncBootstrap> {
      return client.request(syncBootstrapPath(query), { ...init, decode: decodeSyncBootstrap });
    },

    changes(
      query: SyncChangesQuery,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SyncChanges> {
      return client.request(syncChangesPath(query), { ...init, decode: decodeSyncChanges });
    },

    acknowledge(
      input: SyncAckRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SyncAck> {
      const payload = parseAckRequest(input) as SyncAckRequest;
      return client.request('/api/v1/sync/ack', {
        ...init,
        method: 'POST',
        body: JSON.stringify(payload),
        decode: (value) => parseAck(value) as SyncAck,
      });
    },
  };
}
