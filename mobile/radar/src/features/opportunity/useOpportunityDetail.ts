import { RadarApiError } from '@story2u/radar-api/client';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import { useCallback, useEffect, useReducer, useRef } from 'react';

import {
  claimOpportunity as claimOpportunityRequest,
  generateOpportunityAIDraft,
  readReplyTemplates,
  sendOpportunityManualReply,
  updateOpportunityStatus,
} from '../../api/client';
import { getMobileApiBaseUrl } from '../../config/mobileApiConfig';
import { useI18n } from '../../i18n/I18nProvider';
import { logEvent } from '../../logging/redactedLogger';
import { isOfflineReadFailure } from '../../sync/offlineFallback';
import {
  readMessagePageResilient,
  readOpportunityDetailBundleResilient,
} from '../../sync/resilientReads';
import {
  aiDraftErrorMessage,
  manualReplyErrorMessage,
  messagePageErrorMessage,
  opportunityActionErrorMessage,
  opportunityDetailErrorMessage,
  templatesErrorMessage,
} from './errors';
import {
  initialOpportunityDetailState,
  opportunityDetailReducer,
  type OpportunityActionKind,
} from './state';

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === 'AbortError';
}

export function useOpportunityDetail(
  opportunityId: string,
  ownerId: string,
  offlineEnabled: boolean,
  expireSession: () => Promise<void>,
  synchronize: () => Promise<void>,
  queueOpportunityStatus: (
    opportunityId: string,
    status: InternalOpportunityStatus,
  ) => Promise<void>,
) {
  const { t } = useI18n();
  const [state, dispatch] = useReducer(opportunityDetailReducer, initialOpportunityDetailState);
  const [revision, bumpRevision] = useReducer((value: number) => value + 1, 0);
  const moreController = useRef<AbortController | null>(null);
  const actionRunning = useRef(false);
  const templatesRunning = useRef(false);
  const activeOpportunityId = useRef(opportunityId);
  const syncOnNextLoad = useRef(false);

  const retry = useCallback(() => {
    moreController.current?.abort();
    syncOnNextLoad.current = true;
    bumpRevision();
  }, []);

  const expireIfUnauthorized = useCallback(async (error: unknown) => {
    if (error instanceof RadarApiError && error.status === 401) {
      await expireSession();
      return true;
    }
    return false;
  }, [expireSession]);

  useEffect(() => {
    const controller = new AbortController();
    moreController.current?.abort();
    if (activeOpportunityId.current !== opportunityId) {
      activeOpportunityId.current = opportunityId;
      actionRunning.current = false;
      templatesRunning.current = false;
    }
    dispatch({ type: 'started', requestKey: opportunityId });

    async function load() {
      try {
        if (syncOnNextLoad.current) {
          syncOnNextLoad.current = false;
          await synchronize();
        }
        const result = await readOpportunityDetailBundleResilient(
          getMobileApiBaseUrl(),
          { enabled: offlineEnabled, ownerId },
          opportunityId,
          controller.signal,
        );
        dispatch({
          type: 'succeeded',
          requestKey: opportunityId,
          detail: result.detail,
          messages: result.messages,
        });
      } catch (error) {
        if (isAbortError(error)) return;
        if (await expireIfUnauthorized(error)) return;
        logEvent('opportunity_detail.load_failed', {
          errorClass: error instanceof Error ? error.name : 'UnknownError',
          status: error instanceof RadarApiError ? error.status : null,
        });
        dispatch({
          type: 'failed',
          requestKey: opportunityId,
          error: opportunityDetailErrorMessage(error, t),
        });
      }
    }

    void load();
    return () => {
      controller.abort();
      moreController.current?.abort();
    };
  }, [expireIfUnauthorized, offlineEnabled, opportunityId, ownerId, revision, synchronize, t]);

  const loadMore = useCallback(async () => {
    const data = state.data;
    if (
      !data ||
      state.loadingMore ||
      data.messageNextOffset >= data.messageTotal
    ) return;
    const controller = new AbortController();
    moreController.current?.abort();
    moreController.current = controller;
    dispatch({ type: 'more-started', requestKey: opportunityId });
    try {
      const messages = await readMessagePageResilient(
        getMobileApiBaseUrl(),
        { enabled: offlineEnabled, ownerId },
        opportunityId,
        data.messageNextOffset,
        controller.signal,
      );
      dispatch({ type: 'more-succeeded', requestKey: opportunityId, messages });
    } catch (error) {
      if (isAbortError(error)) return;
      if (await expireIfUnauthorized(error)) return;
      logEvent('opportunity_detail.messages_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
        status: error instanceof RadarApiError ? error.status : null,
      });
      dispatch({
        type: 'more-failed',
        requestKey: opportunityId,
        error: messagePageErrorMessage(error, t),
      });
    } finally {
      if (moreController.current === controller) moreController.current = null;
    }
  }, [
    expireIfUnauthorized,
    offlineEnabled,
    opportunityId,
    ownerId,
    state.data,
    state.loadingMore,
    t,
  ]);

  const failAction = useCallback(async (
    kind: OpportunityActionKind,
    error: unknown,
    message: string,
  ) => {
    if (await expireIfUnauthorized(error)) return;
    logEvent('opportunity_detail.action_failed', {
      action: kind,
      errorClass: error instanceof Error ? error.name : 'UnknownError',
      status: error instanceof RadarApiError ? error.status : null,
    });
    dispatch({
      type: 'action-failed',
      requestKey: opportunityId,
      kind,
      error: message,
    });
  }, [expireIfUnauthorized, opportunityId]);

  const sendReply = useCallback(async (
    text: string,
    idempotencyKey: string,
  ): Promise<boolean> => {
    if (actionRunning.current) return false;
    actionRunning.current = true;
    dispatch({ type: 'action-started', requestKey: opportunityId, kind: 'reply' });
    try {
      const result = await sendOpportunityManualReply(
        getMobileApiBaseUrl(),
        opportunityId,
        text,
        idempotencyKey,
      );
      dispatch({ type: 'reply-succeeded', requestKey: opportunityId, result });
      return true;
    } catch (error) {
      await failAction('reply', error, manualReplyErrorMessage(error, t));
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [failAction, opportunityId, t]);

  const generateDraft = useCallback(async (): Promise<string | null> => {
    if (actionRunning.current) return null;
    actionRunning.current = true;
    dispatch({ type: 'action-started', requestKey: opportunityId, kind: 'draft' });
    try {
      const draft = await generateOpportunityAIDraft(getMobileApiBaseUrl(), opportunityId);
      dispatch({ type: 'draft-succeeded', requestKey: opportunityId, draft });
      return draft;
    } catch (error) {
      await failAction('draft', error, aiDraftErrorMessage(error, t));
      return null;
    } finally {
      actionRunning.current = false;
    }
  }, [failAction, opportunityId, t]);

  const setStatus = useCallback(async (
    status: InternalOpportunityStatus,
  ): Promise<boolean> => {
    if (actionRunning.current) return false;
    actionRunning.current = true;
    dispatch({ type: 'action-started', requestKey: opportunityId, kind: 'status' });
    try {
      const detail = await updateOpportunityStatus(
        getMobileApiBaseUrl(),
        opportunityId,
        status,
      );
      dispatch({ type: 'detail-updated', requestKey: opportunityId, detail });
      return true;
    } catch (error) {
      if (offlineEnabled && isOfflineReadFailure(error)) {
        try {
          await queueOpportunityStatus(opportunityId, status);
          dispatch({
            type: 'status-queued',
            requestKey: opportunityId,
            notice: t('opportunity.statusQueued'),
          });
          return true;
        } catch (queueError) {
          logEvent('opportunity_detail.status_queue_failed', {
            errorClass: queueError instanceof Error ? queueError.name : 'UnknownError',
          });
        }
      }
      await failAction('status', error, opportunityActionErrorMessage(error, t));
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [failAction, offlineEnabled, opportunityId, queueOpportunityStatus, t]);

  const claim = useCallback(async (): Promise<boolean> => {
    if (actionRunning.current) return false;
    actionRunning.current = true;
    dispatch({ type: 'action-started', requestKey: opportunityId, kind: 'claim' });
    try {
      const detail = await claimOpportunityRequest(getMobileApiBaseUrl(), opportunityId);
      dispatch({ type: 'detail-updated', requestKey: opportunityId, detail });
      return true;
    } catch (error) {
      await failAction('claim', error, opportunityActionErrorMessage(error, t));
      return false;
    } finally {
      actionRunning.current = false;
    }
  }, [failAction, opportunityId, t]);

  const loadTemplates = useCallback(async () => {
    if (state.templatesLoaded || templatesRunning.current) return;
    templatesRunning.current = true;
    dispatch({ type: 'templates-started', requestKey: opportunityId });
    try {
      const templates = await readReplyTemplates(getMobileApiBaseUrl());
      dispatch({ type: 'templates-succeeded', requestKey: opportunityId, templates });
    } catch (error) {
      if (await expireIfUnauthorized(error)) return;
      logEvent('opportunity_detail.templates_failed', {
        errorClass: error instanceof Error ? error.name : 'UnknownError',
        status: error instanceof RadarApiError ? error.status : null,
      });
      dispatch({
        type: 'templates-failed',
        requestKey: opportunityId,
        error: templatesErrorMessage(error, t),
      });
    } finally {
      templatesRunning.current = false;
    }
  }, [expireIfUnauthorized, opportunityId, state.templatesLoaded, t]);

  return {
    claim,
    generateDraft,
    loadMore,
    loadTemplates,
    retry,
    sendReply,
    setStatus,
    state,
  };
}
