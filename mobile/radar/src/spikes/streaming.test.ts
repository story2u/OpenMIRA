import { expect, it, vi } from 'vitest';

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));

import { collectSseData } from './streaming';

it('decodes SSE events and UTF-8 split across byte chunks', async () => {
  const bytes = new TextEncoder().encode('data: 第一条\n\ndata: line one\ndata: line two\n\n');
  const response = {
    ok: true,
    status: 200,
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes.slice(0, 8));
        controller.enqueue(bytes.slice(8, 11));
        controller.enqueue(bytes.slice(11));
        controller.close();
      },
    }),
  };

  await expect(collectSseData(response)).resolves.toEqual(['第一条', 'line one\nline two']);
});

it('fails closed on a non-success response', async () => {
  await expect(collectSseData({ ok: false, status: 401, body: null })).rejects.toThrow('401');
});
