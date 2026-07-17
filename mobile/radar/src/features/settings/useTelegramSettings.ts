import { RadarApiError } from '@story2u/radar-api/client';
import type { TelegramConnection } from '@story2u/radar-contracts/telegram';
import { useCallback, useEffect, useReducer, useRef } from 'react';

import {
  readTelegramOverview,
  setTelegramConnectionEnabled,
} from '../../api/client';
import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import type { Translator } from '../../i18n/core';
import { logEvent } from '../../logging/redactedLogger';
import {
  initialTelegramSettingsState,
  telegramSettingsReducer,
} from './telegramState';

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

function telegramLoadError(error: unknown, t: Translator) {
  if (error instanceof RadarApiError && error.status >= 500) {
    return t('settings.telegram.error.unavailable');
  }
  return t('settings.telegram.error.network');
}

function telegramWriteError(error: unknown, t: Translator) {
  if (error instanceof RadarApiError) {
    if (error.status === 404) return t('settings.telegram.error.removed');
    if (error.status >= 500) return t('settings.telegram.error.writeUnavailable');
  }
  return t('settings.telegram.error.write');
}

export function useTelegramSettings(expireSession: () => Promise<void>) {
  const { t } = useI18n();
  const [state, dispatch] = useReducer(telegramSettingsReducer, initialTelegramSettingsState);
  const [revision, retry] = useReducer((value: number) => value + 1, 0);
  const actionRunning = useRef(false);

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
        const overview = await readTelegramOverview(getMobileApiBaseUrl(), controller.signal);
        dispatch({ type: 'load-succeeded', ...overview });
      } catch (error) {
        if (isAbortError(error)) return;
        if (await expireIfUnauthorized(error)) return;
        logEvent('telegram_settings.load_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({ type: 'load-failed', error: telegramLoadError(error, t) });
      }
    }
    void load();
    return () => controller.abort();
  }, [expireIfUnauthorized, revision, t]);

  const toggle = useCallback(async (connection: TelegramConnection, enabled: boolean) => {
    if (actionRunning.current || connection.enabled === enabled) return false;
    actionRunning.current = true;
    dispatch({ type: 'toggle-started', connectionId: connection.id });
    try {
      const updated = await setTelegramConnectionEnabled(
        getMobileApiBaseUrl(),
        connection.id,
        enabled,
      );
      dispatch({ type: 'toggle-succeeded', connection: updated });
      return true;
    } catch (error) {
      if (!(await expireIfUnauthorized(error))) {
        logEvent('telegram_settings.toggle_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({
          type: 'toggle-failed',
          connectionId: connection.id,
          error: telegramWriteError(error, t),
        });
      }
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [expireIfUnauthorized, t]);

  return { retry, state, toggle };
}
