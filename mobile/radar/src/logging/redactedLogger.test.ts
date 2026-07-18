import { expect, it } from 'vitest';

import { redactFields } from './redactedLogger';

it('redacts credential-shaped fields before logging', () => {
  expect(redactFields({ accessToken: 'secret', status: 'failed' })).toEqual({
    accessToken: '[REDACTED]',
    status: 'failed',
  });
});

it('redacts message content even when a caller accidentally includes it', () => {
  expect(redactFields({
    messageBody: 'private conversation',
    prompt: 'private agent prompt',
    raw_payload: { content: 'private' },
    exampleCount: 3,
  })).toEqual({
    messageBody: '[REDACTED]',
    prompt: '[REDACTED]',
    raw_payload: '[REDACTED]',
    exampleCount: 3,
  });
});
