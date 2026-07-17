import { fetch as expoFetch } from 'expo/fetch';

interface StreamingResponse {
  ok: boolean;
  status: number;
  body: ReadableStream<Uint8Array> | null;
}

/** Parses SSE data fields while preserving UTF-8 characters split across chunks. */
export async function collectSseData(response: StreamingResponse): Promise<string[]> {
  if (!response.ok) throw new Error(`Streaming request failed with ${response.status}`);
  if (!response.body) throw new Error('Streaming response has no body');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const events: string[] = [];
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const blocks = buffer.split(/\r?\n\r?\n/);
    buffer = blocks.pop() ?? '';

    for (const block of blocks) {
      const data = block
        .split(/\r?\n/)
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trimStart())
        .join('\n');
      if (data) events.push(data);
    }
    if (done) break;
  }

  if (buffer.trim()) {
    const data = buffer
      .split(/\r?\n/)
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trimStart())
      .join('\n');
    if (data) events.push(data);
  }
  return events;
}

export async function fetchSse(url: string, signal?: AbortSignal) {
  const response = await expoFetch(url, {
    headers: { Accept: 'text/event-stream' },
    signal,
  });
  return collectSseData(response);
}

export async function verifyFetchAbort(url: string, timeoutMs = 75) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    await expoFetch(url, { signal: controller.signal });
    throw new Error('Streaming request completed before cancellation');
  } catch (error) {
    if (!controller.signal.aborted) throw error;
    return true;
  } finally {
    clearTimeout(timer);
  }
}

export async function runNativeStreamingSpike(origin: string) {
  const events = await fetchSse(`${origin}/events`);
  const aborted = await verifyFetchAbort(`${origin}/hang`);
  return { events, aborted };
}

export async function runInMemoryStreamingSpike() {
  const encoder = new TextEncoder();
  const payload = encoder.encode('data: 第一条\n\ndata: second\n\n');
  const response: StreamingResponse = {
    ok: true,
    status: 200,
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(payload.slice(0, 9));
        controller.enqueue(payload.slice(9, 13));
        controller.enqueue(payload.slice(13));
        controller.close();
      },
    }),
  };
  return collectSseData(response);
}
