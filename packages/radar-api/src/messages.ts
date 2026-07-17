import type {
  ChatMessage,
  MessagePage,
  MessagePageQuery,
} from '@story2u/radar-contracts/messages';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';

export const ChatMessageSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    senderName: Type.String(),
    content: Type.String(),
    isFromContact: Type.Boolean(),
    sentAt: Type.String({ pattern: dateTimePattern }),
    source: Type.Union([Type.Literal('human'), Type.Literal('ai'), Type.Null()]),
  },
  { additionalProperties: false },
);

export const MessagePageSchema = Type.Object(
  {
    items: Type.Array(ChatMessageSchema, { maxItems: 200 }),
    total: Type.Integer({ minimum: 0 }),
    limit: Type.Integer({ minimum: 1, maximum: 200 }),
    offset: Type.Integer({ minimum: 0 }),
  },
  { additionalProperties: false },
);

const parseMessagePage = typeboxDecoder(MessagePageSchema);

export const decodeMessagePageResponse: ResponseDecoder<MessagePage> = (value) => {
  const parsed = parseMessagePage(value);
  if (
    parsed.items.length > parsed.limit ||
    parsed.items.length > parsed.total ||
    (parsed.items.length > 0 && parsed.offset + parsed.items.length > parsed.total)
  ) {
    throw new Error('Message pagination metadata is inconsistent');
  }
  const ids = new Set(parsed.items.map((item) => item.id));
  if (ids.size !== parsed.items.length) throw new Error('Message page contains duplicate ids');
  for (let index = 1; index < parsed.items.length; index += 1) {
    if (Date.parse(parsed.items[index - 1].sentAt) > Date.parse(parsed.items[index].sentAt)) {
      throw new Error('Message page is not chronological');
    }
  }
  return parsed as MessagePage;
};

function requireInteger(value: number | undefined, minimum: number, maximum: number, label: string) {
  if (value !== undefined && (!Number.isInteger(value) || value < minimum || value > maximum)) {
    throw new Error(`Invalid ${label}`);
  }
}

export function messagePageRequestPath(query: MessagePageQuery) {
  if (!new RegExp(uuidPattern).test(query.opportunity_id)) {
    throw new Error('Invalid opportunity id');
  }
  requireInteger(query.limit, 1, 200, 'message limit');
  requireInteger(query.offset, 0, Number.MAX_SAFE_INTEGER, 'message offset');
  const params = new URLSearchParams();
  params.set('opportunity_id', query.opportunity_id);
  params.set('limit', String(query.limit ?? 50));
  params.set('offset', String(query.offset ?? 0));
  return `/api/v1/messages/page?${params.toString()}`;
}

export function createMessagesApi(client: RadarApiClient) {
  return {
    page(query: MessagePageQuery, init: Pick<RequestInit, 'signal'> = {}): Promise<MessagePage> {
      return client.request(messagePageRequestPath(query), {
        ...init,
        decode: decodeMessagePageResponse,
      });
    },
  };
}
