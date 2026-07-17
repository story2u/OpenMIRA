import { describe, expect, it, vi } from 'vitest';

import { createRadarApiClient } from './client';
import {
  createTelegramApi,
  decodeTelegramConnectionHealth,
  decodeTelegramConnections,
} from './telegram';

const source = {
  id: '22222222-2222-4222-8222-222222222222',
  connectionId: '11111111-1111-4111-8111-111111111111',
  sourceType: 'group',
  externalChatId: '-10042',
  displayName: '采购群',
  username: null,
  enabled: true,
  quotaPaused: true,
  quotaReason: '当前套餐只保留一个监听来源',
  lastError: null,
  updatedAt: '2026-07-17T01:00:00Z',
} as const;

const connection = {
  id: source.connectionId,
  connectionType: 'bot_chat',
  status: 'connected',
  enabled: true,
  label: 'Telegram Bot',
  capabilities: { receive_group_messages: true },
  lastError: null,
  lastCheckedAt: '2026-07-17T01:00:00Z',
  updatedAt: '2026-07-17T01:00:00Z',
  sources: [source],
} as const;

const health = {
  mode: 'live',
  botConfigured: true,
  botUsername: 'story_radar_bot',
  businessAvailable: true,
  mtprotoQrAvailable: false,
  listenerMode: 'vps-long-running',
  legacyMonitoringActive: false,
  legacyActiveSourceCount: 0,
  message: null,
} as const;

describe('Telegram API', () => {
  it('strictly decodes health and bounded owner connections', () => {
    expect(decodeTelegramConnectionHealth(health)).toEqual(health);
    expect(decodeTelegramConnections([connection])).toEqual([connection]);
    expect(() => decodeTelegramConnectionHealth({ ...health, apiHash: 'secret' })).toThrow();
    expect(() => decodeTelegramConnections([connection, connection])).toThrow('duplicate');
    expect(() => decodeTelegramConnections([{
      ...connection,
      capabilities: { receive_group_messages: 'yes' },
    }])).toThrow();
  });

  it('loads health and connections in parallel-capable independent requests', async () => {
    const fetch = vi.fn(async (input: string) => Response.json(
      input.endsWith('/health') ? health : [connection],
    ));
    const api = createTelegramApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'access-token',
    }));

    await expect(Promise.all([api.health(), api.connections()])).resolves.toEqual([
      health,
      [connection],
    ]);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('validates ids before connection writes and replaces with server truth', async () => {
    const updated = { ...connection, enabled: false, status: 'disabled' } as const;
    const fetch = vi.fn(async (_input: string, _init?: RequestInit) => Response.json(updated));
    const api = createTelegramApi(createRadarApiClient({
      baseUrl: 'https://api.example.test',
      fetch,
      getAccessToken: () => 'access-token',
    }));

    await expect(api.updateConnection(connection.id, false)).resolves.toEqual(updated);
    expect(fetch.mock.calls[0][1]?.body).toBe(JSON.stringify({ enabled: false }));
    expect(() => api.updateConnection('not-a-uuid', false)).toThrow('Invalid Telegram');
  });
});
