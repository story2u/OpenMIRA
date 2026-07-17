import { RadarApiError } from '@story2u/radar-api/client';
import type {
  DetectionSettingsUpdate,
  NotificationSettingsUpdate,
  WorkScheduleUpdate,
} from '@story2u/radar-contracts/settings';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  type ReactNode,
} from 'react';

import {
  saveDetectionSettings,
  saveNotificationSettings,
  saveWorkSchedule,
} from '../../api/client';
import { useSession } from '../../auth/SessionProvider';
import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import type { Translator } from '../../i18n/core';
import { logEvent } from '../../logging/redactedLogger';
import { readSettingsResilient } from '../../sync/resilientReads';
import {
  initialSettingsState,
  settingsReducer,
  type SettingsActionKind,
  type SettingsState,
} from './state';

interface SettingsContextValue {
  state: SettingsState;
  retry(): void;
  saveDetection(input: DetectionSettingsUpdate): Promise<boolean>;
  saveSchedule(input: WorkScheduleUpdate): Promise<boolean>;
  saveNotifications(input: NotificationSettingsUpdate): Promise<boolean>;
}

const SettingsContext = createContext<SettingsContextValue | null>(null);

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

function loadErrorMessage(error: unknown, t: Translator) {
  if (error instanceof RadarApiError) {
    if (error.status === 404) return t('settings.error.notSupported');
    if (error.status >= 500) return t('settings.error.unavailable');
  }
  return t('settings.error.network');
}

function saveErrorMessage(error: unknown, t: Translator) {
  if (error instanceof RadarApiError) {
    if (error.status === 422) return t('settings.error.invalidSave');
    if (error.status >= 500) return t('settings.error.unavailableSave');
  }
  return t('settings.error.save');
}

export function SettingsProvider({ children }: { children: ReactNode }) {
  const {
    capabilities,
    expireSession,
    state: sessionState,
    synchronize,
  } = useSession();
  const { t } = useI18n();
  const [state, dispatch] = useReducer(settingsReducer, initialSettingsState);
  const [revision, retry] = useReducer((value: number) => value + 1, 0);
  const actionRunning = useRef<SettingsActionKind | null>(null);
  const syncOnNextLoad = useRef(false);
  const ownerId = sessionState.status === 'authenticated' ? sessionState.user.id : '';

  const retryWithSync = useCallback(() => {
    syncOnNextLoad.current = true;
    retry();
  }, []);

  const expireIfUnauthorized = useCallback(async (error: unknown) => {
    if (error instanceof RadarApiError && error.status === 401) {
      await expireSession();
      return true;
    }
    return false;
  }, [expireSession]);

  useEffect(() => {
    const controller = new AbortController();
    dispatch({ type: 'load-started' });

    async function load() {
      try {
        if (syncOnNextLoad.current) {
          syncOnNextLoad.current = false;
          await synchronize();
        }
        const bundle = await readSettingsResilient(
          getMobileApiBaseUrl(),
          { enabled: capabilities.syncAvailable, ownerId },
          controller.signal,
        );
        dispatch({ type: 'load-succeeded', bundle });
      } catch (error) {
        if (isAbortError(error)) return;
        if (await expireIfUnauthorized(error)) return;
        logEvent('settings.load_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({ type: 'load-failed', error: loadErrorMessage(error, t) });
      }
    }

    void load();
    return () => controller.abort();
  }, [capabilities.syncAvailable, expireIfUnauthorized, ownerId, revision, synchronize, t]);

  const failSave = useCallback(async (kind: SettingsActionKind, error: unknown) => {
    if (await expireIfUnauthorized(error)) return;
    logEvent('settings.save_failed', {
      action: kind,
      errorClass: error instanceof Error ? error.name : 'UnknownError',
      status: error instanceof RadarApiError ? error.status : null,
    });
    dispatch({ type: 'save-failed', kind, error: saveErrorMessage(error, t) });
  }, [expireIfUnauthorized, t]);

  const saveDetection = useCallback(async (input: DetectionSettingsUpdate) => {
    if (actionRunning.current) return false;
    actionRunning.current = 'detection';
    dispatch({ type: 'save-started', kind: 'detection' });
    try {
      const value = await saveDetectionSettings(getMobileApiBaseUrl(), input);
      dispatch({ type: 'detection-succeeded', value });
      return true;
    } catch (error) {
      await failSave('detection', error);
      return false;
    } finally {
      actionRunning.current = null;
    }
  }, [failSave]);

  const saveSchedule = useCallback(async (input: WorkScheduleUpdate) => {
    if (actionRunning.current) return false;
    actionRunning.current = 'work-schedule';
    dispatch({ type: 'save-started', kind: 'work-schedule' });
    try {
      const value = await saveWorkSchedule(getMobileApiBaseUrl(), input);
      dispatch({ type: 'work-schedule-succeeded', value });
      return true;
    } catch (error) {
      await failSave('work-schedule', error);
      return false;
    } finally {
      actionRunning.current = null;
    }
  }, [failSave]);

  const saveNotifications = useCallback(async (input: NotificationSettingsUpdate) => {
    if (actionRunning.current) return false;
    actionRunning.current = 'notifications';
    dispatch({ type: 'save-started', kind: 'notifications' });
    try {
      const value = await saveNotificationSettings(getMobileApiBaseUrl(), input);
      dispatch({ type: 'notifications-succeeded', value });
      return true;
    } catch (error) {
      await failSave('notifications', error);
      return false;
    } finally {
      actionRunning.current = null;
    }
  }, [failSave]);

  const value = useMemo<SettingsContextValue>(() => ({
    retry: retryWithSync,
    saveDetection,
    saveNotifications,
    saveSchedule,
    state,
  }), [retryWithSync, saveDetection, saveNotifications, saveSchedule, state]);

  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

export function useSettings() {
  const context = useContext(SettingsContext);
  if (!context) throw new Error('useSettings must be used within SettingsProvider');
  return context;
}
