import { expect, it } from 'vitest';

import {
  dashboardLoadReducer,
  initialDashboardLoadState,
} from './state';

const emptyDashboard = {
  items: [],
  total: 0,
  limit: 20,
  offset: 0,
  pendingCount: 0,
  attentionItems: [],
  keywordOptions: [],
};

it('represents loading, empty success, refresh failure and retry without stale query data', () => {
  const loading = dashboardLoadReducer(initialDashboardLoadState, {
    type: 'started',
    requestKey: 'first',
  });
  expect(loading).toMatchObject({ loading: true, data: null });

  const ready = dashboardLoadReducer(loading, {
    type: 'succeeded',
    requestKey: 'first',
    data: emptyDashboard,
  });
  expect(ready).toMatchObject({ loading: false, data: emptyDashboard, error: null });

  const refreshing = dashboardLoadReducer(ready, { type: 'started', requestKey: 'first' });
  expect(refreshing).toMatchObject({ refreshing: true, data: emptyDashboard });
  const failedRefresh = dashboardLoadReducer(refreshing, {
    type: 'failed',
    requestKey: 'first',
    error: '暂时无法刷新',
  });
  expect(failedRefresh).toMatchObject({ data: emptyDashboard, error: '暂时无法刷新' });

  const changedQuery = dashboardLoadReducer(failedRefresh, {
    type: 'started',
    requestKey: 'second',
  });
  expect(changedQuery).toMatchObject({ loading: true, data: null, error: null });
  expect(dashboardLoadReducer(changedQuery, {
    type: 'succeeded',
    requestKey: 'first',
    data: emptyDashboard,
  })).toBe(changedQuery);
});
