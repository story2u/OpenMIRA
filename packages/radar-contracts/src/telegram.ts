import type { components } from './openapi';

export type TelegramConnectionType = components['schemas']['TelegramConnectionType'];
export type TelegramConnectionStatus = components['schemas']['TelegramConnectionStatus'];
export type TelegramConnectionAttemptStatus = components['schemas']['TelegramConnectionAttemptStatus'];
export type TelegramSourceType = components['schemas']['TelegramSourceType'];

type GeneratedTelegramConnectionHealth = components['schemas']['TelegramConnectionHealthRead'];
type GeneratedTelegramConnectionSource = components['schemas']['TelegramSourceRead'];
type GeneratedTelegramConnection = components['schemas']['TelegramConnectionRead'];
type GeneratedTelegramConnectionAttempt = components['schemas']['TelegramConnectionAttemptRead'];
type GeneratedTelegramMtprotoDialog = components['schemas']['TelegramMtprotoDialogRead'];

export interface TelegramConnectionHealth extends Omit<GeneratedTelegramConnectionHealth, 'botUsername' | 'message' | 'mode'> {
  mode: 'mock' | 'live';
  botUsername: string | null;
  message: string | null;
}

export interface TelegramConnectionSource extends Omit<GeneratedTelegramConnectionSource, 'lastError' | 'quotaReason' | 'username'> {
  lastError: string | null;
  quotaReason: string | null;
  username: string | null;
}

export interface TelegramConnection extends Omit<GeneratedTelegramConnection, 'capabilities' | 'lastCheckedAt' | 'lastError' | 'sources'> {
  capabilities: Record<string, boolean>;
  lastCheckedAt: string | null;
  lastError: string | null;
  sources: TelegramConnectionSource[];
}

export interface TelegramConnectionAttempt extends Omit<GeneratedTelegramConnectionAttempt, 'connectionId' | 'error' | 'instructions' | 'qrCodeUrl' | 'telegramUrl'> {
  connectionId: string | null;
  error: string | null;
  instructions: string[];
  qrCodeUrl: string | null;
  telegramUrl: string | null;
}

export interface TelegramMtprotoDialog extends Omit<GeneratedTelegramMtprotoDialog, 'username'> {
  username: string | null;
}
