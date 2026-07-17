import type { Dashboard } from '@story2u/radar-contracts/opportunities';

export interface DashboardLoadState {
  requestKey: string | null;
  data: Dashboard | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
}

export type DashboardLoadEvent =
  | { type: 'started'; requestKey: string }
  | { type: 'succeeded'; requestKey: string; data: Dashboard }
  | { type: 'failed'; requestKey: string; error: string };

export const initialDashboardLoadState: DashboardLoadState = {
  requestKey: null,
  data: null,
  loading: false,
  refreshing: false,
  error: null,
};

export function dashboardLoadReducer(
  state: DashboardLoadState,
  event: DashboardLoadEvent,
): DashboardLoadState {
  if (event.type === 'started') {
    const canRefresh = state.requestKey === event.requestKey && state.data !== null;
    return {
      requestKey: event.requestKey,
      data: canRefresh ? state.data : null,
      loading: !canRefresh,
      refreshing: canRefresh,
      error: null,
    };
  }
  if (state.requestKey !== event.requestKey) return state;
  if (event.type === 'succeeded') {
    return {
      requestKey: state.requestKey,
      data: event.data,
      loading: false,
      refreshing: false,
      error: null,
    };
  }
  return {
    ...state,
    loading: false,
    refreshing: false,
    error: event.error,
  };
}
