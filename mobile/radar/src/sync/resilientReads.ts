import type { MessagePage } from '@story2u/radar-contracts/messages';
import type {
  Dashboard,
  DashboardQuery,
} from '@story2u/radar-contracts/opportunities';
import type { SettingsBundle } from '@story2u/radar-contracts/settings';

import {
  readDashboard,
  readMessagePage,
  readOpportunityDetailBundle,
  readSettings,
  type OpportunityDetailBundle,
} from '../api/client';
import { initializeRadarDatabase } from '../storage/database';
import { readWithOfflineFallback } from './offlineFallback';
import {
  OfflineProjectionUnavailableError,
  readOfflineDashboard,
  readOfflineMessagePage,
  readOfflineOpportunityDetail,
  readOfflineProjectionWithRecovery,
  readOfflineSettings,
} from './offlineRepository';

export interface OfflineReadScope {
  enabled: boolean;
  ownerId: string;
}

export function readDashboardResilient(
  baseUrl: string,
  scope: OfflineReadScope,
  query: DashboardQuery,
  signal?: AbortSignal,
): Promise<Dashboard> {
  return readWithOfflineFallback(
    scope.enabled,
    () => readDashboard(baseUrl, query, signal),
    async () => {
      const database = await initializeRadarDatabase();
      return readOfflineProjectionWithRecovery(
        database,
        scope.ownerId,
        () => readOfflineDashboard(database, scope.ownerId, query),
      );
    },
  );
}

export function readOpportunityDetailBundleResilient(
  baseUrl: string,
  scope: OfflineReadScope,
  opportunityId: string,
  signal?: AbortSignal,
): Promise<OpportunityDetailBundle> {
  return readWithOfflineFallback(
    scope.enabled,
    () => readOpportunityDetailBundle(baseUrl, opportunityId, signal),
    async () => {
      const database = await initializeRadarDatabase();
      return readOfflineProjectionWithRecovery(database, scope.ownerId, async () => {
        const [detail, messages] = await Promise.all([
          readOfflineOpportunityDetail(database, scope.ownerId, opportunityId),
          readOfflineMessagePage(database, scope.ownerId, opportunityId),
        ]);
        if (!detail) throw new OfflineProjectionUnavailableError('opportunity_not_cached');
        return { detail, messages };
      });
    },
  );
}

export function readMessagePageResilient(
  baseUrl: string,
  scope: OfflineReadScope,
  opportunityId: string,
  offset: number,
  signal?: AbortSignal,
): Promise<MessagePage> {
  return readWithOfflineFallback(
    scope.enabled,
    () => readMessagePage(baseUrl, opportunityId, offset, signal),
    async () => {
      const database = await initializeRadarDatabase();
      return readOfflineProjectionWithRecovery(
        database,
        scope.ownerId,
        () => readOfflineMessagePage(
          database,
          scope.ownerId,
          opportunityId,
          { limit: 20, offset },
        ),
      );
    },
  );
}

export function readSettingsResilient(
  baseUrl: string,
  scope: OfflineReadScope,
  signal?: AbortSignal,
): Promise<SettingsBundle> {
  return readWithOfflineFallback(
    scope.enabled,
    () => readSettings(baseUrl, signal),
    async () => {
      const database = await initializeRadarDatabase();
      return readOfflineProjectionWithRecovery(
        database,
        scope.ownerId,
        () => readOfflineSettings(database, scope.ownerId),
      );
    },
  );
}
