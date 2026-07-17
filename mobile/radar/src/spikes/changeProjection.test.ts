import { expect, it } from 'vitest';

import { foldChanges } from '@story2u/radar-core/sync/change-projection';

it('folds out-of-order and duplicate changes idempotently', () => {
  const projection = foldChanges([
    { sequence: 3, entityId: 'a', operation: 'upsert', title: 'new' },
    { sequence: 1, entityId: 'a', operation: 'upsert', title: 'old' },
    { sequence: 2, entityId: 'b', operation: 'upsert', title: 'deleted' },
    { sequence: 3, entityId: 'a', operation: 'upsert', title: 'duplicate' },
    { sequence: 4, entityId: 'b', operation: 'delete' },
  ]);

  expect([...projection.values()]).toEqual([{ entityId: 'a', sequence: 3, title: 'new' }]);
});
