import { RadarApiError } from '@story2u/radar-api/client';

import { fallbackTranslator, type Translator } from '../../i18n/core';

export function opportunityDetailErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 404) return t('opportunity.error.notFound');
    if (error.status === 422) return t('opportunity.error.invalidLink');
    if (error.status >= 500) return t('opportunity.error.unavailable');
  }
  return t('opportunity.error.network');
}

export function messagePageErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError && error.status >= 500) {
    return t('opportunity.error.messagesUnavailable');
  }
  return t('opportunity.error.messagesNetwork');
}

export function manualReplyErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 409) return t('opportunity.error.replyConflict');
    if (error.status === 422) return t('opportunity.error.replyInvalid');
    if (error.status === 502) {
      return t('opportunity.error.replyUncertain');
    }
    if (error.status >= 500) return t('opportunity.error.replyUnavailable');
  }
  return t('opportunity.error.replyNetwork');
}

export function aiDraftErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 429) return t('opportunity.error.aiQuota');
    if (error.status === 503) return t('opportunity.error.aiUnavailable');
    if (error.status === 409) return t('opportunity.error.aiState');
  }
  return t('opportunity.error.aiNetwork');
}

export function opportunityActionErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError) {
    if (error.status === 409) return t('opportunity.error.actionConflict');
    if (error.status === 422) return t('opportunity.error.actionInvalid');
    if (error.status >= 500) return t('opportunity.error.unavailable');
  }
  return t('opportunity.error.actionNetwork');
}

export function templatesErrorMessage(error: unknown, t: Translator = fallbackTranslator) {
  if (error instanceof RadarApiError && error.status >= 500) {
    return t('opportunity.error.templatesUnavailable');
  }
  return t('opportunity.error.templatesNetwork');
}
