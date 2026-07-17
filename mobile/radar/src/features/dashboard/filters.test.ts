import { describe, expect, it } from 'vitest';

import {
  countAdvancedDashboardFilters,
  createDefaultDashboardFilters,
  dashboardFiltersToQuery,
  normalizeDashboardKeywords,
  validateDashboardFilters,
} from './filters';

describe('dashboard filters', () => {
  it('maps all supported filters to the server query and stable pagination', () => {
    const filters = {
      ...createDefaultDashboardFilters(),
      status: 'pending' as const,
      platform: 'wecom' as const,
      sourceType: 'group' as const,
      timeRange: 'custom' as const,
      customFrom: '2026-07-01',
      customTo: '2026-07-17',
      trustLevels: ['trusted' as const, 'risky' as const],
      sopStages: ['verified' as const],
      keywords: ['报价', '采购'],
      sort: 'trust' as const,
    };

    const query = dashboardFiltersToQuery(filters, 2, 20);
    expect(query).toMatchObject({
      status: 'pending',
      platform: 'wecom',
      source_type: 'group',
      trust_levels: ['trusted', 'risky'],
      sop_stages: ['verified'],
      keywords: ['报价', '采购'],
      sort: 'trust',
      limit: 20,
      offset: 40,
    });
    expect(query.created_from).toMatch(/^2026-06-30T|^2026-07-01T/);
    expect(query.created_to).toMatch(/^2026-07-17T|^2026-07-18T/);
    expect(countAdvancedDashboardFilters(filters)).toBe(6);
  });

  it('uses local day boundaries for today without storing derived state', () => {
    const filters = { ...createDefaultDashboardFilters(), timeRange: 'today' as const };
    const now = new Date(2026, 6, 17, 14, 30, 0);
    const query = dashboardFiltersToQuery(filters, 0, 20, now);
    const start = new Date(String(query.created_from));
    expect(start.getHours()).toBe(0);
    expect(start.getDate()).toBe(17);
  });

  it('validates custom ranges and normalizes comma-separated keywords', () => {
    expect(normalizeDashboardKeywords(' 报价，采购,报价\n预算 ')).toEqual(['报价', '采购', '预算']);
    expect(validateDashboardFilters({
      ...createDefaultDashboardFilters(),
      timeRange: 'custom',
      customFrom: '2026-07-18',
      customTo: '2026-07-17',
    })).toContain('开始日期');
  });
});
