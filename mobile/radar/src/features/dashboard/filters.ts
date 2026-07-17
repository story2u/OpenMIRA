import type { DashboardTrustLevel } from '@story2u/radar-api/opportunities';
import type { DashboardQuery } from '@story2u/radar-contracts/opportunities';

import { fallbackTranslator, type Translator } from '../../i18n/core';

export const dashboardStatuses = ['all', 'pending', 'replied', 'ignored'] as const;
export const dashboardPlatforms = ['all', 'telegram', 'wecom'] as const;
export const dashboardSourceTypes = ['all', 'group', 'private'] as const;
export const dashboardTimeRanges = ['all', 'today', '3d', '7d', 'custom'] as const;
export const dashboardSorts = ['newest', 'oldest', 'confidence', 'trust'] as const;
export const dashboardSopStages = [
  'detected',
  'analyzing',
  'verified',
  'contact_extracted',
  'friend_requested',
  'ready_to_chat',
  'chatting',
  'closed',
] as const;
export const dashboardTrustLevels: readonly DashboardTrustLevel[] = [
  'trusted',
  'unverified',
  'suspicious',
  'risky',
];

export type DashboardStatusFilter = (typeof dashboardStatuses)[number];
export type DashboardPlatformFilter = (typeof dashboardPlatforms)[number];
export type DashboardSourceFilter = (typeof dashboardSourceTypes)[number];
export type DashboardTimeRange = (typeof dashboardTimeRanges)[number];
export type DashboardSortFilter = (typeof dashboardSorts)[number];
export type DashboardSopStage = (typeof dashboardSopStages)[number];

export interface DashboardFilters {
  status: DashboardStatusFilter;
  platform: DashboardPlatformFilter;
  sourceType: DashboardSourceFilter;
  timeRange: DashboardTimeRange;
  customFrom: string;
  customTo: string;
  trustLevels: DashboardTrustLevel[];
  sopStages: DashboardSopStage[];
  keywords: string[];
  sort: DashboardSortFilter;
}

export function createDefaultDashboardFilters(): DashboardFilters {
  return {
    status: 'all',
    platform: 'all',
    sourceType: 'all',
    timeRange: 'all',
    customFrom: '',
    customTo: '',
    trustLevels: [],
    sopStages: [],
    keywords: [],
    sort: 'newest',
  };
}

export function cloneDashboardFilters(filters: DashboardFilters): DashboardFilters {
  return {
    ...filters,
    trustLevels: [...filters.trustLevels],
    sopStages: [...filters.sopStages],
    keywords: [...filters.keywords],
  };
}

function localDateTime(dateOnly: string, endOfDay: boolean, t: Translator) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateOnly);
  if (!match) throw new Error(t('dashboard.filters.error.dateFormat'));
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const value = new Date(
    year,
    month - 1,
    day,
    endOfDay ? 23 : 0,
    endOfDay ? 59 : 0,
    endOfDay ? 59 : 0,
    endOfDay ? 999 : 0,
  );
  if (
    value.getFullYear() !== year ||
    value.getMonth() !== month - 1 ||
    value.getDate() !== day
  ) {
    throw new Error(t('dashboard.filters.error.invalidDate'));
  }
  return value;
}

function timeBounds(filters: DashboardFilters, now: Date, t: Translator) {
  if (filters.timeRange === 'all') return {};
  if (filters.timeRange === 'today') {
    const start = new Date(now);
    start.setHours(0, 0, 0, 0);
    return { created_from: start.toISOString() };
  }
  if (filters.timeRange === '3d' || filters.timeRange === '7d') {
    const days = filters.timeRange === '3d' ? 3 : 7;
    return { created_from: new Date(now.getTime() - days * 24 * 60 * 60 * 1000).toISOString() };
  }

  const start = filters.customFrom ? localDateTime(filters.customFrom, false, t) : null;
  const end = filters.customTo ? localDateTime(filters.customTo, true, t) : null;
  if (!start && !end) throw new Error(t('dashboard.filters.error.dateRequired'));
  if (start && end && start.getTime() > end.getTime()) {
    throw new Error(t('dashboard.filters.error.dateOrder'));
  }
  return {
    created_from: start?.toISOString(),
    created_to: end?.toISOString(),
  };
}

export function normalizeDashboardKeywords(input: string, t: Translator = fallbackTranslator) {
  const keywords = Array.from(new Set(
    input
      .split(/[,，\n]/)
      .map((value) => value.trim())
      .filter(Boolean),
  ));
  if (keywords.length > 64) throw new Error(t('dashboard.filters.error.keywordCount'));
  if (keywords.some((keyword) => keyword.length > 128)) {
    throw new Error(t('dashboard.filters.error.keywordLength'));
  }
  return keywords;
}

export function dashboardFiltersToQuery(
  filters: DashboardFilters,
  page: number,
  limit = 20,
  now = new Date(),
  t: Translator = fallbackTranslator,
): DashboardQuery {
  if (!Number.isInteger(page) || page < 0) throw new Error('Invalid dashboard page');
  const bounds = timeBounds(filters, now, t);
  return {
    status: filters.status === 'all' ? undefined : filters.status,
    platform: filters.platform === 'all' ? undefined : filters.platform,
    source_type: filters.sourceType === 'all' ? undefined : filters.sourceType,
    ...bounds,
    trust_levels: filters.trustLevels.length > 0 ? filters.trustLevels : undefined,
    sop_stages: filters.sopStages.length > 0 ? filters.sopStages : undefined,
    keywords: filters.keywords.length > 0 ? filters.keywords : undefined,
    sort: filters.sort,
    limit,
    offset: page * limit,
  };
}

export function validateDashboardFilters(
  filters: DashboardFilters,
  t: Translator = fallbackTranslator,
) {
  try {
    dashboardFiltersToQuery(filters, 0, 20, new Date(), t);
    return null;
  } catch (error) {
    return error instanceof Error ? error.message : t('dashboard.filters.invalid');
  }
}

export function countAdvancedDashboardFilters(filters: DashboardFilters) {
  let count = 0;
  if (filters.sourceType !== 'all') count += 1;
  if (filters.timeRange !== 'all') count += 1;
  if (filters.trustLevels.length > 0) count += 1;
  if (filters.sopStages.length > 0) count += 1;
  if (filters.keywords.length > 0) count += 1;
  if (filters.sort !== 'newest') count += 1;
  return count;
}
