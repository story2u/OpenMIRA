import { expect, it } from 'vitest';

import { parseCapabilities } from './clientCapabilities';

it('fails closed for absent and non-boolean server capabilities', () => {
  expect(parseCapabilities({ syncAvailable: true, deviceAgentAvailable: 'yes' })).toMatchObject({
    syncAvailable: true,
    deviceAgentAvailable: false,
    agentToolsAvailable: false,
  });
});
