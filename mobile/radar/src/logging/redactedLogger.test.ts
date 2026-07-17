import { expect, it } from 'vitest';

import { redactFields } from './redactedLogger';

it('redacts credential-shaped fields before logging', () => {
  expect(redactFields({ accessToken: 'secret', status: 'failed' })).toEqual({
    accessToken: '[REDACTED]',
    status: 'failed',
  });
});
