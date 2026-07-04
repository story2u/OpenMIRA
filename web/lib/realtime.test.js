import assert from "node:assert/strict";
import test from "node:test";

import {
  acknowledgeRealtimeCursor,
  createWebSocketUrl,
  detectRealtimeGap,
  fetchRealtimeEventsReplay,
  fetchRealtimeWorkbenchSnapshot,
  getRealtimeCursorSnapshot,
  mergeRealtimeCursorSnapshot,
  normalizeTopics,
  resetRealtimeStateForTest,
  subscribeRealtimeChannel,
} from "./realtime.js";

test("createWebSocketUrl preserves channel, sorted topics, and token", () => {
  const url = createWebSocketUrl("conversations", ["task.status", "conversation.message", "task.status"], {
    token: "session-token",
    baseUrl: "https://api.example.com/",
  });
  assert.equal(
    url,
    "wss://api.example.com/ws/conversations?topics=conversation.message%2Ctask.status&token=session-token",
  );
  assert.deepEqual(normalizeTopics([" b ", "a", "b", ""]), ["a", "b"]);
});

test("detectRealtimeGap tracks per-scope cursors", () => {
  resetRealtimeStateForTest();
  assert.equal(acknowledgeRealtimeCursor("conversations:conversation.message", 3), true);
  const gap = detectRealtimeGap({
    scope_key: "conversations:conversation.message",
    cursor: 6,
  });
  assert.deepEqual(gap, {
    scopeKey: "conversations:conversation.message",
    expectedCursor: 4,
    receivedCursor: 6,
    gapSize: 2,
    needsResync: false,
  });

  mergeRealtimeCursorSnapshot({ "conversations:conversation.message": 200 });
  const largeGap = detectRealtimeGap({
    scope_key: "conversations:conversation.message",
    cursor: 305,
  });
  assert.equal(largeGap.needsResync, true);
});

test("fetchRealtime helpers call Python-compatible paths", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options });
    return {
      ok: true,
      async text() {
        return JSON.stringify({ ok: true });
      },
    };
  };
  try {
    await fetchRealtimeEventsReplay({
      scope: "conversations:conversation.message",
      afterCursor: 7,
      limit: 20,
      token: "session-token",
    });
    await fetchRealtimeWorkbenchSnapshot({ token: "session-token" });
  } finally {
    globalThis.fetch = originalFetch;
  }
  assert.equal(
    calls[0].url,
    "/api/v1/realtime/events/replay?scope=conversations%3Aconversation.message&after_cursor=7&limit=20",
  );
  assert.equal(calls[0].options.headers.Authorization, "Bearer session-token");
  assert.equal(calls[1].url, "/api/v1/realtime/snapshot/workbench");
});

test("subscribeRealtimeChannel dispatches envelopes and gaps", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }

    close() {
      this.closed = true;
      this.onclose?.();
    }
  }
  const events = [];
  const gaps = [];
  const unsubscribe = subscribeRealtimeChannel(
    "conversations",
    ["conversation.message"],
    (event, meta) => events.push({ event, meta }),
    {
      WebSocketImpl: FakeWebSocket,
      baseUrl: "http://localhost:9000",
      onGap: (gap) => gaps.push(gap),
    },
  );
  assert.equal(sockets[0].url, "ws://localhost:9000/ws/conversations?topics=conversation.message");
  acknowledgeRealtimeCursor("conversations:conversation.message", 1);
  sockets[0].onmessage({
    data: JSON.stringify({
      channel: "conversations",
      event: "conversation.message",
      topic: "conversation.message",
      scope_key: "conversations:conversation.message",
      cursor: 3,
      payload: { conversation_id: "conv-1" },
    }),
  });
  await flushRealtime();
  assert.equal(events.length, 1);
  assert.equal(gaps.length, 1);
  assert.equal(gaps[0].expectedCursor, 2);
  unsubscribe();
  assert.equal(sockets[0].closed, true);
});

test("subscribeRealtimeChannel replays small cursor gaps before live envelope", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  const replayCalls = [];
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }
  }
  const events = [];
  const replays = [];
  subscribeRealtimeChannel(
    "conversations",
    ["conversation.message"],
    (event, meta) => events.push({ event, meta }),
    {
      WebSocketImpl: FakeWebSocket,
      baseUrl: "http://localhost:9000",
      token: "session-token",
      fetchReplay: async (request) => {
        replayCalls.push(request);
        return {
          events: [
            {
              channel: "conversations",
              event: "conversation.message",
              topic: "conversation.message",
              scope_key: "conversations:conversation.message",
              cursor: 2,
              payload: { conversation_id: "conv-1", message_id: "m-2" },
            },
            {
              channel: "conversations",
              event: "conversation.message",
              topic: "conversation.message",
              scope_key: "conversations:conversation.message",
              cursor: 3,
              payload: { conversation_id: "conv-1", message_id: "duplicate-live" },
            },
          ],
          has_more: false,
          latest_cursor: 3,
        };
      },
      onReplay: (result) => replays.push(result),
    },
  );

  acknowledgeRealtimeCursor("conversations:conversation.message", 1);
  sockets[0].onmessage({
    data: JSON.stringify({
      channel: "conversations",
      event: "conversation.message",
      topic: "conversation.message",
      scope_key: "conversations:conversation.message",
      cursor: 3,
      payload: { conversation_id: "conv-1", message_id: "m-3" },
    }),
  });
  await flushRealtime();

  assert.deepEqual(replayCalls, [
    {
      scope: "conversations:conversation.message",
      afterCursor: 1,
      limit: 1,
      token: "session-token",
    },
  ]);
  assert.equal(events.length, 2);
  assert.equal(events[0].event.payload.message_id, "m-2");
  assert.equal(events[0].meta.replayed, true);
  assert.equal(events[1].event.payload.message_id, "m-3");
  assert.equal(events[1].meta.gap.expectedCursor, 2);
  assert.equal(replays.length, 1);
  assert.equal(replays[0].events.length, 1);
});

test("subscribeRealtimeChannel snapshots when cursor gap exceeds replay window", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  const snapshotCalls = [];
  const resyncs = [];
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }
  }

  subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "http://localhost:9000",
    token: "session-token",
    fetchSnapshot: async (request) => {
      snapshotCalls.push(request);
      return {
        cursors: {
          "chat:identity.updated": 7,
          "conversations:conversation.message": 105,
        },
        resync_required: false,
      };
    },
    onResync: (result) => resyncs.push(result),
  });

  acknowledgeRealtimeCursor("conversations:conversation.message", 1);
  sockets[0].onmessage({
    data: JSON.stringify({
      channel: "conversations",
      event: "conversation.message",
      topic: "conversation.message",
      scope_key: "conversations:conversation.message",
      cursor: 105,
      payload: { conversation_id: "conv-1" },
    }),
  });
  await flushRealtime();

  assert.deepEqual(snapshotCalls, [{ token: "session-token" }]);
  assert.equal(resyncs.length, 1);
  assert.equal(resyncs[0].reason, "gap_too_large");
  assert.equal(getRealtimeCursorSnapshot()["chat:identity.updated"], 7);
  assert.equal(getRealtimeCursorSnapshot()["conversations:conversation.message"], 105);
});

test("subscribeRealtimeChannel snapshots when replay is incomplete", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  const replayFailures = [];
  const resyncs = [];
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }
  }

  subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "http://localhost:9000",
    token: "session-token",
    fetchReplay: async () => ({ events: [], has_more: false, latest_cursor: 1 }),
    fetchSnapshot: async () => ({ cursors: { "conversations:conversation.message": 4 } }),
    onReplayFailed: (gap, error) => replayFailures.push({ gap, error }),
    onResync: (result) => resyncs.push(result),
  });

  acknowledgeRealtimeCursor("conversations:conversation.message", 1);
  sockets[0].onmessage({
    data: JSON.stringify({
      channel: "conversations",
      event: "conversation.message",
      topic: "conversation.message",
      scope_key: "conversations:conversation.message",
      cursor: 4,
      payload: { conversation_id: "conv-1" },
    }),
  });
  await flushRealtime();

  assert.equal(replayFailures.length, 0);
  assert.equal(resyncs.length, 1);
  assert.equal(resyncs[0].reason, "replay_incomplete");
  assert.equal(getRealtimeCursorSnapshot()["conversations:conversation.message"], 4);
});

test("subscribeRealtimeChannel refreshes token before websocket connect", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  let token = "old-token";
  let refreshCalled = false;
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }

    close() {
      this.closed = true;
    }
  }

  subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "https://api.example.com",
    getToken: () => token,
    ensureTokenFresh: async ({ minTtlMs }) => {
      assert.equal(minTtlMs, 120000);
      refreshCalled = true;
      token = "fresh-token";
      return true;
    },
  });
  assert.equal(sockets.length, 0);
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(refreshCalled, true);
  assert.equal(
    sockets[0].url,
    "wss://api.example.com/ws/conversations?topics=conversation.message&token=fresh-token",
  );
});

test("subscribeRealtimeChannel skips websocket when token refresh fails", async () => {
  resetRealtimeStateForTest();
  const sockets = [];
  let failed = false;
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }
  }

  subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "https://api.example.com",
    ensureTokenFresh: async () => false,
    onAuthRefreshFailed: () => {
      failed = true;
    },
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(failed, true);
  assert.equal(sockets.length, 0);
});

test("subscribeRealtimeChannel reconnects after close with exponential backoff", () => {
  resetRealtimeStateForTest();
  const sockets = [];
  const scheduled = [];
  const reconnects = [];
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }
  }

  subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "http://localhost:9000",
    reconnectBaseDelayMs: 10,
    reconnectMaxDelayMs: 80,
    setTimeout: (callback, delayMs) => {
      scheduled.push({ callback, delayMs });
      return scheduled.length;
    },
    clearTimeout: () => {},
    onReconnectScheduled: (event) => reconnects.push(event),
  });

  sockets[0].onclose();
  assert.equal(scheduled[0].delayMs, 10);
  assert.equal(reconnects[0].delayMs, 10);
  scheduled.shift().callback();
  assert.equal(sockets.length, 2);

  sockets[1].onclose();
  assert.equal(scheduled[0].delayMs, 20);
  scheduled.shift().callback();
  assert.equal(sockets.length, 3);

  sockets[2].onopen();
  sockets[2].onclose();
  assert.equal(scheduled[0].delayMs, 10);
});

test("subscribeRealtimeChannel clears pending reconnect when unsubscribed", () => {
  resetRealtimeStateForTest();
  const sockets = [];
  const timers = new Map();
  const cleared = [];
  let timerID = 0;
  class FakeWebSocket {
    constructor(url) {
      this.url = url;
      sockets.push(this);
    }

    close() {
      this.closed = true;
      this.onclose?.();
    }
  }

  const unsubscribe = subscribeRealtimeChannel("conversations", ["conversation.message"], () => {}, {
    WebSocketImpl: FakeWebSocket,
    baseUrl: "http://localhost:9000",
    setTimeout: (callback, delayMs) => {
      timerID += 1;
      timers.set(timerID, { callback, delayMs });
      return timerID;
    },
    clearTimeout: (id) => {
      cleared.push(id);
      timers.delete(id);
    },
  });

  sockets[0].onclose();
  assert.equal(timers.size, 1);
  unsubscribe();
  assert.deepEqual(cleared, [1]);
  assert.equal(timers.size, 0);
});

async function flushRealtime() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}
