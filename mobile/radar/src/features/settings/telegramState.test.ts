import { describe, expect, it } from 'vitest';
import type { TelegramConnection } from '@story2u/radar-contracts/telegram';

import { initialTelegramSettingsState, telegramSettingsReducer } from './telegramState';

const health = {
  mode: 'live',
  botConfigured: true,
  botUsername: 'story_radar_bot',
  businessAvailable: false,
  mtprotoQrAvailable: false,
  listenerMode: 'vps-long-running',
  legacyMonitoringActive: false,
  legacyActiveSourceCount: 0,
  message: null,
} as const;

const connection: TelegramConnection = {
  id: '11111111-1111-4111-8111-111111111111',
  connectionType: 'bot_chat',
  status: 'connected',
  enabled: true,
  label: '采购群 Bot',
  capabilities: { receive_group_messages: true },
  lastError: null,
  lastCheckedAt: null,
  updatedAt: '2026-07-17T01:00:00Z',
  sources: [],
};

describe('Telegram settings reducer', () => {
  it('keeps the previous connection when a toggle fails', () => {
    const loaded = telegramSettingsReducer(initialTelegramSettingsState, {
      type: 'load-succeeded',
      health,
      connections: [connection],
    });
    const toggling = telegramSettingsReducer(loaded, {
      type: 'toggle-started',
      connectionId: connection.id,
    });
    const failed = telegramSettingsReducer(toggling, {
      type: 'toggle-failed',
      connectionId: connection.id,
      error: '更新失败',
    });

    expect(failed.connections[0]).toEqual(connection);
    expect(failed.actionId).toBeNull();
  });

  it('replaces a toggled connection with server truth', () => {
    const loaded = telegramSettingsReducer(initialTelegramSettingsState, {
      type: 'load-succeeded',
      health,
      connections: [connection],
    });
    const toggling = telegramSettingsReducer(loaded, {
      type: 'toggle-started',
      connectionId: connection.id,
    });
    const updated: TelegramConnection = { ...connection, enabled: false, status: 'disabled' };
    const next = telegramSettingsReducer(toggling, {
      type: 'toggle-succeeded',
      connection: updated,
    });

    expect(next.connections[0]).toEqual(updated);
  });
});
