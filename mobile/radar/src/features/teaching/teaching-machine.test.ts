import { describe, expect, it } from 'vitest';

import {
  classifyTeachingSwipe,
  initialTeachingMachineState,
  teachingReducer,
} from './teaching-machine';

const sample = {
  translationX: -130,
  translationY: 8,
  velocityX: -200,
  width: 300,
  startX: 80,
};

describe('teaching swipe semantics', () => {
  it('maps physical left to positive and physical right to negative', () => {
    expect(classifyTeachingSwipe(sample)).toBe('positive');
    expect(classifyTeachingSwipe({ ...sample, translationX: 130, velocityX: 200 }))
      .toBe('negative');
  });

  it('accepts a deliberate fling but rejects snapback, vertical, edge, and multi-touch gestures', () => {
    expect(classifyTeachingSwipe({ ...sample, translationX: -30, velocityX: -900 }))
      .toBe('positive');
    expect(classifyTeachingSwipe({ ...sample, translationX: -80, velocityX: -300 })).toBeNull();
    expect(classifyTeachingSwipe({ ...sample, translationX: -130, translationY: 120 })).toBeNull();
    expect(classifyTeachingSwipe({ ...sample, startX: 12 })).toBeNull();
    expect(classifyTeachingSwipe({ ...sample, pointers: 2 })).toBeNull();
  });
});

describe('teaching state machine', () => {
  it('commits, advances and supports ten-step compatible repeated undo', () => {
    let state = teachingReducer(initialTeachingMachineState, { type: 'LOAD' });
    state = teachingReducer(state, { type: 'READY' });
    state = teachingReducer(state, { type: 'DRAG', direction: 'left' });
    state = teachingReducer(state, { type: 'COMMIT', label: 'positive' });
    state = teachingReducer(state, { type: 'ADVANCE', finished: false });
    state = teachingReducer(state, { type: 'COMMIT', label: 'negative' });
    state = teachingReducer(state, { type: 'ADVANCE', finished: false });
    state = teachingReducer(state, { type: 'UNDO' });
    state = teachingReducer(state, { type: 'UNDO_COMPLETE' });
    expect(state).toMatchObject({ phase: 'presenting', cardIndex: 1 });
    expect(state.completedActions).toHaveLength(1);
  });

  it('records skip without treating it as a positive or negative committed phase', () => {
    let state = teachingReducer(initialTeachingMachineState, { type: 'READY' });
    state = teachingReducer(state, { type: 'COMMIT', label: 'skipped' });
    expect(state.phase).toBe('presenting');
    expect(state.completedActions.at(-1)?.label).toBe('skipped');
  });

  it('processes one hundred continuous teaching decisions without losing order', () => {
    let state = teachingReducer(initialTeachingMachineState, { type: 'READY' });
    for (let index = 0; index < 100; index += 1) {
      state = teachingReducer(state, {
        type: 'COMMIT',
        label: index % 2 === 0 ? 'positive' : 'negative',
      });
      state = teachingReducer(state, { type: 'ADVANCE', finished: index === 99 });
    }
    expect(state).toMatchObject({ cardIndex: 100, phase: 'completed' });
    expect(state.completedActions).toHaveLength(100);
    expect(state.completedActions[99]).toEqual({ cardIndex: 99, label: 'negative' });
  });
});
