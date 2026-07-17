import type { ChatMessage, MessagePage } from '@story2u/radar-contracts/messages';
import type { ManualReplyResult } from '@story2u/radar-contracts/opportunity-actions';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { ReplyTemplate } from '@story2u/radar-contracts/templates';

export type OpportunityActionKind = 'claim' | 'draft' | 'reply' | 'status';

export interface OpportunityDetailData {
  detail: OpportunityDetail;
  messages: ChatMessage[];
  tailMessages: ChatMessage[];
  messageNextOffset: number;
  messageTotal: number;
  messageLimit: number;
}

export interface OpportunityDetailState {
  requestKey: string | null;
  data: OpportunityDetailData | null;
  loading: boolean;
  refreshing: boolean;
  loadingMore: boolean;
  error: string | null;
  messageError: string | null;
  actionKind: OpportunityActionKind | null;
  actionError: string | null;
  actionErrorKind: OpportunityActionKind | null;
  actionNotice: string | null;
  templates: ReplyTemplate[];
  templatesLoaded: boolean;
  templatesLoading: boolean;
  templateError: string | null;
}

export type OpportunityDetailEvent =
  | { type: 'started'; requestKey: string }
  | {
      type: 'succeeded';
      requestKey: string;
      detail: OpportunityDetail;
      messages: MessagePage;
    }
  | { type: 'failed'; requestKey: string; error: string }
  | { type: 'more-started'; requestKey: string }
  | { type: 'more-succeeded'; requestKey: string; messages: MessagePage }
  | { type: 'more-failed'; requestKey: string; error: string }
  | { type: 'action-started'; requestKey: string; kind: OpportunityActionKind }
  | {
      type: 'action-failed';
      requestKey: string;
      kind: OpportunityActionKind;
      error: string;
    }
  | { type: 'detail-updated'; requestKey: string; detail: OpportunityDetail }
  | { type: 'status-queued'; requestKey: string; notice: string }
  | { type: 'draft-succeeded'; requestKey: string; draft: string }
  | { type: 'reply-succeeded'; requestKey: string; result: ManualReplyResult }
  | { type: 'templates-started'; requestKey: string }
  | { type: 'templates-succeeded'; requestKey: string; templates: ReplyTemplate[] }
  | { type: 'templates-failed'; requestKey: string; error: string };

export const initialOpportunityDetailState: OpportunityDetailState = {
  requestKey: null,
  data: null,
  loading: true,
  refreshing: false,
  loadingMore: false,
  error: null,
  messageError: null,
  actionKind: null,
  actionError: null,
  actionErrorKind: null,
  actionNotice: null,
  templates: [],
  templatesLoaded: false,
  templatesLoading: false,
  templateError: null,
};

export function visibleMessages(data: OpportunityDetailData): ChatMessage[] {
  const prefixIds = new Set(data.messages.map((message) => message.id));
  return [
    ...data.messages,
    ...data.tailMessages.filter((message) => !prefixIds.has(message.id)),
  ];
}

export function opportunityDetailReducer(
  state: OpportunityDetailState,
  event: OpportunityDetailEvent,
): OpportunityDetailState {
  if (event.type === 'started') {
    const canRefresh = state.requestKey === event.requestKey && state.data !== null;
    return {
      ...initialOpportunityDetailState,
      requestKey: event.requestKey,
      data: canRefresh ? state.data : null,
      loading: !canRefresh,
      refreshing: canRefresh,
      actionKind: canRefresh ? state.actionKind : null,
      actionError: canRefresh ? state.actionError : null,
      actionErrorKind: canRefresh ? state.actionErrorKind : null,
      actionNotice: canRefresh ? state.actionNotice : null,
      templates: canRefresh ? state.templates : [],
      templatesLoaded: canRefresh && state.templatesLoaded,
      templatesLoading: canRefresh && state.templatesLoading,
      templateError: canRefresh ? state.templateError : null,
    };
  }
  if (state.requestKey !== event.requestKey) return state;
  if (event.type === 'succeeded') {
    return {
      ...state,
      data: {
        detail: event.detail,
        messages: event.messages.items,
        tailMessages: [],
        messageNextOffset: event.messages.offset + event.messages.items.length,
        messageTotal: event.messages.total,
        messageLimit: event.messages.limit,
      },
      loading: false,
      refreshing: false,
      loadingMore: false,
      error: null,
      messageError: null,
    };
  }
  if (event.type === 'failed') {
    return {
      ...state,
      loading: false,
      refreshing: false,
      loadingMore: false,
      error: event.error,
    };
  }
  if (event.type === 'action-started') {
    if (state.actionKind) return state;
    return {
      ...state,
      actionKind: event.kind,
      actionError: null,
      actionErrorKind: null,
      actionNotice: null,
    };
  }
  if (event.type === 'action-failed') {
    return {
      ...state,
      actionKind: null,
      actionError: event.error,
      actionErrorKind: event.kind,
      actionNotice: null,
    };
  }
  if (event.type === 'status-queued') {
    return {
      ...state,
      actionKind: null,
      actionError: null,
      actionErrorKind: null,
      actionNotice: event.notice,
    };
  }
  if (event.type === 'detail-updated') {
    if (!state.data) return state;
    return {
      ...state,
      data: { ...state.data, detail: event.detail },
      actionKind: null,
      actionError: null,
      actionErrorKind: null,
      actionNotice: null,
    };
  }
  if (event.type === 'draft-succeeded') {
    if (!state.data) return state;
    return {
      ...state,
      data: {
        ...state.data,
        detail: { ...state.data.detail, aiReplyDraft: event.draft },
      },
      actionKind: null,
      actionError: null,
      actionErrorKind: null,
    };
  }
  if (event.type === 'reply-succeeded') {
    if (!state.data) return state;
    const currentVisible = visibleMessages(state.data);
    const alreadyVisible = currentVisible.some(
      (message) => message.id === event.result.message.id,
    );
    const fullyLoadedBeforeReply =
      state.data.messageNextOffset === state.data.messageTotal &&
      event.result.messageTotal === state.data.messageTotal + 1;
    const messages = !alreadyVisible && fullyLoadedBeforeReply
      ? [...state.data.messages, event.result.message]
      : state.data.messages;
    const tailMessages = alreadyVisible || fullyLoadedBeforeReply
      ? state.data.tailMessages
      : [...state.data.tailMessages, event.result.message];
    return {
      ...state,
      data: {
        ...state.data,
        detail: event.result.opportunity,
        messages,
        tailMessages,
        messageNextOffset: fullyLoadedBeforeReply
          ? event.result.messageTotal
          : state.data.messageNextOffset,
        messageTotal: event.result.messageTotal,
      },
      actionKind: null,
      actionError: null,
      actionErrorKind: null,
    };
  }
  if (event.type === 'templates-started') {
    if (state.templatesLoaded || state.templatesLoading) return state;
    return { ...state, templatesLoading: true, templateError: null };
  }
  if (event.type === 'templates-succeeded') {
    return {
      ...state,
      templates: event.templates,
      templatesLoaded: true,
      templatesLoading: false,
      templateError: null,
    };
  }
  if (event.type === 'templates-failed') {
    return { ...state, templatesLoading: false, templateError: event.error };
  }
  if (event.type === 'more-started') {
    if (
      !state.data ||
      state.loadingMore ||
      state.data.messageNextOffset >= state.data.messageTotal
    ) {
      return state;
    }
    return { ...state, loadingMore: true, messageError: null };
  }
  if (event.type === 'more-failed') {
    return { ...state, loadingMore: false, messageError: event.error };
  }
  if (!state.data || event.messages.offset !== state.data.messageNextOffset) return state;
  const existingIds = new Set(state.data.messages.map((message) => message.id));
  const newMessages = event.messages.items.filter((message) => !existingIds.has(message.id));
  const pageIds = new Set(event.messages.items.map((message) => message.id));
  return {
    ...state,
    data: {
      ...state.data,
      messages: [...state.data.messages, ...newMessages],
      tailMessages: state.data.tailMessages.filter((message) => !pageIds.has(message.id)),
      messageNextOffset: event.messages.offset + event.messages.items.length,
      messageTotal: event.messages.total,
      messageLimit: event.messages.limit,
    },
    loadingMore: false,
    messageError: null,
  };
}
