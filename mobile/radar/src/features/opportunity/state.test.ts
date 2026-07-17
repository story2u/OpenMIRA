import type { MessagePage } from '@story2u/radar-contracts/messages';
import type { ManualReplyResult } from '@story2u/radar-contracts/opportunity-actions';
import { expect, it } from 'vitest';

import {
  initialOpportunityDetailState,
  opportunityDetailReducer,
  visibleMessages,
} from './state';

const detail = { id: '11111111-1111-4111-8111-111111111111' } as never;
const message = {
  id: '22222222-2222-4222-8222-222222222222',
  senderName: '联系人',
  content: '需要企业方案',
  isFromContact: true,
  sentAt: '2026-07-17T01:00:00Z',
  source: 'human' as const,
};

function page(overrides: Partial<MessagePage> = {}): MessagePage {
  return { items: [message], total: 2, limit: 1, offset: 0, ...overrides };
}

it('loads, refreshes and ignores a stale opportunity response', () => {
  const loading = opportunityDetailReducer(initialOpportunityDetailState, {
    type: 'started', requestKey: 'first',
  });
  const ready = opportunityDetailReducer(loading, {
    type: 'succeeded', requestKey: 'first', detail, messages: page(),
  });
  expect(ready).toMatchObject({ loading: false, data: { messageTotal: 2 } });

  const refreshing = opportunityDetailReducer(ready, { type: 'started', requestKey: 'first' });
  expect(refreshing).toMatchObject({ refreshing: true, data: ready.data });
  const changed = opportunityDetailReducer(refreshing, { type: 'started', requestKey: 'second' });
  expect(changed).toMatchObject({ loading: true, data: null });
  expect(opportunityDetailReducer(changed, {
    type: 'succeeded', requestKey: 'first', detail, messages: page(),
  })).toBe(changed);
});

it('exposes a queued internal status command without mutating server detail truth', () => {
  const ready = opportunityDetailReducer(
    opportunityDetailReducer(initialOpportunityDetailState, {
      type: 'started', requestKey: 'first',
    }),
    { type: 'succeeded', requestKey: 'first', detail, messages: page() },
  );
  const running = opportunityDetailReducer(ready, {
    type: 'action-started', requestKey: 'first', kind: 'status',
  });
  const queued = opportunityDetailReducer(running, {
    type: 'status-queued', requestKey: 'first', notice: 'queued safely',
  });

  expect(queued).toMatchObject({
    data: ready.data,
    actionKind: null,
    actionError: null,
    actionNotice: 'queued safely',
  });
});

it('appends the expected page once and retains data on pagination failure', () => {
  const ready = opportunityDetailReducer(
    opportunityDetailReducer(initialOpportunityDetailState, {
      type: 'started', requestKey: 'first',
    }),
    { type: 'succeeded', requestKey: 'first', detail, messages: page() },
  );
  const loadingMore = opportunityDetailReducer(ready, {
    type: 'more-started', requestKey: 'first',
  });
  const second = { ...message, id: '33333333-3333-4333-8333-333333333333' };
  const extended = opportunityDetailReducer(loadingMore, {
    type: 'more-succeeded',
    requestKey: 'first',
    messages: page({ items: [second], offset: 1 }),
  });
  expect(extended.data?.messages.map((item) => item.id)).toEqual([message.id, second.id]);

  const ignoredOffset = opportunityDetailReducer(extended, {
    type: 'more-succeeded',
    requestKey: 'first',
    messages: page({ items: [second], offset: 1 }),
  });
  expect(ignoredOffset).toBe(extended);

  const failed = opportunityDetailReducer(
    { ...extended, loadingMore: true },
    { type: 'more-failed', requestKey: 'first', error: '无法加载更多' },
  );
  expect(failed).toMatchObject({ data: extended.data, messageError: '无法加载更多' });
});

it('keeps a newly sent tail message without corrupting the contiguous page offset', () => {
  const ready = opportunityDetailReducer(
    opportunityDetailReducer(initialOpportunityDetailState, {
      type: 'started', requestKey: 'first',
    }),
    {
      type: 'succeeded',
      requestKey: 'first',
      detail,
      messages: page({ total: 3 }),
    },
  );
  const sent = { ...message, id: '44444444-4444-4444-8444-444444444444' };
  const replied = opportunityDetailReducer(ready, {
    type: 'reply-succeeded',
    requestKey: 'first',
    result: {
      opportunity: detail,
      message: sent,
      messageTotal: 4,
    } as ManualReplyResult,
  });
  expect(replied.data?.messageNextOffset).toBe(1);
  expect(visibleMessages(replied.data!).map((item) => item.id)).toEqual([
    message.id,
    sent.id,
  ]);

  const second = { ...message, id: '33333333-3333-4333-8333-333333333333' };
  const middle = opportunityDetailReducer(replied, {
    type: 'more-succeeded',
    requestKey: 'first',
    messages: page({ items: [second], total: 4, offset: 1 }),
  });
  expect(middle.data?.messageNextOffset).toBe(2);
  expect(visibleMessages(middle.data!).map((item) => item.id)).toEqual([
    message.id,
    second.id,
    sent.id,
  ]);

  const third = { ...message, id: '55555555-5555-4555-8555-555555555555' };
  const completed = opportunityDetailReducer(middle, {
    type: 'more-succeeded',
    requestKey: 'first',
    messages: page({ items: [third, sent], total: 4, limit: 2, offset: 2 }),
  });
  expect(completed.data?.messageNextOffset).toBe(4);
  expect(completed.data?.tailMessages).toEqual([]);
  expect(visibleMessages(completed.data!).map((item) => item.id)).toEqual([
    message.id,
    second.id,
    third.id,
    sent.id,
  ]);
});
