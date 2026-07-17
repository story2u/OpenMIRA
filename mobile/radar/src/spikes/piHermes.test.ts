import { expect, it } from 'vitest';

import { runPiHermesSpike } from './piHermes';

it('runs the real pi stream and structured tool-call pipeline', async () => {
  const result = await runPiHermesSpike();

  expect(result.callCount).toBe(1);
  expect(result.messageCount).toBeGreaterThanOrEqual(3);
  expect(result.submitted).toMatchObject({ is_opportunity: true, confidence: 0.93 });
});
