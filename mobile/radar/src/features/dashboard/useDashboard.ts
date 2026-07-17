import { RadarApiError } from '@story2u/radar-api/client';
import { dashboardRequestPath } from '@story2u/radar-api/opportunities';
import { useCallback, useEffect, useMemo, useReducer, useRef } from 'react';

import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import type { Translator } from '../../i18n/core';
import { logEvent } from '../../logging/redactedLogger';
import { readDashboardResilient } from '../../sync/resilientReads';
import { dashboardFiltersToQuery, type DashboardFilters } from './filters';
import {
  dashboardLoadReducer,
  initialDashboardLoadState,
} from './state';

function dashboardErrorMessage(error: unknown, t: Translator) {
  if (error instanceof RadarApiError) {
    if (error.status === 422) return t('dashboard.error.invalidFilter');
    if (error.status >= 500) return t('dashboard.error.unavailable');
  }
  return t('dashboard.error.network');
}

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

export function useDashboard(
  filters: DashboardFilters,
  page: number,
  ownerId: string,
  offlineEnabled: boolean,
  expireSession: () => Promise<void>,
  synchronize: () => Promise<void>,
) {
  const { t } = useI18n();
  const query = useMemo(() => dashboardFiltersToQuery(filters, page), [filters, page]);
  const requestKey = useMemo(() => dashboardRequestPath(query), [query]);
  const [state, dispatch] = useReducer(dashboardLoadReducer, initialDashboardLoadState);
  const [revision, bumpRevision] = useReducer((value: number) => value + 1, 0);
  const syncOnNextLoad = useRef(false);

  const retry = useCallback(() => {
    syncOnNextLoad.current = true;
    bumpRevision();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    dispatch({ type: 'started', requestKey });

    async function load() {
      try {
        if (syncOnNextLoad.current) {
          syncOnNextLoad.current = false;
          await synchronize();
        }
        const result = await readDashboardResilient(
          getMobileApiBaseUrl(),
          { enabled: offlineEnabled, ownerId },
          query,
          controller.signal,
        );
        if (active) dispatch({ type: 'succeeded', requestKey, data: result });
      } catch (error) {
        if (!active || isAbortError(error)) return;
        if (error instanceof RadarApiError && error.status === 401) {
          await expireSession();
          return;
        }
        logEvent('dashboard.load_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        if (active) {
          dispatch({ type: 'failed', requestKey, error: dashboardErrorMessage(error, t) });
        }
      }
    }

    void load();
    return () => {
      active = false;
      controller.abort();
    };
  }, [expireSession, offlineEnabled, ownerId, query, requestKey, revision, synchronize, t]);

  return { state, retry };
}
