import { describe, expect, it } from 'vitest';

import { scheduleWindowAt } from './intent-map-model';

const windows = [{
  id: 'morning',
  days: [1, 2, 3, 4, 5],
  startMinute: 540,
  endMinute: 720,
  label: 'Morning focus',
  activeIntentIds: ['customer'],
  fallbackDeliveryMode: 'digest' as const,
}];

describe('scheduleWindowAt', () => {
  it('uses an inclusive start and exclusive end on configured days', () => {
    expect(scheduleWindowAt(windows, 540, 1)?.id).toBe('morning');
    expect(scheduleWindowAt(windows, 719, 5)?.id).toBe('morning');
    expect(scheduleWindowAt(windows, 720, 1)).toBeNull();
    expect(scheduleWindowAt(windows, 600, 6)).toBeNull();
  });
});
