import type {
  DetectionSettings,
  NotificationSettings,
  SettingsBundle,
  WorkSchedule,
} from '@story2u/radar-contracts/settings';

export type SettingsActionKind = 'detection' | 'work-schedule' | 'notifications';

export interface SettingsState {
  bundle: SettingsBundle | null;
  isLoading: boolean;
  loadError: string | null;
  busyAction: SettingsActionKind | null;
  saveError: string | null;
}

export const initialSettingsState: SettingsState = {
  bundle: null,
  isLoading: true,
  loadError: null,
  busyAction: null,
  saveError: null,
};

export type SettingsAction =
  | { type: 'load-started' }
  | { type: 'load-succeeded'; bundle: SettingsBundle }
  | { type: 'load-failed'; error: string }
  | { type: 'save-started'; kind: SettingsActionKind }
  | { type: 'detection-succeeded'; value: DetectionSettings }
  | { type: 'work-schedule-succeeded'; value: WorkSchedule }
  | { type: 'notifications-succeeded'; value: NotificationSettings }
  | { type: 'save-failed'; kind: SettingsActionKind; error: string };

export function settingsReducer(state: SettingsState, action: SettingsAction): SettingsState {
  switch (action.type) {
    case 'load-started':
      return { ...state, isLoading: true, loadError: null };
    case 'load-succeeded':
      return {
        bundle: action.bundle,
        isLoading: false,
        loadError: null,
        busyAction: null,
        saveError: null,
      };
    case 'load-failed':
      return { ...state, isLoading: false, loadError: action.error };
    case 'save-started':
      return { ...state, busyAction: action.kind, saveError: null };
    case 'detection-succeeded':
      return state.bundle ? {
        ...state,
        bundle: { ...state.bundle, detection: action.value },
        busyAction: null,
        saveError: null,
      } : state;
    case 'work-schedule-succeeded':
      return state.bundle ? {
        ...state,
        bundle: { ...state.bundle, workSchedule: action.value },
        busyAction: null,
        saveError: null,
      } : state;
    case 'notifications-succeeded':
      return state.bundle ? {
        ...state,
        bundle: { ...state.bundle, notifications: action.value },
        busyAction: null,
        saveError: null,
      } : state;
    case 'save-failed':
      if (state.busyAction !== action.kind) return state;
      return { ...state, busyAction: null, saveError: action.error };
  }
}
