import type {
  TelegramConnection,
  TelegramConnectionHealth,
} from '@story2u/radar-contracts/telegram';

export interface TelegramSettingsState {
  health: TelegramConnectionHealth | null;
  connections: TelegramConnection[];
  isLoading: boolean;
  error: string | null;
  actionId: string | null;
}

export const initialTelegramSettingsState: TelegramSettingsState = {
  health: null,
  connections: [],
  isLoading: true,
  error: null,
  actionId: null,
};

export type TelegramSettingsAction =
  | { type: 'load-started' }
  | { type: 'load-succeeded'; health: TelegramConnectionHealth; connections: TelegramConnection[] }
  | { type: 'load-failed'; error: string }
  | { type: 'toggle-started'; connectionId: string }
  | { type: 'toggle-succeeded'; connection: TelegramConnection }
  | { type: 'toggle-failed'; connectionId: string; error: string };

export function telegramSettingsReducer(
  state: TelegramSettingsState,
  action: TelegramSettingsAction,
): TelegramSettingsState {
  switch (action.type) {
    case 'load-started':
      return { ...state, isLoading: true, error: null };
    case 'load-succeeded':
      return {
        health: action.health,
        connections: action.connections,
        isLoading: false,
        error: null,
        actionId: null,
      };
    case 'load-failed':
      return { ...state, isLoading: false, error: action.error };
    case 'toggle-started':
      return { ...state, actionId: action.connectionId, error: null };
    case 'toggle-succeeded':
      if (state.actionId !== action.connection.id) return state;
      return {
        ...state,
        actionId: null,
        error: null,
        connections: state.connections.map((item) =>
          item.id === action.connection.id ? action.connection : item),
      };
    case 'toggle-failed':
      if (state.actionId !== action.connectionId) return state;
      return { ...state, actionId: null, error: action.error };
  }
}
