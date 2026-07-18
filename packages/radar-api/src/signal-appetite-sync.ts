import type {
  SignalAppetiteEvent,
  SignalAppetiteEventsAppend,
  SignalAppetiteEventsAppendRequest,
  SignalAppetiteEventsPage,
  SignalAppetiteEventsQuery,
} from '@story2u/radar-contracts/signal-appetite-sync';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';

const EventTypeSchema = Type.Union([
  Type.Literal('TeachingSessionStarted'),
  Type.Literal('TeachingCardPresented'),
  Type.Literal('PreferenceExampleCaptured'),
  Type.Literal('PreferenceExampleReverted'),
  Type.Literal('TeachingSessionCompleted'),
  Type.Literal('PreferenceChangeProposed'),
  Type.Literal('PreferenceSimulationCompleted'),
  Type.Literal('PreferenceShadowStarted'),
  Type.Literal('PreferenceApplied'),
  Type.Literal('PreferenceReverted'),
  Type.Literal('MessageFilterDecisionMade'),
  Type.Literal('MessageDecisionCorrected'),
  Type.Literal('IntentMapUpdated'),
  Type.Literal('TemporaryFocusCreated'),
  Type.Literal('TemporaryFocusExpired'),
]);

const PayloadSchema = Type.Record(Type.String({ maxLength: 128 }), Type.Unknown(), {
  maxProperties: 128,
});

const EventWriteSchema = Type.Object({
  eventId: Type.String({ pattern: uuidPattern }),
  eventType: EventTypeSchema,
  aggregateId: Type.String({ pattern: uuidPattern }),
  aggregateVersion: Type.Integer({ minimum: 1 }),
  schemaVersion: Type.Literal(1),
  occurredAt: Type.String({ pattern: dateTimePattern }),
  payload: PayloadSchema,
}, { additionalProperties: false });

const EventSchema = Type.Object({
  ...EventWriteSchema.properties,
  ownerId: Type.String({ pattern: uuidPattern }),
  deviceId: Type.String({ pattern: uuidPattern }),
  cursor: Type.Integer({ minimum: 1 }),
  serverReceivedAt: Type.String({ pattern: dateTimePattern }),
}, { additionalProperties: false });

const AppendRequestSchema = Type.Object({
  events: Type.Array(EventWriteSchema, { minItems: 1, maxItems: 100 }),
}, { additionalProperties: false });

const AppendSchema = Type.Object({
  events: Type.Array(EventSchema, { maxItems: 100 }),
  serverCursor: Type.Integer({ minimum: 0 }),
}, { additionalProperties: false });

const PageSchema = Type.Object({
  events: Type.Array(EventSchema, { maxItems: 500 }),
  nextCursor: Type.Integer({ minimum: 0 }),
  serverCursor: Type.Integer({ minimum: 0 }),
  hasMore: Type.Boolean(),
}, { additionalProperties: false });

const parseRequest = typeboxDecoder(AppendRequestSchema);
const parseAppend = typeboxDecoder(AppendSchema);
const parsePage = typeboxDecoder(PageSchema);

function inspectPayload(value: unknown, depth = 0): void {
  if (depth > 12) throw new Error('Signal Appetite event payload is too deeply nested');
  if (Array.isArray(value)) {
    if (value.length > 500) throw new Error('Signal Appetite event payload list is too large');
    value.forEach((item) => inspectPayload(item, depth + 1));
    return;
  }
  if (!value || typeof value !== 'object') return;
  const entries = Object.entries(value);
  if (entries.length > 128) throw new Error('Signal Appetite event payload has too many fields');
  const forbidden = new Set(['body', 'content', 'raw', 'rawpayload', 'messagebody']);
  for (const [key, item] of entries) {
    if (forbidden.has(key.replaceAll('_', '').toLowerCase())) {
      throw new Error('Message content is forbidden in preference events');
    }
    inspectPayload(item, depth + 1);
  }
}

function validateEvents(events: readonly { payload: unknown }[]) {
  for (const event of events) inspectPayload(event.payload);
}

export const decodeSignalAppetiteEventsPage: ResponseDecoder<SignalAppetiteEventsPage> = (
  value,
) => {
  const parsed = parsePage(value);
  validateEvents(parsed.events);
  let previous = 0;
  const ids = new Set<string>();
  for (const event of parsed.events) {
    if (ids.has(event.eventId) || event.cursor <= previous || event.cursor > parsed.serverCursor) {
      throw new Error('Signal Appetite events are not unique and strictly ordered');
    }
    ids.add(event.eventId);
    previous = event.cursor;
  }
  if (parsed.events.length && parsed.nextCursor !== parsed.events.at(-1)?.cursor) {
    throw new Error('Signal Appetite next cursor does not match the last event');
  }
  if (parsed.nextCursor > parsed.serverCursor) {
    throw new Error('Signal Appetite next cursor exceeds the stream head');
  }
  return parsed as SignalAppetiteEventsPage;
};

function path(query: SignalAppetiteEventsQuery) {
  if (!Number.isSafeInteger(query.after) || query.after < 0) throw new Error('Invalid cursor');
  if (query.limit !== undefined && (!Number.isInteger(query.limit) || query.limit < 1 || query.limit > 500)) {
    throw new Error('Invalid limit');
  }
  const params = new URLSearchParams({
    after: String(query.after),
    limit: String(query.limit ?? 200),
  });
  return `/api/v1/sync/signal-appetite/events?${params.toString()}`;
}

export function createSignalAppetiteSyncApi(client: RadarApiClient) {
  return {
    append(
      input: SignalAppetiteEventsAppendRequest,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SignalAppetiteEventsAppend> {
      const payload = parseRequest(input);
      validateEvents(payload.events);
      return client.request('/api/v1/sync/signal-appetite/events', {
        ...init,
        method: 'POST',
        body: JSON.stringify(payload),
        decode: (value) => {
          const parsed = parseAppend(value);
          validateEvents(parsed.events);
          return parsed as SignalAppetiteEventsAppend;
        },
      });
    },
    list(
      query: SignalAppetiteEventsQuery,
      init: Pick<RequestInit, 'signal'> = {},
    ): Promise<SignalAppetiteEventsPage> {
      return client.request(path(query), { ...init, decode: decodeSignalAppetiteEventsPage });
    },
  };
}
