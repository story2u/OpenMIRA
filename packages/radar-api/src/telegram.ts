import type {
  TelegramConnection,
  TelegramConnectionAttempt,
  TelegramConnectionHealth,
  TelegramMtprotoDialog,
} from '@story2u/radar-contracts/telegram';
import { Type } from 'typebox';

import type { RadarApiClient, ResponseDecoder } from './client';
import { typeboxDecoder } from './typebox-decoder';

const uuidPattern = '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';
const dateTimePattern = '^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})$';
function nullableString(maxLength: number) {
  return Type.Union([Type.String({ maxLength }), Type.Null()]);
}
const nullableDateTime = Type.Union([Type.String({ pattern: dateTimePattern }), Type.Null()]);
const connectionType = Type.Union([
  Type.Literal('bot_chat'),
  Type.Literal('business'),
  Type.Literal('mtproto_qr'),
]);
const connectionStatus = Type.Union([
  Type.Literal('pending'),
  Type.Literal('connected'),
  Type.Literal('disabled'),
  Type.Literal('error'),
  Type.Literal('expired'),
]);

export const TelegramConnectionHealthSchema = Type.Object(
  {
    mode: Type.Union([Type.Literal('mock'), Type.Literal('live')]),
    botConfigured: Type.Boolean(),
    botUsername: nullableString(255),
    businessAvailable: Type.Boolean(),
    mtprotoQrAvailable: Type.Boolean(),
    listenerMode: Type.String({ minLength: 1, maxLength: 128 }),
    legacyMonitoringActive: Type.Boolean(),
    legacyActiveSourceCount: Type.Integer({ minimum: 0 }),
    message: nullableString(4000),
  },
  { additionalProperties: false },
);

export const TelegramConnectionSourceSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    connectionId: Type.String({ pattern: uuidPattern }),
    sourceType: Type.Union([
      Type.Literal('group'),
      Type.Literal('channel'),
      Type.Literal('private'),
    ]),
    externalChatId: Type.String({ maxLength: 256 }),
    displayName: Type.String({ minLength: 1, maxLength: 512 }),
    username: nullableString(255),
    enabled: Type.Boolean(),
    quotaPaused: Type.Boolean(),
    quotaReason: nullableString(500),
    lastError: nullableString(1000),
    updatedAt: Type.String({ pattern: dateTimePattern }),
  },
  { additionalProperties: false },
);

export const TelegramConnectionSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    connectionType,
    status: connectionStatus,
    enabled: Type.Boolean(),
    label: Type.String({ minLength: 1, maxLength: 512 }),
    capabilities: Type.Record(Type.String({ maxLength: 128 }), Type.Boolean(), { maxProperties: 64 }),
    lastError: nullableString(1000),
    lastCheckedAt: nullableDateTime,
    updatedAt: Type.String({ pattern: dateTimePattern }),
    sources: Type.Array(TelegramConnectionSourceSchema, { maxItems: 512 }),
  },
  { additionalProperties: false },
);

export const TelegramConnectionAttemptSchema = Type.Object(
  {
    id: Type.String({ pattern: uuidPattern }),
    connectionType,
    status: Type.Union([
      Type.Literal('pending'),
      Type.Literal('completed'),
      Type.Literal('cancelled'),
      Type.Literal('expired'),
      Type.Literal('failed'),
    ]),
    expiresAt: Type.String({ pattern: dateTimePattern }),
    connectionId: Type.Union([Type.String({ pattern: uuidPattern }), Type.Null()]),
    error: nullableString(1000),
    telegramUrl: nullableString(4096),
    qrCodeUrl: nullableString(4096),
    instructions: Type.Array(Type.String({ maxLength: 2000 }), { maxItems: 20 }),
    localMock: Type.Boolean(),
  },
  { additionalProperties: false },
);

export const TelegramMtprotoDialogSchema = Type.Object(
  {
    id: Type.String({ minLength: 1, maxLength: 128 }),
    sourceType: Type.Union([Type.Literal('group'), Type.Literal('channel')]),
    displayName: Type.String({ minLength: 1, maxLength: 512 }),
    username: nullableString(255),
  },
  { additionalProperties: false },
);

const parseHealth = typeboxDecoder(TelegramConnectionHealthSchema);
const parseConnection = typeboxDecoder(TelegramConnectionSchema);
const parseConnections = typeboxDecoder(Type.Array(TelegramConnectionSchema, { maxItems: 100 }));
const parseAttempt = typeboxDecoder(TelegramConnectionAttemptSchema);
const parseDialogs = typeboxDecoder(Type.Array(TelegramMtprotoDialogSchema, { maxItems: 100 }));

function requireUuid(value: string, label: string) {
  if (!new RegExp(uuidPattern).test(value)) throw new Error(`Invalid ${label}`);
}

function requireUniqueIds(items: readonly { id: string }[], label: string) {
  if (new Set(items.map((item) => item.id)).size !== items.length) {
    throw new Error(`${label} contains duplicate ids`);
  }
}

export const decodeTelegramConnectionHealth: ResponseDecoder<TelegramConnectionHealth> = (value) =>
  parseHealth(value) as TelegramConnectionHealth;
export const decodeTelegramConnection: ResponseDecoder<TelegramConnection> = (value) =>
  parseConnection(value) as TelegramConnection;
export const decodeTelegramConnections: ResponseDecoder<TelegramConnection[]> = (value) => {
  const parsed = parseConnections(value) as TelegramConnection[];
  requireUniqueIds(parsed, 'Telegram connections');
  parsed.forEach((connection) => requireUniqueIds(connection.sources, 'Telegram sources'));
  return parsed;
};
export const decodeTelegramConnectionAttempt: ResponseDecoder<TelegramConnectionAttempt> = (value) =>
  parseAttempt(value) as TelegramConnectionAttempt;
export const decodeTelegramMtprotoDialogs: ResponseDecoder<TelegramMtprotoDialog[]> = (value) => {
  const parsed = parseDialogs(value) as TelegramMtprotoDialog[];
  requireUniqueIds(parsed, 'Telegram dialogs');
  return parsed;
};

function connectionPath(connectionId: string, suffix = '') {
  requireUuid(connectionId, 'Telegram connection id');
  return `/api/v1/integrations/telegram/connections/${encodeURIComponent(connectionId)}${suffix}`;
}

function attemptPath(attemptId: string, suffix = '') {
  requireUuid(attemptId, 'Telegram connection attempt id');
  return `/api/v1/integrations/telegram/connect/attempts/${encodeURIComponent(attemptId)}${suffix}`;
}

export function createTelegramApi(client: RadarApiClient) {
  return {
    health(init: Pick<RequestInit, 'signal'> = {}): Promise<TelegramConnectionHealth> {
      return client.request('/api/v1/integrations/telegram/health', {
        ...init,
        decode: decodeTelegramConnectionHealth,
      });
    },

    connections(init: Pick<RequestInit, 'signal'> = {}): Promise<TelegramConnection[]> {
      return client.request('/api/v1/integrations/telegram/connections', {
        ...init,
        decode: decodeTelegramConnections,
      });
    },

    startBotChat(): Promise<TelegramConnectionAttempt> {
      return client.request('/api/v1/integrations/telegram/connect/bot-chat', {
        method: 'POST',
        decode: decodeTelegramConnectionAttempt,
      });
    },

    startBusiness(): Promise<TelegramConnectionAttempt> {
      return client.request('/api/v1/integrations/telegram/connect/business', {
        method: 'POST',
        decode: decodeTelegramConnectionAttempt,
      });
    },

    startMtprotoQr(): Promise<TelegramConnectionAttempt> {
      return client.request('/api/v1/integrations/telegram/connect/mtproto-qr', {
        method: 'POST',
        decode: decodeTelegramConnectionAttempt,
      });
    },

    attempt(attemptId: string): Promise<TelegramConnectionAttempt> {
      return client.request(attemptPath(attemptId), { decode: decodeTelegramConnectionAttempt });
    },

    cancelAttempt(attemptId: string): Promise<TelegramConnectionAttempt> {
      return client.request(attemptPath(attemptId, '/cancel'), {
        method: 'POST',
        decode: decodeTelegramConnectionAttempt,
      });
    },

    updateConnection(connectionId: string, enabled: boolean): Promise<TelegramConnection> {
      return client.request(connectionPath(connectionId), {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
        decode: decodeTelegramConnection,
      });
    },

    deleteConnection(connectionId: string): Promise<void> {
      return client.request(connectionPath(connectionId), { method: 'DELETE' });
    },

    deleteSource(sourceId: string): Promise<void> {
      requireUuid(sourceId, 'Telegram source id');
      return client.request(`/api/v1/integrations/telegram/sources/${encodeURIComponent(sourceId)}`, {
        method: 'DELETE',
      });
    },

    dialogs(connectionId: string): Promise<TelegramMtprotoDialog[]> {
      return client.request(connectionPath(connectionId, '/dialogs'), {
        decode: decodeTelegramMtprotoDialogs,
      });
    },

    addSource(connectionId: string, chatId: string): Promise<TelegramConnection> {
      const normalizedChatId = chatId.trim();
      if (!normalizedChatId || normalizedChatId.length > 128) throw new Error('Invalid Telegram chat id');
      return client.request(connectionPath(connectionId, '/sources'), {
        method: 'POST',
        body: JSON.stringify({ chatId: normalizedChatId }),
        decode: decodeTelegramConnection,
      });
    },
  };
}
