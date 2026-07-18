export type TeachingPhase =
  | 'idle'
  | 'loading'
  | 'onboarding'
  | 'presenting'
  | 'dragging-left'
  | 'dragging-right'
  | 'committed-positive'
  | 'committed-negative'
  | 'undoing'
  | 'completed'
  | 'error';

export interface TeachingMachineState {
  phase: TeachingPhase;
  cardIndex: number;
  completedActions: Array<{ cardIndex: number; label: 'positive' | 'negative' | 'skipped' }>;
  error: string | null;
}

export type TeachingMachineEvent =
  | { type: 'LOAD' }
  | { type: 'SHOW_ONBOARDING' }
  | { type: 'READY' }
  | { type: 'DRAG'; direction: 'left' | 'right' }
  | { type: 'CANCEL_DRAG' }
  | { type: 'COMMIT'; label: 'positive' | 'negative' | 'skipped' }
  | { type: 'ADVANCE'; finished: boolean }
  | { type: 'UNDO' }
  | { type: 'UNDO_COMPLETE' }
  | { type: 'COMPLETE' }
  | { type: 'FAIL'; error: string };

export const initialTeachingMachineState: TeachingMachineState = {
  phase: 'idle',
  cardIndex: 0,
  completedActions: [],
  error: null,
};

export function teachingReducer(
  state: TeachingMachineState,
  event: TeachingMachineEvent,
): TeachingMachineState {
  switch (event.type) {
    case 'LOAD':
      return { ...initialTeachingMachineState, phase: 'loading' };
    case 'SHOW_ONBOARDING':
      return { ...state, phase: 'onboarding' };
    case 'READY':
      return { ...state, phase: 'presenting', error: null };
    case 'DRAG':
      if (state.phase !== 'presenting') return state;
      return { ...state, phase: event.direction === 'left' ? 'dragging-left' : 'dragging-right' };
    case 'CANCEL_DRAG':
      if (state.phase !== 'dragging-left' && state.phase !== 'dragging-right') return state;
      return { ...state, phase: 'presenting' };
    case 'COMMIT':
      if (!['presenting', 'dragging-left', 'dragging-right'].includes(state.phase)) return state;
      return {
        ...state,
        phase: event.label === 'positive'
          ? 'committed-positive'
          : event.label === 'negative'
            ? 'committed-negative'
            : 'presenting',
        completedActions: [...state.completedActions, { cardIndex: state.cardIndex, label: event.label }],
      };
    case 'ADVANCE':
      return {
        ...state,
        cardIndex: state.cardIndex + 1,
        phase: event.finished ? 'completed' : 'presenting',
      };
    case 'UNDO':
      return state.completedActions.length ? { ...state, phase: 'undoing' } : state;
    case 'UNDO_COMPLETE': {
      const restored = state.completedActions.at(-1);
      if (!restored) return { ...state, phase: 'presenting' };
      return {
        ...state,
        cardIndex: restored.cardIndex,
        completedActions: state.completedActions.slice(0, -1),
        phase: 'presenting',
      };
    }
    case 'COMPLETE':
      return { ...state, phase: 'completed' };
    case 'FAIL':
      return { ...state, phase: 'error', error: event.error };
  }
}

export interface SwipeSample {
  translationX: number;
  translationY: number;
  velocityX: number;
  width: number;
  startX: number;
  pointers?: number;
}

export function classifyTeachingSwipe(sample: SwipeSample): 'positive' | 'negative' | null {
  if (sample.pointers && sample.pointers > 1) return null;
  if (sample.startX < 24 || sample.width <= 0) return null;
  const horizontal = Math.abs(sample.translationX);
  if (horizontal <= Math.abs(sample.translationY) * 1.25) return null;
  const distanceCommitted = horizontal >= sample.width * 0.4;
  const flingCommitted = horizontal >= sample.width * 0.08 && Math.abs(sample.velocityX) >= 850;
  if (!distanceCommitted && !flingCommitted) return null;
  // Product semantics are physical and intentionally never mirrored for RTL.
  return sample.translationX < 0 ? 'positive' : 'negative';
}
