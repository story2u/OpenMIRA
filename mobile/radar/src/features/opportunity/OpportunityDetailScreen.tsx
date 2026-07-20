import type { ChatMessage } from '@story2u/radar-contracts/messages';
import type { InternalOpportunityStatus } from '@story2u/radar-contracts/opportunity-actions';
import type { OpportunityDetail } from '@story2u/radar-contracts/opportunities';
import type { ReplyTemplate } from '@story2u/radar-contracts/templates';
import { randomUUID } from 'expo-crypto';
import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import {
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {
  ActivityIndicator,
  FlatList,
  KeyboardAvoidingView,
  Linking,
  Platform,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
  type ListRenderItemInfo,
} from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import type { AppLocale, MessageKey } from '../../i18n/catalog';
import type { Translator } from '../../i18n/core';
import { colors } from '../../ui/theme';
import { visibleMessages, type OpportunityActionKind } from './state';
import { useOpportunityDetail } from './useOpportunityDetail';

const internalStatusLabelKeys: Record<OpportunityDetail['internalStatus'], MessageKey> = {
  pending_human: 'opportunity.status.pendingHuman',
  ai_auto_reply: 'opportunity.status.aiAutoReply',
  replied: 'opportunity.status.replied',
  following: 'opportunity.status.following',
  ignored: 'opportunity.status.ignored',
  closed: 'opportunity.status.closed',
};
const priorityLabelKeys: Record<OpportunityDetail['priority'], MessageKey> = {
  low: 'opportunity.priority.low',
  normal: 'opportunity.priority.normal',
  high: 'opportunity.priority.high',
  urgent: 'opportunity.priority.urgent',
};
const analysisLabelKeys: Record<OpportunityDetail['agentAnalysisStatus'], MessageKey> = {
  not_requested: 'opportunity.analysis.notRequested',
  quota_exceeded: 'opportunity.analysis.quotaExceeded',
  queued: 'opportunity.analysis.queued',
  running: 'opportunity.analysis.running',
  completed: 'opportunity.analysis.completed',
  failed: 'opportunity.analysis.failed',
};
const statusActions: Partial<Record<InternalOpportunityStatus, InternalOpportunityStatus[]>> = {
  pending_human: ['following', 'ignored', 'closed'],
  ai_auto_reply: ['pending_human', 'following', 'closed'],
  replied: ['following', 'ignored', 'closed'],
  following: ['replied', 'ignored', 'closed'],
  ignored: ['pending_human', 'closed'],
};
type AgentAction = NonNullable<OpportunityDetail['agentActions']>[number];

const actionLabelKeys: Record<AgentAction['actionType'], MessageKey> = {
  send_email: 'opportunity.agentAction.sendEmail',
  add_friend: 'opportunity.agentAction.addFriend',
  private_message: 'opportunity.agentAction.privateMessage',
  notify_user: 'opportunity.agentAction.notifyUser',
};
const MAX_AGENT_ACTIONS = 12;

function formatTimestamp(value: string, locale: AppLocale, t: Translator) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return t('common.unknownTime');
  return date.toLocaleString(locale, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.detailRow}>
      <Text style={styles.detailLabel}>{label}</Text>
      <Text selectable style={styles.detailValue}>{value}</Text>
    </View>
  );
}

function SectionTitle({ children }: { children: ReactNode }) {
  return <Text accessibilityRole="header" style={styles.sectionTitle}>{children}</Text>;
}

const DetailHeader = memo(function DetailHeader({
  detail,
  error,
  onRetry,
}: {
  detail: OpportunityDetail;
  error: string | null;
  onRetry(): void;
}) {
  const { locale, t } = useI18n();
  const allAgentActions = detail.agentActions ?? [];
  const agentActions = allAgentActions.slice(0, MAX_AGENT_ACTIONS);
  const isJob = detail.opportunityType === 'job';
  const rawMessageLinks = detail.rawMessageLinks ?? [];
  const sourceLabel = detail.sourceType === 'group'
    ? (detail.groupName ? `${t('dashboard.source.group')} · ${detail.groupName}` : t('dashboard.source.group'))
    : t('dashboard.source.private');
  const reasons = [
    detail.attentionRequired ? t('opportunity.reason.attention') : null,
    detail.detectionReason,
    detail.matchedKeywords.length > 0 ? t('opportunity.keywords', {
      keywords: detail.matchedKeywords.join(locale === 'zh-CN' ? '、' : ', '),
    }) : null,
    ...agentActions.map((action) => action.reason),
  ].filter((value): value is string => Boolean(value));
  const risks = [
    detail.agentAnalysisError ? t('opportunity.analysisIncomplete') : null,
    detail.agentAnalysisStatus !== 'completed'
      ? t('opportunity.risk.analysisState', { status: t(analysisLabelKeys[detail.agentAnalysisStatus]) })
      : null,
    detail.trustScore < 50 ? t('opportunity.risk.lowTrust') : null,
    rawMessageLinks.length > 0
      ? t('opportunity.evidence.linkCount', { count: rawMessageLinks.length })
      : null,
  ].filter((value): value is string => Boolean(value));

  return (
    <View>
      {error ? (
        <View accessibilityRole="alert" style={styles.errorBanner}>
          <Text style={styles.errorBannerText}>{error}</Text>
          <Pressable accessibilityRole="button" onPress={onRetry}>
            <Text style={styles.inlineRetry}>{t('common.retry')}</Text>
          </Pressable>
        </View>
      ) : null}

      {detail.archivedAt ? (
        <View style={styles.archivedBanner}>
          <Text style={styles.archivedTitle}>{t('opportunity.archived')}</Text>
          <Text style={styles.archivedText}>{t('opportunity.archivedDetail')}</Text>
          {detail.archiveReason ? (
            <Text style={styles.archivedText}>{t('opportunity.archiveReason', { reason: detail.archiveReason })}</Text>
          ) : null}
        </View>
      ) : null}

      <View style={[styles.sectionCard, detail.attentionRequired && styles.attentionCard]}>
        <Text style={styles.kindLabel}>
          {t(isJob ? 'opportunity.kind.job' : 'opportunity.kind.business')}
        </Text>
        <View style={styles.contactRow}>
          <View style={styles.avatar}>
            <Text style={styles.avatarText}>{detail.contactName.trim().slice(0, 1) || '?'}</Text>
          </View>
          <View style={styles.contactCopy}>
            <Text style={styles.contactName}>{detail.contactName}</Text>
            <Text style={styles.contactMeta}>
              {detail.platform === 'telegram' ? 'Telegram' : t('dashboard.platform.wecom')} · {t(
                detail.sourceType === 'group' ? 'dashboard.source.group' : 'dashboard.source.private',
              )}
            </Text>
          </View>
          <View style={styles.confidenceBadge}>
            <Text style={styles.confidenceValue}>{Math.round(detail.confidenceScore * 100)}%</Text>
            <Text style={styles.confidenceLabel}>{t('opportunity.relevance')}</Text>
          </View>
        </View>
        {detail.attentionRequired ? (
          <Text style={styles.attentionText}>{t('opportunity.attention')}</Text>
        ) : null}
        <Text style={styles.summary}>{detail.summary}</Text>
      </View>

      <View style={styles.sectionCard}>
        <SectionTitle>{t('opportunity.miraConclusion')}</SectionTitle>
        <Text style={styles.conclusionText}>{detail.summary}</Text>
      </View>

      <View style={styles.sectionCard}>
        <SectionTitle>{t('opportunity.keyFacts')}</SectionTitle>
        <DetailRow label={t('opportunity.field.status')} value={t(internalStatusLabelKeys[detail.internalStatus])} />
        <DetailRow label={t('opportunity.field.priority')} value={t(priorityLabelKeys[detail.priority])} />
        <DetailRow label={t('opportunity.field.trust')} value={`${detail.trustScore} / 100`} />
        <DetailRow label={t('opportunity.field.source')} value={sourceLabel} />
        <DetailRow label={t('opportunity.field.updatedAt')} value={formatTimestamp(detail.updatedAt, locale, t)} />
      </View>

      <View style={styles.sectionCard}>
        <SectionTitle>{t('opportunity.whyRelevant')}</SectionTitle>
        {reasons.length > 0 ? reasons.slice(0, 8).map((reason, index) => (
          <Text key={`${reason}-${index}`} style={styles.bulletText}>• {reason}</Text>
        )) : (
          <Text style={styles.emptyCopy}>{t('opportunity.noFindings')}</Text>
        )}
      </View>

      <View style={styles.sectionCard}>
        <SectionTitle>{t('opportunity.riskReview')}</SectionTitle>
        {risks.length > 0 ? risks.map((risk, index) => (
          <Text key={`${risk}-${index}`} style={styles.bulletText}>• {risk}</Text>
        )) : (
          <Text style={styles.emptyCopy}>{t('opportunity.risk.none')}</Text>
        )}
      </View>

      {agentActions.length > 0 ? (
        <View style={styles.sectionCard}>
          <SectionTitle>{t('opportunity.nextSteps')}</SectionTitle>
          {agentActions.map((action, index) => (
            <View key={`${action.actionType}-${index}`} style={styles.actionCard}>
              <View style={styles.actionTitleRow}>
                <Text style={styles.actionTitle}>{t(actionLabelKeys[action.actionType])}</Text>
                {action.requiresApproval ? <Text style={styles.approvalBadge}>{t('opportunity.approvalRequired')}</Text> : null}
              </View>
              <Text style={styles.actionReason}>{action.reason}</Text>
              {action.draft && !isJob ? <Text style={styles.actionDraft}>{action.draft}</Text> : null}
            </View>
          ))}
          {allAgentActions.length > agentActions.length ? (
            <Text style={styles.omittedCopy}>{t('opportunity.omittedActions', { count: allAgentActions.length - agentActions.length })}</Text>
          ) : null}
        </View>
      ) : null}

      {detail.aiReplyDraft || detail.finalReply ? (
        <View style={styles.sectionCard}>
          <SectionTitle>{t('opportunity.replyRecord')}</SectionTitle>
          {detail.aiReplyDraft ? <DetailRow label={t('opportunity.aiDraftExisting')} value={detail.aiReplyDraft} /> : null}
          {detail.finalReply ? <DetailRow label={t('opportunity.finalReply')} value={detail.finalReply} /> : null}
        </View>
      ) : null}

      <SectionTitle>{t('opportunity.messageHistory')}</SectionTitle>
    </View>
  );
});

const OpportunityActions = memo(function OpportunityActions({
  actionError,
  actionKind,
  actionNotice,
  currentUserId,
  detail,
  onClaim,
  onStatus,
}: {
  actionError: string | null;
  actionKind: OpportunityActionKind | null;
  actionNotice: string | null;
  currentUserId: string | null;
  detail: OpportunityDetail;
  onClaim(): Promise<boolean>;
  onStatus(status: InternalOpportunityStatus): Promise<boolean>;
}) {
  const { t } = useI18n();
  const busy = actionKind !== null;
  const actions = statusActions[detail.internalStatus] ?? [];
  return (
    <View style={styles.sectionCard}>
      <SectionTitle>{t('opportunity.claimAndStatus')}</SectionTitle>
      <DetailRow label={t('opportunity.currentStatus')} value={t(internalStatusLabelKeys[detail.internalStatus])} />
      <DetailRow
        label={t('opportunity.owner')}
        value={
          detail.assignedTo
            ? detail.assignedTo === currentUserId
              ? t('opportunity.claimedByYou')
              : t('opportunity.claimedByOther')
            : t('opportunity.unclaimed')
        }
      />
      {actionError ? (
        <Text accessibilityRole="alert" style={styles.actionError}>{actionError}</Text>
      ) : null}
      {actionNotice ? (
        <Text accessibilityLiveRegion="polite" style={styles.actionNotice}>{actionNotice}</Text>
      ) : null}
      {!detail.archivedAt && !detail.assignedTo ? (
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ busy: actionKind === 'claim', disabled: busy }}
          disabled={busy}
          onPress={() => void onClaim()}
          style={styles.primaryActionButton}
        >
          {actionKind === 'claim' ? <ActivityIndicator color={colors.text} /> : null}
          <Text style={styles.primaryActionText}>
            {actionKind === 'claim' ? t('opportunity.claiming') : t('opportunity.claim')}
          </Text>
        </Pressable>
      ) : null}
      {!detail.archivedAt && actions.length > 0 ? (
        <View style={styles.statusActionRow}>
          {actions.map((status) => (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ disabled: busy }}
              disabled={busy}
              key={status}
              onPress={() => void onStatus(status)}
              style={[
                styles.statusActionButton,
                (status === 'ignored' || status === 'closed') && styles.dangerActionButton,
              ]}
            >
              <Text style={styles.statusActionText}>{t(internalStatusLabelKeys[status])}</Text>
            </Pressable>
          ))}
        </View>
      ) : null}
    </View>
  );
});

const JobActionPanel = memo(function JobActionPanel({
  actionError,
  actionKind,
  detail,
  onStatus,
}: {
  actionError: string | null;
  actionKind: OpportunityActionKind | null;
  detail: OpportunityDetail;
  onStatus(status: InternalOpportunityStatus): Promise<boolean>;
}) {
  const router = useRouter();
  const { t } = useI18n();
  const busy = actionKind !== null;
  const sourceUrl = detail.rawMessageLinks?.[0] ?? null;
  const openPrepAdvice = () => {
    const prompt = t('agent.context.prepareJob', { opportunityId: detail.id });
    router.push(`/(tabs)/agent?prompt=${encodeURIComponent(prompt)}` as Href);
  };
  return (
    <View style={styles.sectionCard}>
      <SectionTitle>{t('opportunity.job.actions.title')}</SectionTitle>
      <Text style={styles.jobActionDetail}>{t('opportunity.job.actions.detail')}</Text>
      {actionError ? (
        <Text accessibilityRole="alert" style={styles.actionError}>{actionError}</Text>
      ) : null}
      <View style={styles.jobActionGrid}>
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ disabled: busy }}
          disabled={busy}
          onPress={() => void onStatus('following')}
          style={styles.jobActionButton}
        >
          <Text style={styles.jobActionText}>{t('opportunity.job.save')}</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ disabled: busy }}
          disabled={busy}
          onPress={openPrepAdvice}
          style={styles.jobActionButton}
        >
          <Text style={styles.jobActionText}>{t('opportunity.job.prepare')}</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ disabled: !sourceUrl }}
          disabled={!sourceUrl}
          onPress={() => {
            if (sourceUrl) void Linking.openURL(sourceUrl);
          }}
          style={[styles.jobActionButton, !sourceUrl && styles.disabled]}
        >
          <Text style={styles.jobActionText}>{t('opportunity.job.source')}</Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ disabled: busy }}
          disabled={busy}
          onPress={() => void onStatus('ignored')}
          style={[styles.jobActionButton, styles.jobDismissButton]}
        >
          <Text style={styles.jobActionText}>{t('opportunity.job.notInterested')}</Text>
        </Pressable>
      </View>
      {!sourceUrl ? <Text style={styles.omittedCopy}>{t('opportunity.job.sourceUnavailable')}</Text> : null}
    </View>
  );
});

function fillTemplate(content: string, detail: OpportunityDetail, t: Translator) {
  return content
    .replaceAll('{{联系人姓名}}', detail.contactName)
    .replaceAll('{{群名称}}', detail.groupName ?? t('opportunity.template.groupFallback'))
    .replaceAll('{{公司名称}}', t('opportunity.template.companyFallback'))
    .replaceAll('{{工作时间}}', t('opportunity.template.workHoursFallback'));
}

function ReplyComposer({
  actionError,
  actionKind,
  detail,
  onGenerate,
  onLoadTemplates,
  onSend,
  templateError,
  templates,
  templatesLoaded,
  templatesLoading,
}: {
  actionError: string | null;
  actionKind: OpportunityActionKind | null;
  detail: OpportunityDetail;
  onGenerate(): Promise<string | null>;
  onLoadTemplates(): Promise<void>;
  onSend(text: string, idempotencyKey: string): Promise<boolean>;
  templateError: string | null;
  templates: ReplyTemplate[];
  templatesLoaded: boolean;
  templatesLoading: boolean;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState(detail.aiReplyDraft ?? '');
  const [showTemplates, setShowTemplates] = useState(false);
  const pendingRequest = useRef<{ text: string; key: string } | null>(null);
  const disabled = Boolean(detail.archivedAt) || ['closed', 'ignored'].includes(
    detail.internalStatus,
  );
  const busy = actionKind !== null;

  useEffect(() => {
    pendingRequest.current = null;
    setDraft(detail.aiReplyDraft ?? '');
    setShowTemplates(false);
  }, [detail.id]);

  const updateDraft = (value: string) => {
    if (pendingRequest.current?.text !== value.trim()) pendingRequest.current = null;
    setDraft(value);
  };

  const generate = async () => {
    const generated = await onGenerate();
    if (generated) {
      pendingRequest.current = null;
      setDraft(generated);
    }
  };

  const send = async () => {
    const text = draft.trim();
    if (!text || disabled || busy) return;
    const request = pendingRequest.current?.text === text
      ? pendingRequest.current
      : { text, key: randomUUID() };
    pendingRequest.current = request;
    if (await onSend(request.text, request.key)) {
      pendingRequest.current = null;
      setDraft('');
    }
  };

  const toggleTemplates = () => {
    const next = !showTemplates;
    setShowTemplates(next);
    if (next && !templatesLoaded) void onLoadTemplates();
  };

  return (
    <View style={styles.composerCard}>
      <SectionTitle>{t('opportunity.manualReply')}</SectionTitle>
      {disabled ? (
        <Text style={styles.composerHint}>{t('opportunity.replyDisabled')}</Text>
      ) : null}
      <TextInput
        accessibilityLabel={t('opportunity.replyContent')}
        editable={!disabled && actionKind !== 'reply'}
        maxLength={4000}
        multiline
        onChangeText={updateDraft}
        placeholder={t('opportunity.replyPlaceholder')}
        placeholderTextColor={colors.placeholder}
        style={styles.replyInput}
        textAlignVertical="top"
        value={draft}
      />
      {actionError ? (
        <Text accessibilityRole="alert" style={styles.actionError}>{actionError}</Text>
      ) : null}
      <View style={styles.composerActionRow}>
        <Pressable
          accessibilityRole="button"
          disabled={busy || disabled}
          onPress={() => void generate()}
          style={styles.secondaryActionButton}
        >
          {actionKind === 'draft' ? <ActivityIndicator color={colors.text} /> : null}
          <Text style={styles.secondaryActionText}>
            {actionKind === 'draft' ? t('opportunity.generating') : t('opportunity.generateDraft')}
          </Text>
        </Pressable>
        <Pressable
          accessibilityRole="button"
          disabled={busy || disabled || !draft.trim()}
          onPress={() => void send()}
          style={[styles.primaryActionButton, styles.sendButton]}
        >
          {actionKind === 'reply' ? <ActivityIndicator color={colors.text} /> : null}
          <Text style={styles.primaryActionText}>
            {actionKind === 'reply' ? t('opportunity.sending') : t('opportunity.send')}
          </Text>
        </Pressable>
      </View>
      <Pressable accessibilityRole="button" onPress={toggleTemplates} style={styles.templateToggle}>
        <Text style={styles.templateToggleText}>
          {showTemplates ? t('opportunity.templates.collapse') : t('opportunity.templates.choose')}
        </Text>
      </Pressable>
      {showTemplates ? (
        <View style={styles.templateArea}>
          {templatesLoading ? <ActivityIndicator color={colors.accent} /> : null}
          {templateError ? (
            <View style={styles.templateErrorRow}>
              <Text accessibilityRole="alert" style={styles.paginationError}>{templateError}</Text>
              <Pressable accessibilityRole="button" onPress={() => void onLoadTemplates()}>
                <Text style={styles.inlineRetry}>{t('common.retry')}</Text>
              </Pressable>
            </View>
          ) : null}
          {templatesLoaded && templates.length === 0 ? (
            <Text style={styles.emptyCopy}>{t('opportunity.templates.empty')}</Text>
          ) : null}
          {templates.length > 0 ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              <View style={styles.templateRow}>
                {templates.slice(0, 20).map((template) => (
                  <Pressable
                    accessibilityRole="button"
                    key={template.id}
                    onPress={() => {
                      pendingRequest.current = null;
                      setDraft(fillTemplate(template.content, detail, t));
                    }}
                    style={styles.templateChip}
                  >
                    <Text style={styles.templateChipText}>{template.title}</Text>
                  </Pressable>
                ))}
              </View>
            </ScrollView>
          ) : null}
          {templates.length > 20 ? (
            <Text style={styles.omittedCopy}>{t('opportunity.templates.limit')}</Text>
          ) : null}
        </View>
      ) : null}
    </View>
  );
}

const MessageBubble = memo(function MessageBubble({ message }: { message: ChatMessage }) {
  const { locale, t } = useI18n();
  return (
    <View style={[
      styles.messageRow,
      message.isFromContact ? styles.incomingRow : styles.outgoingRow,
    ]}>
      <View style={styles.messageMetaRow}>
        <Text style={styles.messageSender}>{message.senderName}</Text>
        {message.source === 'ai' ? <Text style={styles.aiBadge}>AI</Text> : null}
        <Text style={styles.messageTime}>{formatTimestamp(message.sentAt, locale, t)}</Text>
      </View>
      <View style={[
        styles.messageBubble,
        message.isFromContact ? styles.incomingBubble : styles.outgoingBubble,
      ]}>
        <Text selectable style={styles.messageContent}>{message.content || t('opportunity.emptyMessage')}</Text>
      </View>
    </View>
  );
});

function FullPageState({
  error,
  loading,
  onBack,
  onRetry,
}: {
  error: string | null;
  loading: boolean;
  onBack(): void;
  onRetry(): void;
}) {
  const { t } = useI18n();
  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.topBar}>
        <Pressable accessibilityRole="button" onPress={onBack} style={styles.backButton}>
          <Text style={styles.backButtonText}>‹ {t('common.back')}</Text>
        </Pressable>
        <Text accessibilityRole="header" style={styles.topBarTitle}>{t('opportunity.title')}</Text>
        <View style={styles.topBarSpacer} />
      </View>
      <View style={styles.fullPageState}>
        {loading ? <ActivityIndicator color={colors.accent} size="large" /> : null}
        <Text accessibilityRole="header" style={styles.stateTitle}>
          {loading ? t('opportunity.loading') : t('opportunity.loadFailed')}
        </Text>
        {error ? <Text accessibilityRole="alert" style={styles.stateDescription}>{error}</Text> : null}
        {!loading ? (
          <Pressable accessibilityRole="button" onPress={onRetry} style={styles.retryButton}>
            <Text style={styles.retryButtonText}>{t('common.retry')}</Text>
          </Pressable>
        ) : null}
      </View>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

export default function OpportunityDetailScreen({ opportunityId }: { opportunityId: string }) {
  const router = useRouter();
  const {
    capabilities,
    expireSession,
    queueOpportunityStatus,
    state: sessionState,
    synchronize,
  } = useSession();
  const { t } = useI18n();
  const {
    claim,
    generateDraft,
    loadMore,
    loadTemplates,
    retry,
    sendReply,
    setStatus,
    state,
  } = useOpportunityDetail(
    opportunityId,
    sessionState.status === 'authenticated' ? sessionState.user.id : '',
    capabilities.syncAvailable,
    expireSession,
    synchronize,
    queueOpportunityStatus,
  );
  const requestMatches = state.requestKey === opportunityId;
  const data = requestMatches ? state.data : null;

  const goBack = useCallback(() => {
    if (router.canGoBack()) router.back();
    else router.replace('/(tabs)/dashboard');
  }, [router]);
  const renderMessage = useCallback(
    ({ item }: ListRenderItemInfo<ChatMessage>) => <MessageBubble message={item} />,
    [],
  );

  if (!data) {
    return (
      <FullPageState
        error={requestMatches ? state.error : null}
        loading={!requestMatches || state.loading}
        onBack={goBack}
        onRetry={retry}
      />
    );
  }

  const messages = visibleMessages(data);
  const hasMore = data.messageNextOffset < data.messageTotal;
  const currentUserId = sessionState.status === 'authenticated' ? sessionState.user.id : null;
  const statusActionError = state.actionErrorKind === 'claim' || state.actionErrorKind === 'status'
    ? state.actionError
    : null;
  const replyActionError = state.actionErrorKind === 'draft' || state.actionErrorKind === 'reply'
    ? state.actionError
    : null;
  const isJob = data.detail.opportunityType === 'job';
  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.topBar}>
        <Pressable accessibilityRole="button" onPress={goBack} style={styles.backButton}>
          <Text style={styles.backButtonText}>‹ {t('common.back')}</Text>
        </Pressable>
        <Text accessibilityRole="header" style={styles.topBarTitle}>{t('opportunity.title')}</Text>
        <View style={styles.topBarSpacer} />
      </View>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.content}
      >
        <FlatList
          contentContainerStyle={styles.listContent}
          data={messages}
          initialNumToRender={12}
          keyExtractor={(message) => message.id}
          keyboardShouldPersistTaps="handled"
          ListEmptyComponent={(
            <View style={styles.messageEmpty}>
              <Text style={styles.stateTitle}>{t('opportunity.messages.empty')}</Text>
              <Text style={styles.emptyCopy}>{t('opportunity.messages.emptyDetail')}</Text>
            </View>
          )}
          ListFooterComponent={(
            <View>
              <View style={styles.messageFooter}>
                <Text accessibilityLiveRegion="polite" style={styles.messageCount}>
                  {t('opportunity.messages.count', { shown: messages.length, total: data.messageTotal })}
                </Text>
                {state.messageError ? (
                  <Text accessibilityRole="alert" style={styles.paginationError}>{state.messageError}</Text>
                ) : null}
                {hasMore ? (
                  <Pressable
                    accessibilityRole="button"
                    accessibilityState={{ busy: state.loadingMore, disabled: state.loadingMore }}
                    disabled={state.loadingMore}
                    onPress={() => void loadMore()}
                    style={styles.loadMoreButton}
                  >
                    {state.loadingMore ? <ActivityIndicator color={colors.text} /> : null}
                    <Text style={styles.loadMoreText}>
                      {state.loadingMore
                        ? t('opportunity.messages.loadingMore')
                        : t('opportunity.messages.loadMore')}
                    </Text>
                  </Pressable>
                ) : null}
              </View>
              {isJob ? null : (
                <ReplyComposer
                  actionError={replyActionError}
                  actionKind={state.actionKind}
                  detail={data.detail}
                  onGenerate={generateDraft}
                  onLoadTemplates={loadTemplates}
                  onSend={sendReply}
                  templateError={state.templateError}
                  templates={state.templates}
                  templatesLoaded={state.templatesLoaded}
                  templatesLoading={state.templatesLoading}
                />
              )}
            </View>
          )}
          ListHeaderComponent={(
            <View>
              <DetailHeader detail={data.detail} error={state.error} onRetry={retry} />
              <OpportunityActions
                actionError={statusActionError}
                actionKind={state.actionKind}
                actionNotice={state.actionNotice}
                currentUserId={currentUserId}
                detail={data.detail}
                onClaim={claim}
                onStatus={setStatus}
              />
              {isJob ? (
                <JobActionPanel
                  actionError={statusActionError}
                  actionKind={state.actionKind}
                  detail={data.detail}
                  onStatus={setStatus}
                />
              ) : null}
              <Pressable
                accessibilityRole="button"
                onPress={() => {
                  const prompt = t('agent.context.teachOpportunity', { opportunityId: data.detail.id });
                  router.push(`/(tabs)/agent?prompt=${encodeURIComponent(prompt)}` as Href);
                }}
                style={styles.teachPiCard}
              >
                <View style={styles.teachPiCopy}>
                  <Text style={styles.teachPiTitle}>{t('opportunity.teachPi.title')}</Text>
                  <Text style={styles.teachPiDetail}>{t('opportunity.teachPi.detail')}</Text>
                </View>
                <Text style={styles.teachPiArrow}>›</Text>
              </Pressable>
            </View>
          )}
          onRefresh={retry}
          refreshing={state.refreshing}
          renderItem={renderMessage}
          showsVerticalScrollIndicator={false}
          windowSize={7}
        />
      </KeyboardAvoidingView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  content: { flex: 1 },
  topBar: { minHeight: 52, flexDirection: 'row', alignItems: 'center', borderBottomWidth: 1, borderBottomColor: colors.border, paddingHorizontal: 16 },
  backButton: { minWidth: 70, paddingVertical: 10 },
  backButtonText: { color: colors.accent, fontSize: 15, fontWeight: '700' },
  topBarTitle: { flex: 1, color: colors.text, fontSize: 17, fontWeight: '800', textAlign: 'center' },
  topBarSpacer: { width: 70 },
  listContent: { flexGrow: 1, padding: 16, paddingBottom: 34 },
  errorBanner: { marginBottom: 14, flexDirection: 'row', alignItems: 'center', gap: 10, borderRadius: 12, backgroundColor: colors.errorBackground, padding: 12 },
  errorBannerText: { flex: 1, color: colors.errorText, fontSize: 13, lineHeight: 18 },
  inlineRetry: { color: colors.text, fontSize: 13, fontWeight: '800' },
  archivedBanner: { marginBottom: 14, gap: 5, borderWidth: 1, borderColor: colors.border, borderRadius: 14, backgroundColor: colors.card, padding: 14 },
  archivedTitle: { color: colors.text, fontSize: 14, fontWeight: '800' },
  archivedText: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  sectionCard: { marginBottom: 14, borderWidth: 1, borderColor: colors.border, borderRadius: 16, backgroundColor: colors.card, padding: 16 },
  attentionCard: { borderColor: colors.warning },
  contactRow: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  avatar: { width: 48, height: 48, alignItems: 'center', justifyContent: 'center', borderRadius: 24, backgroundColor: colors.accentMuted },
  avatarText: { color: colors.accent, fontSize: 20, fontWeight: '900' },
  contactCopy: { flex: 1 },
  contactName: { color: colors.text, fontSize: 17, fontWeight: '800' },
  contactMeta: { marginTop: 4, color: colors.mutedText, fontSize: 12 },
  kindLabel: { marginBottom: 10, color: colors.accent, fontSize: 11, fontWeight: '900', letterSpacing: 1 },
  confidenceBadge: { alignItems: 'center', borderRadius: 12, backgroundColor: colors.background, paddingHorizontal: 10, paddingVertical: 7 },
  confidenceValue: { color: colors.accent, fontSize: 16, fontWeight: '900' },
  confidenceLabel: { color: colors.subtleText, fontSize: 10 },
  attentionText: { marginTop: 14, color: colors.warning, fontSize: 12, fontWeight: '800' },
  summary: { marginTop: 12, color: colors.text, fontSize: 14, lineHeight: 21 },
  keywords: { marginTop: 10, color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  sectionTitle: { marginBottom: 12, color: colors.text, fontSize: 16, fontWeight: '800' },
  conclusionText: { color: colors.text, fontSize: 14, lineHeight: 21 },
  bulletText: { marginBottom: 8, color: colors.mutedText, fontSize: 13, lineHeight: 19 },
  detailRow: { marginBottom: 9, gap: 3 },
  detailLabel: { color: colors.subtleText, fontSize: 11, fontWeight: '700' },
  detailValue: { color: colors.text, fontSize: 13, lineHeight: 19 },
  agentError: { marginBottom: 10, color: colors.errorText, fontSize: 13 },
  omittedCopy: { marginBottom: 9, color: colors.subtleText, fontSize: 11, lineHeight: 17 },
  actionCard: { marginTop: 8, gap: 6, borderRadius: 12, backgroundColor: colors.background, padding: 12 },
  actionTitleRow: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  actionTitle: { flex: 1, color: colors.text, fontSize: 13, fontWeight: '800' },
  approvalBadge: { borderRadius: 10, backgroundColor: colors.noticeBackground, color: colors.noticeText, fontSize: 10, fontWeight: '800', paddingHorizontal: 8, paddingVertical: 4 },
  actionReason: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  actionDraft: { color: colors.text, fontSize: 12, lineHeight: 18 },
  actionError: { marginVertical: 8, color: colors.errorText, fontSize: 12, lineHeight: 18 },
  actionNotice: { marginVertical: 8, color: colors.noticeText, fontSize: 12, lineHeight: 18 },
  primaryActionButton: { minHeight: 42, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8, borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 16, paddingVertical: 10 },
  primaryActionText: { color: colors.text, fontSize: 13, fontWeight: '800' },
  disabled: { opacity: 0.38 },
  statusActionRow: { marginTop: 10, flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  statusActionButton: { minHeight: 38, justifyContent: 'center', borderWidth: 1, borderColor: colors.border, borderRadius: 11, paddingHorizontal: 12, paddingVertical: 8 },
  dangerActionButton: { borderColor: colors.danger },
  statusActionText: { color: colors.text, fontSize: 12, fontWeight: '700' },
  jobActionDetail: { marginBottom: 12, color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  jobActionGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 9 },
  jobActionButton: { width: '48%', flexGrow: 1, minHeight: 44, alignItems: 'center', justifyContent: 'center', borderWidth: 1, borderColor: colors.border, borderRadius: 12, backgroundColor: colors.background, paddingHorizontal: 10, paddingVertical: 10 },
  jobDismissButton: { borderColor: colors.danger },
  jobActionText: { color: colors.text, fontSize: 12, fontWeight: '800', textAlign: 'center' },
  teachPiCard: { minHeight: 70, flexDirection: 'row', alignItems: 'center', gap: 12, marginBottom: 16, borderWidth: 1, borderColor: '#27516a', borderRadius: 16, backgroundColor: '#0b1b2d', padding: 14 },
  teachPiCopy: { flex: 1, gap: 4 },
  teachPiTitle: { color: colors.text, fontSize: 14, fontWeight: '900' },
  teachPiDetail: { color: colors.mutedText, fontSize: 11, lineHeight: 16 },
  teachPiArrow: { color: colors.accent, fontSize: 25 },
  messageRow: { marginBottom: 14, maxWidth: '88%' },
  incomingRow: { alignSelf: 'flex-start' },
  outgoingRow: { alignSelf: 'flex-end' },
  messageMetaRow: { marginBottom: 5, flexDirection: 'row', alignItems: 'center', gap: 6 },
  messageSender: { color: colors.mutedText, fontSize: 11, fontWeight: '700' },
  aiBadge: { borderRadius: 8, backgroundColor: colors.accentMuted, color: colors.accent, fontSize: 9, fontWeight: '900', paddingHorizontal: 6, paddingVertical: 2 },
  messageTime: { color: colors.subtleText, fontSize: 10 },
  messageBubble: { borderRadius: 13, paddingHorizontal: 12, paddingVertical: 10 },
  incomingBubble: { backgroundColor: colors.card },
  outgoingBubble: { backgroundColor: colors.accentMuted },
  messageContent: { color: colors.text, fontSize: 14, lineHeight: 21 },
  messageEmpty: { alignItems: 'center', gap: 7, paddingVertical: 32 },
  emptyCopy: { color: colors.mutedText, fontSize: 13, lineHeight: 19, textAlign: 'center' },
  messageFooter: { alignItems: 'center', gap: 10, paddingTop: 8, paddingBottom: 24 },
  messageCount: { color: colors.subtleText, fontSize: 11 },
  paginationError: { color: colors.errorText, fontSize: 12, textAlign: 'center' },
  loadMoreButton: { minHeight: 42, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8, borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 18, paddingVertical: 10 },
  loadMoreText: { color: colors.text, fontSize: 13, fontWeight: '800' },
  composerCard: { marginTop: 4, marginBottom: 20, borderWidth: 1, borderColor: colors.border, borderRadius: 16, backgroundColor: colors.card, padding: 16 },
  composerHint: { marginBottom: 10, color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  replyInput: { minHeight: 112, borderWidth: 1, borderColor: colors.border, borderRadius: 12, backgroundColor: colors.background, color: colors.text, fontSize: 14, lineHeight: 21, paddingHorizontal: 12, paddingVertical: 11 },
  composerActionRow: { marginTop: 10, flexDirection: 'row', alignItems: 'center', gap: 9 },
  secondaryActionButton: { minHeight: 42, flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8, borderWidth: 1, borderColor: colors.border, borderRadius: 12, paddingHorizontal: 12, paddingVertical: 10 },
  secondaryActionText: { color: colors.text, fontSize: 12, fontWeight: '800' },
  sendButton: { flex: 1 },
  templateToggle: { alignSelf: 'flex-start', marginTop: 13, paddingVertical: 5 },
  templateToggleText: { color: colors.accent, fontSize: 12, fontWeight: '800' },
  templateArea: { marginTop: 8, gap: 9 },
  templateErrorRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 10 },
  templateRow: { flexDirection: 'row', gap: 8, paddingVertical: 3 },
  templateChip: { maxWidth: 220, borderWidth: 1, borderColor: colors.border, borderRadius: 18, backgroundColor: colors.background, paddingHorizontal: 12, paddingVertical: 8 },
  templateChipText: { color: colors.text, fontSize: 12, fontWeight: '700' },
  fullPageState: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 14, padding: 28 },
  stateTitle: { color: colors.text, fontSize: 19, fontWeight: '800', textAlign: 'center' },
  stateDescription: { color: colors.mutedText, fontSize: 14, lineHeight: 21, textAlign: 'center' },
  retryButton: { borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 22, paddingVertical: 12 },
  retryButtonText: { color: colors.text, fontSize: 14, fontWeight: '800' },
});
