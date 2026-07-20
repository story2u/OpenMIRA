import type {
  Dashboard,
  Opportunity,
} from '@story2u/radar-contracts/opportunities';
import { useRouter, type Href } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { memo, useCallback, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Modal,
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
import {
  cloneDashboardFilters,
  countAdvancedDashboardFilters,
  createDefaultDashboardFilters,
  dashboardSopStages,
  dashboardTrustLevels,
  normalizeDashboardKeywords,
  validateDashboardFilters,
  type DashboardFilters,
  type DashboardPlatformFilter,
  type DashboardSopStage,
  type DashboardStatusFilter,
} from './filters';
import { useDashboard } from './useDashboard';

interface ChoiceOption {
  label: string;
  value: string;
}

interface ChoiceDefinition {
  labelKey: MessageKey;
  value: string;
}

const statusDefinitions: readonly ChoiceDefinition[] = [
  { value: 'all', labelKey: 'dashboard.status.all' },
  { value: 'pending', labelKey: 'dashboard.pending' },
  { value: 'replied', labelKey: 'dashboard.status.replied' },
  { value: 'ignored', labelKey: 'dashboard.status.ignored' },
];
const platformDefinitions: readonly ChoiceDefinition[] = [
  { value: 'all', labelKey: 'dashboard.platform.all' },
  { value: 'telegram', labelKey: 'dashboard.platform.telegram' },
  { value: 'wecom', labelKey: 'dashboard.platform.wecom' },
];
const sourceDefinitions: readonly ChoiceDefinition[] = [
  { value: 'all', labelKey: 'dashboard.source.all' },
  { value: 'group', labelKey: 'dashboard.source.group' },
  { value: 'private', labelKey: 'dashboard.source.private' },
];
const timeDefinitions: readonly ChoiceDefinition[] = [
  { value: 'all', labelKey: 'dashboard.time.all' },
  { value: 'today', labelKey: 'dashboard.time.today' },
  { value: '3d', labelKey: 'dashboard.time.3d' },
  { value: '7d', labelKey: 'dashboard.time.7d' },
  { value: 'custom', labelKey: 'dashboard.time.custom' },
];
const sortDefinitions: readonly ChoiceDefinition[] = [
  { value: 'newest', labelKey: 'dashboard.sort.newest' },
  { value: 'oldest', labelKey: 'dashboard.sort.oldest' },
  { value: 'confidence', labelKey: 'dashboard.sort.confidence' },
  { value: 'trust', labelKey: 'dashboard.sort.trust' },
];
const trustDefinitions: readonly ChoiceDefinition[] = [
  { value: 'trusted', labelKey: 'dashboard.trust.trusted' },
  { value: 'unverified', labelKey: 'dashboard.trust.unverified' },
  { value: 'suspicious', labelKey: 'dashboard.trust.suspicious' },
  { value: 'risky', labelKey: 'dashboard.trust.risky' },
];
const sopLabelKeys: Record<DashboardSopStage, MessageKey> = {
  detected: 'dashboard.sop.detected',
  analyzing: 'dashboard.sop.analyzing',
  verified: 'dashboard.sop.verified',
  contact_extracted: 'dashboard.sop.contactExtracted',
  friend_requested: 'dashboard.sop.friendRequested',
  ready_to_chat: 'dashboard.sop.readyToChat',
  chatting: 'dashboard.sop.chatting',
  closed: 'dashboard.sop.closed',
};
const sopDefinitions: readonly ChoiceDefinition[] = dashboardSopStages.map((value) => ({
  value,
  labelKey: sopLabelKeys[value],
}));

const priorityLabelKeys: Record<Opportunity['priority'], MessageKey> = {
  low: 'dashboard.priority.low',
  normal: 'dashboard.priority.normal',
  high: 'dashboard.priority.high',
  urgent: 'dashboard.priority.urgent',
};
const statusLabelKeys: Record<Opportunity['status'], MessageKey> = {
  pending: 'dashboard.pending',
  replied: 'dashboard.status.replied',
  ignored: 'dashboard.status.ignored',
};

function localizeChoices(definitions: readonly ChoiceDefinition[], t: Translator) {
  return definitions.map(({ labelKey, value }) => ({ value, label: t(labelKey) }));
}

function ChoiceRow({
  label,
  onChange,
  options,
  value,
}: {
  label: string;
  onChange(value: string): void;
  options: readonly ChoiceOption[];
  value: string;
}) {
  return (
    <View style={styles.choiceSection}>
      <Text style={styles.filterLabel}>{label}</Text>
      <ScrollView
        horizontal
        contentContainerStyle={styles.choiceRow}
        showsHorizontalScrollIndicator={false}
      >
        {options.map((option) => {
          const selected = value === option.value;
          return (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected }}
              key={option.value}
              onPress={() => onChange(option.value)}
              style={({ pressed }) => [
                styles.choice,
                selected && styles.choiceSelected,
                pressed && styles.pressed,
              ]}
            >
              <Text style={[styles.choiceText, selected && styles.choiceTextSelected]}>
                {option.label}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

function MultiChoiceRow({
  label,
  onToggle,
  options,
  values,
}: {
  label: string;
  onToggle(value: string): void;
  options: readonly ChoiceOption[];
  values: readonly string[];
}) {
  const selectedValues = new Set(values);
  return (
    <View style={styles.choiceSection}>
      <Text style={styles.filterLabel}>{label}</Text>
      <View style={styles.wrappedChoices}>
        {options.map((option) => {
          const selected = selectedValues.has(option.value);
          return (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected }}
              key={option.value}
              onPress={() => onToggle(option.value)}
              style={({ pressed }) => [
                styles.choice,
                selected && styles.choiceSelected,
                pressed && styles.pressed,
              ]}
            >
              <Text style={[styles.choiceText, selected && styles.choiceTextSelected]}>
                {option.label}
              </Text>
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

function toggleArrayValue<Value extends string>(values: Value[], value: Value) {
  return values.includes(value)
    ? values.filter((entry) => entry !== value)
    : [...values, value];
}

function FilterModal({
  filters,
  keywordOptions,
  onApply,
  onClose,
  visible,
}: {
  filters: DashboardFilters;
  keywordOptions: readonly string[];
  onApply(filters: DashboardFilters): void;
  onClose(): void;
  visible: boolean;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState(() => cloneDashboardFilters(filters));
  const [keywords, setKeywords] = useState(filters.keywords.join('，'));
  const [error, setError] = useState<string | null>(null);
  const selectedKeywordValues = keywords
    .split(/[,，\n]/)
    .map((value) => value.trim())
    .filter(Boolean);
  const selectedKeywords = new Set(selectedKeywordValues);
  const sourceOptions = useMemo(() => localizeChoices(sourceDefinitions, t), [t]);
  const timeOptions = useMemo(() => localizeChoices(timeDefinitions, t), [t]);
  const trustOptions = useMemo(() => localizeChoices(trustDefinitions, t), [t]);
  const sopOptions = useMemo(() => localizeChoices(sopDefinitions, t), [t]);
  const sortOptions = useMemo(() => localizeChoices(sortDefinitions, t), [t]);

  function resetFromApplied() {
    setDraft(cloneDashboardFilters(filters));
    setKeywords(filters.keywords.join('，'));
    setError(null);
  }

  function close() {
    resetFromApplied();
    onClose();
  }

  function resetAdvanced() {
    const defaults = createDefaultDashboardFilters();
    setDraft((current) => ({
      ...defaults,
      status: current.status,
      platform: current.platform,
    }));
    setKeywords('');
    setError(null);
  }

  function apply() {
    try {
      const candidate = { ...draft, keywords: normalizeDashboardKeywords(keywords, t) };
      const validationError = validateDashboardFilters(candidate, t);
      if (validationError) {
        setError(validationError);
        return;
      }
      onApply(candidate);
      setError(null);
      onClose();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t('dashboard.filters.invalid'));
    }
  }

  return (
    <Modal
      animationType="slide"
      onRequestClose={close}
      presentationStyle="pageSheet"
      visible={visible}
    >
      <SafeAreaView style={styles.modalSafeArea}>
        <View style={styles.modalHeader}>
          <Pressable accessibilityRole="button" onPress={close} style={styles.headerAction}>
            <Text style={styles.headerActionText}>{t('common.cancel')}</Text>
          </Pressable>
          <Text accessibilityRole="header" style={styles.modalTitle}>{t('dashboard.filters.title')}</Text>
          <Pressable accessibilityRole="button" onPress={resetAdvanced} style={styles.headerAction}>
            <Text style={styles.headerActionText}>{t('common.reset')}</Text>
          </Pressable>
        </View>
        <ScrollView contentContainerStyle={styles.modalContent} keyboardShouldPersistTaps="handled">
          <ChoiceRow
            label={t('dashboard.filters.source')}
            onChange={(sourceType) => setDraft((current) => ({
              ...current,
              sourceType: sourceType as DashboardFilters['sourceType'],
            }))}
            options={sourceOptions}
            value={draft.sourceType}
          />
          <ChoiceRow
            label={t('dashboard.filters.time')}
            onChange={(timeRange) => setDraft((current) => ({
              ...current,
              timeRange: timeRange as DashboardFilters['timeRange'],
            }))}
            options={timeOptions}
            value={draft.timeRange}
          />
          {draft.timeRange === 'custom' ? (
            <View style={styles.dateRow}>
              <View style={styles.dateField}>
                <Text style={styles.filterLabel}>{t('dashboard.filters.startDate')}</Text>
                <TextInput
                  accessibilityLabel={t('dashboard.filters.startDate')}
                  autoCapitalize="none"
                  onChangeText={(customFrom) => setDraft((current) => ({ ...current, customFrom }))}
                  placeholder="YYYY-MM-DD"
                  placeholderTextColor={colors.placeholder}
                  style={styles.input}
                  value={draft.customFrom}
                />
              </View>
              <View style={styles.dateField}>
                <Text style={styles.filterLabel}>{t('dashboard.filters.endDate')}</Text>
                <TextInput
                  accessibilityLabel={t('dashboard.filters.endDate')}
                  autoCapitalize="none"
                  onChangeText={(customTo) => setDraft((current) => ({ ...current, customTo }))}
                  placeholder="YYYY-MM-DD"
                  placeholderTextColor={colors.placeholder}
                  style={styles.input}
                  value={draft.customTo}
                />
              </View>
            </View>
          ) : null}
          <MultiChoiceRow
            label={t('dashboard.filters.trust')}
            onToggle={(value) => setDraft((current) => ({
              ...current,
              trustLevels: toggleArrayValue(
                current.trustLevels,
                value as (typeof dashboardTrustLevels)[number],
              ),
            }))}
            options={trustOptions}
            values={draft.trustLevels}
          />
          <MultiChoiceRow
            label={t('dashboard.filters.sop')}
            onToggle={(value) => setDraft((current) => ({
              ...current,
              sopStages: toggleArrayValue(current.sopStages, value as DashboardSopStage),
            }))}
            options={sopOptions}
            values={draft.sopStages}
          />
          <View style={styles.choiceSection}>
            <Text style={styles.filterLabel}>{t('dashboard.filters.keywords')}</Text>
            {keywordOptions.length > 0 ? (
              <View style={styles.wrappedChoices}>
                {keywordOptions.slice(0, 24).map((keyword) => {
                  const selected = selectedKeywords.has(keyword);
                  return (
                    <Pressable
                      accessibilityRole="button"
                      accessibilityState={{ selected }}
                      key={keyword}
                      onPress={() => {
                        setKeywords(toggleArrayValue(selectedKeywordValues, keyword).join('，'));
                      }}
                      style={({ pressed }) => [
                        styles.choice,
                        selected && styles.choiceSelected,
                        pressed && styles.pressed,
                      ]}
                    >
                      <Text style={[styles.choiceText, selected && styles.choiceTextSelected]}>
                        {keyword}
                      </Text>
                    </Pressable>
                  );
                })}
              </View>
            ) : null}
            <TextInput
              accessibilityLabel={t('dashboard.filters.keywordsAccessibility')}
              autoCapitalize="none"
              multiline
              onChangeText={setKeywords}
              placeholder={t('dashboard.filters.keywordsPlaceholder')}
              placeholderTextColor={colors.placeholder}
              style={[styles.input, styles.keywordInput]}
              value={keywords}
            />
          </View>
          <ChoiceRow
            label={t('dashboard.filters.sort')}
            onChange={(sort) => setDraft((current) => ({
              ...current,
              sort: sort as DashboardFilters['sort'],
            }))}
            options={sortOptions}
            value={draft.sort}
          />
          <View accessibilityLiveRegion="polite" style={styles.modalErrorSlot}>
            {error ? <Text accessibilityRole="alert" style={styles.errorText}>{error}</Text> : null}
          </View>
          <Pressable
            accessibilityRole="button"
            onPress={apply}
            style={({ pressed }) => [styles.applyButton, pressed && styles.pressed]}
          >
            <Text style={styles.applyButtonText}>{t('dashboard.filters.apply')}</Text>
          </Pressable>
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}

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

const OpportunityCard = memo(function OpportunityCard({
  item,
  onOpen,
}: {
  item: Opportunity;
  onOpen(id: string): void;
}) {
  const { locale, t } = useI18n();
  const visibleKeywords = item.matchedKeywords.slice(0, 3);
  const status = t(statusLabelKeys[item.status]);
  return (
    <Pressable
      accessibilityLabel={t('dashboard.card.accessibility', {
        name: item.contactName,
        status,
        confidence: Math.round(item.confidenceScore * 100),
      })}
      accessibilityHint={t('dashboard.card.hint')}
      accessibilityRole="button"
      onPress={() => onOpen(item.id)}
      style={({ pressed }) => [
        styles.card,
        item.attentionRequired && styles.attentionCard,
        pressed && styles.pressed,
      ]}
    >
      <View style={styles.cardHeader}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>{item.contactName.trim().slice(0, 1) || '?'}</Text>
        </View>
        <View style={styles.cardHeading}>
          <Text numberOfLines={1} style={styles.contactName}>{item.contactName}</Text>
          <Text style={styles.cardMeta}>
            {item.platform === 'telegram' ? 'Telegram' : t('dashboard.platform.wecom')} · {t(
              item.sourceType === 'group' ? 'dashboard.source.group' : 'dashboard.source.private',
            )}
          </Text>
        </View>
        <View style={styles.statusBadge}>
          <Text style={styles.statusBadgeText}>{status}</Text>
        </View>
      </View>

      {item.attentionRequired ? (
        <Text style={styles.attentionText}>{t('dashboard.card.attention')}</Text>
      ) : null}
      <Text style={styles.summary}>{item.summary}</Text>
      {item.lastMessagePreview ? (
        <Text numberOfLines={2} style={styles.preview}>“{item.lastMessagePreview}”</Text>
      ) : null}
      {visibleKeywords.length > 0 ? (
        <View style={styles.keywordRow}>
          {visibleKeywords.map((keyword) => (
            <View key={keyword} style={styles.keywordBadge}>
              <Text style={styles.keywordText}>{keyword}</Text>
            </View>
          ))}
          {item.matchedKeywords.length > visibleKeywords.length ? (
            <Text style={styles.moreKeywords}>+{item.matchedKeywords.length - visibleKeywords.length}</Text>
          ) : null}
        </View>
      ) : null}
      <View style={styles.metricsRow}>
        <Text style={styles.metric}>{t('dashboard.card.confidence', { value: Math.round(item.confidenceScore * 100) })}</Text>
        <Text style={styles.metric}>{t('dashboard.card.trust', { value: item.trustScore })}</Text>
        <Text style={styles.metric}>{t('dashboard.card.priority', { value: t(priorityLabelKeys[item.priority]) })}</Text>
      </View>
      <View style={styles.cardFooter}>
        <Text style={styles.sopText}>{sopLabelKeys[item.sopStage as DashboardSopStage] ? t(sopLabelKeys[item.sopStage as DashboardSopStage]) : item.sopStage}</Text>
        <Text style={styles.cardTime}>{formatTimestamp(item.createdAt, locale, t)}</Text>
      </View>
    </Pressable>
  );
});

function AttentionBanner({ dashboard }: { dashboard: Dashboard }) {
  const { t } = useI18n();
  const attention = dashboard.attentionItems ?? [];
  if (attention.length === 0) return null;
  return (
    <View accessibilityRole="alert" style={styles.attentionBanner}>
      <Text style={styles.attentionTitle}>{t('dashboard.attention.title', { count: attention.length })}</Text>
      <Text style={styles.attentionDescription}>
        {attention.slice(0, 3).map((item) => item.contactName).join('、')}
        {attention.length > 3 ? t('dashboard.attention.more', { count: attention.length }) : ''}
        {t('dashboard.attention.approval')}
      </Text>
    </View>
  );
}

function EmptyState({
  error,
  loading,
  onRetry,
}: {
  error: string | null;
  loading: boolean;
  onRetry(): void;
}) {
  const { t } = useI18n();
  if (loading) {
    return (
      <View style={styles.emptyState}>
        <ActivityIndicator color={colors.accent} size="large" />
        <Text style={styles.emptyTitle}>{t('dashboard.loading')}</Text>
      </View>
    );
  }
  if (error) {
    return (
      <View style={styles.emptyState}>
        <Text accessibilityRole="header" style={styles.emptyTitle}>{t('dashboard.loadError.title')}</Text>
        <Text accessibilityRole="alert" style={styles.emptyDescription}>{error}</Text>
        <Pressable accessibilityRole="button" onPress={onRetry} style={styles.retryButton}>
          <Text style={styles.retryButtonText}>{t('common.retry')}</Text>
        </Pressable>
      </View>
    );
  }
  return (
    <View style={styles.emptyState}>
      <Text accessibilityRole="header" style={styles.emptyTitle}>{t('dashboard.empty.title')}</Text>
      <Text style={styles.emptyDescription}>{t('dashboard.empty.message')}</Text>
    </View>
  );
}

function Separator() {
  return <View style={styles.separator} />;
}

export default function DashboardScreen({ initialFilters }: { initialFilters?: DashboardFilters } = {}) {
  const router = useRouter();
  const {
    capabilities,
    commandSummary,
    dismissCommand,
    expireSession,
    state: sessionState,
    synchronize,
  } = useSession();
  const { t } = useI18n();
  const [filters, setFilters] = useState(() => (
    initialFilters ? cloneDashboardFilters(initialFilters) : createDefaultDashboardFilters()
  ));
  const [page, setPage] = useState(0);
  const [filtersVisible, setFiltersVisible] = useState(false);
  const ownerId = sessionState.status === 'authenticated' ? sessionState.user.id : '';
  const { retry, state } = useDashboard(
    filters,
    page,
    ownerId,
    capabilities.syncAvailable,
    expireSession,
    synchronize,
  );

  const dashboard = state.data;
  const activeAdvancedFilters = countAdvancedDashboardFilters(filters);
  const totalPages = dashboard ? Math.max(1, Math.ceil(dashboard.total / dashboard.limit)) : 1;
  const statusOptions = useMemo(() => localizeChoices(statusDefinitions, t), [t]);
  const platformOptions = useMemo(() => localizeChoices(platformDefinitions, t), [t]);

  function applyBasicFilter(patch: Partial<Pick<DashboardFilters, 'platform' | 'status'>>) {
    setPage(0);
    setFilters((current) => ({ ...current, ...patch }));
  }

  function applyAdvancedFilters(next: DashboardFilters) {
    setPage(0);
    setFilters(cloneDashboardFilters(next));
  }

  const openOpportunity = useCallback((id: string) => {
    router.push(`/opportunity/${id}` as Href);
  }, [router]);
  const renderOpportunity = useCallback(
    ({ item }: ListRenderItemInfo<Opportunity>) => (
      <OpportunityCard item={item} onOpen={openOpportunity} />
    ),
    [openOpportunity],
  );

  const header = (
    <View>
      <View style={styles.titleRow}>
        <View style={styles.titleCopy}>
          <Text style={styles.eyebrow}>OPPORTUNITY RADAR</Text>
          <Text accessibilityRole="header" style={styles.title}>{t('dashboard.title')}</Text>
          <Text style={styles.subtitle}>{t('dashboard.subtitle')}</Text>
        </View>
        <View style={styles.pendingBadge}>
          <Text style={styles.pendingCount}>{dashboard?.pendingCount ?? '—'}</Text>
          <Text style={styles.pendingLabel}>{t('dashboard.pending')}</Text>
        </View>
      </View>

      <Pressable
        accessibilityRole="button"
        onPress={() => router.push('/teaching' as Href)}
        style={({ pressed }) => [styles.appetiteCard, pressed && styles.pressed]}
      >
        <View style={styles.appetiteMark}><Text style={styles.appetiteGlyph}>✦</Text></View>
        <View style={styles.appetiteCopy}>
          <Text style={styles.appetiteEyebrow}>{t('dashboard.appetite.eyebrow')}</Text>
          <Text style={styles.appetiteTitle}>{t('dashboard.appetite.title')}</Text>
          <Text style={styles.appetiteDetail}>{t('dashboard.appetite.detail')}</Text>
        </View>
        <Text style={styles.appetiteAction}>{t('dashboard.appetite.action')} ›</Text>
      </Pressable>

      {dashboard ? <AttentionBanner dashboard={dashboard} /> : null}
      {commandSummary.pendingCount > 0 ? (
        <View accessibilityLiveRegion="polite" style={styles.commandNotice}>
          <Text style={styles.commandNoticeText}>
            {t('dashboard.commands.pending', { count: commandSummary.pendingCount })}
          </Text>
        </View>
      ) : null}
      {commandSummary.conflictCount > 0 || commandSummary.failedCount > 0 ? (
        <View style={styles.refreshError}>
          <View style={styles.commandAttentionContent}>
            <Text accessibilityRole="alert" style={styles.refreshErrorText}>
              {t('dashboard.commands.attention', {
                conflicts: commandSummary.conflictCount,
                failed: commandSummary.failedCount,
              })}
            </Text>
            {commandSummary.attentionCommands.map((command) => {
              const item = dashboard?.items.find(
                (opportunity) => opportunity.id === command.opportunityId,
              );
              return (
                <View key={command.id} style={styles.commandActionRow}>
                  <Text numberOfLines={1} style={styles.commandOpportunity}>
                    {item?.contactName ?? command.opportunityId.slice(0, 8)}
                  </Text>
                  <Pressable
                    accessibilityRole="button"
                    onPress={() => openOpportunity(command.opportunityId)}
                  >
                    <Text style={styles.inlineRetry}>{t('dashboard.commands.open')}</Text>
                  </Pressable>
                  <Pressable
                    accessibilityRole="button"
                    onPress={() => void dismissCommand(command.id)}
                  >
                    <Text style={styles.commandDismiss}>{t('dashboard.commands.dismiss')}</Text>
                  </Pressable>
                </View>
              );
            })}
          </View>
        </View>
      ) : null}
      {state.error && dashboard ? (
        <View accessibilityRole="alert" style={styles.refreshError}>
          <Text style={styles.refreshErrorText}>{state.error}</Text>
          <Pressable accessibilityRole="button" onPress={retry}>
            <Text style={styles.inlineRetry}>{t('common.retry')}</Text>
          </Pressable>
        </View>
      ) : null}

      <ChoiceRow
        label={t('dashboard.status')}
        onChange={(status) => applyBasicFilter({ status: status as DashboardStatusFilter })}
        options={statusOptions}
        value={filters.status}
      />
      <ChoiceRow
        label={t('dashboard.platform')}
        onChange={(platform) => applyBasicFilter({ platform: platform as DashboardPlatformFilter })}
        options={platformOptions}
        value={filters.platform}
      />
      <View style={styles.filterActionRow}>
        <Pressable
          accessibilityRole="button"
          onPress={() => setFiltersVisible(true)}
          style={({ pressed }) => [styles.filterButton, pressed && styles.pressed]}
        >
          <Text style={styles.filterButtonText}>
            {t('dashboard.filters.button')}{activeAdvancedFilters > 0 ? ` (${activeAdvancedFilters})` : ''}
          </Text>
        </Pressable>
        <Text accessibilityLiveRegion="polite" style={styles.resultCount}>
          {dashboard
            ? t('dashboard.resultCount', { total: dashboard.total, page: page + 1, pages: totalPages })
            : t('dashboard.readingResults')}
        </Text>
      </View>
    </View>
  );

  if (sessionState.status !== 'authenticated') return null;

  return (
    <SafeAreaView style={styles.safeArea}>
      <FlatList
        contentContainerStyle={styles.listContent}
        data={dashboard?.items ?? []}
        initialNumToRender={8}
        ItemSeparatorComponent={Separator}
        keyExtractor={(item) => item.id}
        ListEmptyComponent={<EmptyState error={state.error} loading={state.loading} onRetry={retry} />}
        ListFooterComponent={dashboard && dashboard.total > 0 ? (
          <View style={styles.pagination}>
            <Pressable
              accessibilityLabel={t('dashboard.previousPage')}
              accessibilityRole="button"
              accessibilityState={{ disabled: page === 0 }}
              disabled={page === 0}
              onPress={() => setPage((current) => Math.max(0, current - 1))}
              style={[styles.pageButton, page === 0 && styles.disabled]}
            >
              <Text style={styles.pageButtonText}>{t('dashboard.previousPage')}</Text>
            </Pressable>
            <Text style={styles.pageIndicator}>{page + 1} / {totalPages}</Text>
            <Pressable
              accessibilityLabel={t('dashboard.nextPage')}
              accessibilityRole="button"
              accessibilityState={{ disabled: page + 1 >= totalPages }}
              disabled={page + 1 >= totalPages}
              onPress={() => setPage((current) => Math.min(totalPages - 1, current + 1))}
              style={[styles.pageButton, page + 1 >= totalPages && styles.disabled]}
            >
              <Text style={styles.pageButtonText}>{t('dashboard.nextPage')}</Text>
            </Pressable>
          </View>
        ) : null}
        ListHeaderComponent={header}
        onRefresh={retry}
        refreshing={state.refreshing}
        renderItem={renderOpportunity}
        showsVerticalScrollIndicator={false}
        windowSize={7}
      />
      <FilterModal
        filters={filters}
        key={JSON.stringify(filters)}
        keywordOptions={dashboard?.keywordOptions ?? []}
        onApply={applyAdvancedFilters}
        onClose={() => setFiltersVisible(false)}
        visible={filtersVisible}
      />
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  listContent: { flexGrow: 1, paddingHorizontal: 18, paddingBottom: 28 },
  titleRow: { flexDirection: 'row', alignItems: 'flex-end', gap: 16, paddingTop: 18, paddingBottom: 20 },
  titleCopy: { flex: 1 },
  eyebrow: { color: colors.accent, fontSize: 11, fontWeight: '800', letterSpacing: 1.8 },
  title: { marginTop: 6, color: colors.text, fontSize: 30, fontWeight: '800' },
  subtitle: { marginTop: 6, color: colors.mutedText, fontSize: 13, lineHeight: 19 },
  pendingBadge: { minWidth: 70, alignItems: 'center', borderRadius: 16, backgroundColor: colors.card, padding: 10 },
  pendingCount: { color: colors.warning, fontSize: 22, fontWeight: '900' },
  pendingLabel: { marginTop: 2, color: colors.mutedText, fontSize: 11 },
  appetiteCard: {
    flexDirection: 'row', alignItems: 'center', gap: 12, marginBottom: 16,
    borderRadius: 20, borderWidth: 1, borderColor: '#245b5e',
    backgroundColor: '#0d2a32', padding: 15,
  },
  appetiteMark: { width: 42, height: 42, alignItems: 'center', justifyContent: 'center', borderRadius: 15, backgroundColor: colors.accentMuted },
  appetiteGlyph: { color: colors.accent, fontSize: 19 },
  appetiteCopy: { flex: 1, gap: 3 },
  appetiteEyebrow: { color: colors.accent, fontSize: 9, fontWeight: '900', letterSpacing: 1.1 },
  appetiteTitle: { color: colors.text, fontSize: 15, fontWeight: '900' },
  appetiteDetail: { color: colors.mutedText, fontSize: 11, lineHeight: 16 },
  appetiteAction: { maxWidth: 62, color: colors.accent, fontSize: 11, fontWeight: '900', textAlign: 'right' },
  attentionBanner: { marginBottom: 18, borderWidth: 1, borderColor: colors.warning, borderRadius: 14, backgroundColor: colors.noticeBackground, padding: 14 },
  attentionTitle: { color: colors.noticeText, fontSize: 14, fontWeight: '800' },
  attentionDescription: { marginTop: 5, color: colors.noticeText, fontSize: 12, lineHeight: 18 },
  refreshError: { marginBottom: 14, flexDirection: 'row', alignItems: 'center', gap: 10, borderRadius: 12, backgroundColor: colors.errorBackground, padding: 12 },
  refreshErrorText: { flex: 1, color: colors.errorText, fontSize: 13, lineHeight: 18 },
  commandNotice: { marginBottom: 14, borderRadius: 12, backgroundColor: colors.noticeBackground, padding: 12 },
  commandNoticeText: { color: colors.noticeText, fontSize: 13, lineHeight: 18 },
  commandAttentionContent: { flex: 1, gap: 8 },
  commandActionRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  commandOpportunity: { flex: 1, color: colors.errorText, fontSize: 12, fontWeight: '700' },
  commandDismiss: { color: colors.mutedText, fontSize: 12, fontWeight: '700' },
  inlineRetry: { color: colors.text, fontSize: 13, fontWeight: '800' },
  choiceSection: { marginBottom: 14, gap: 8 },
  filterLabel: { color: colors.mutedText, fontSize: 12, fontWeight: '700' },
  choiceRow: { gap: 8, paddingRight: 16 },
  wrappedChoices: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  choice: { minHeight: 36, justifyContent: 'center', borderWidth: 1, borderColor: colors.border, borderRadius: 18, backgroundColor: colors.card, paddingHorizontal: 14, paddingVertical: 8 },
  choiceSelected: { borderColor: colors.accent, backgroundColor: colors.accentMuted },
  choiceText: { color: colors.mutedText, fontSize: 13, fontWeight: '700' },
  choiceTextSelected: { color: colors.text },
  pressed: { opacity: 0.72 },
  filterActionRow: { marginTop: 2, marginBottom: 18, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12 },
  filterButton: { borderRadius: 10, backgroundColor: colors.button, paddingHorizontal: 14, paddingVertical: 10 },
  filterButtonText: { color: colors.text, fontSize: 13, fontWeight: '800' },
  resultCount: { flex: 1, color: colors.mutedText, fontSize: 12, textAlign: 'right' },
  separator: { height: 12 },
  card: { borderWidth: 1, borderColor: colors.border, borderRadius: 18, backgroundColor: colors.card, padding: 16 },
  attentionCard: { borderColor: colors.warning },
  cardHeader: { flexDirection: 'row', alignItems: 'center', gap: 11 },
  avatar: { width: 42, height: 42, alignItems: 'center', justifyContent: 'center', borderRadius: 21, backgroundColor: colors.accentMuted },
  avatarText: { color: colors.accent, fontSize: 18, fontWeight: '900' },
  cardHeading: { flex: 1 },
  contactName: { color: colors.text, fontSize: 16, fontWeight: '800' },
  cardMeta: { marginTop: 3, color: colors.mutedText, fontSize: 11 },
  statusBadge: { borderRadius: 10, backgroundColor: colors.accentMuted, paddingHorizontal: 9, paddingVertical: 5 },
  statusBadgeText: { color: colors.accent, fontSize: 11, fontWeight: '800' },
  attentionText: { marginTop: 13, color: colors.warning, fontSize: 12, fontWeight: '800' },
  summary: { marginTop: 13, color: colors.text, fontSize: 15, fontWeight: '700', lineHeight: 22 },
  preview: { marginTop: 8, color: colors.mutedText, fontSize: 13, lineHeight: 20 },
  keywordRow: { marginTop: 12, flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: 6 },
  keywordBadge: { borderRadius: 8, backgroundColor: colors.accentMuted, paddingHorizontal: 8, paddingVertical: 4 },
  keywordText: { color: colors.accent, fontSize: 11, fontWeight: '700' },
  moreKeywords: { color: colors.mutedText, fontSize: 11 },
  metricsRow: { marginTop: 14, flexDirection: 'row', flexWrap: 'wrap', gap: 12 },
  metric: { color: colors.mutedText, fontSize: 11, fontWeight: '600' },
  cardFooter: { marginTop: 14, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 10 },
  sopText: { flex: 1, color: colors.success, fontSize: 11, fontWeight: '700' },
  cardTime: { color: colors.subtleText, fontSize: 11 },
  emptyState: { minHeight: 280, alignItems: 'center', justifyContent: 'center', gap: 12, paddingHorizontal: 28 },
  emptyTitle: { color: colors.text, fontSize: 18, fontWeight: '800', textAlign: 'center' },
  emptyDescription: { color: colors.mutedText, fontSize: 14, lineHeight: 21, textAlign: 'center' },
  retryButton: { marginTop: 8, borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 22, paddingVertical: 12 },
  retryButtonText: { color: colors.text, fontSize: 14, fontWeight: '800' },
  pagination: { flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 14, paddingTop: 24 },
  pageButton: { minWidth: 88, alignItems: 'center', borderWidth: 1, borderColor: colors.border, borderRadius: 11, paddingHorizontal: 14, paddingVertical: 11 },
  pageButtonText: { color: colors.text, fontSize: 13, fontWeight: '800' },
  pageIndicator: { color: colors.mutedText, fontSize: 13, fontWeight: '700' },
  disabled: { opacity: 0.35 },
  modalSafeArea: { flex: 1, backgroundColor: colors.background },
  modalHeader: { minHeight: 56, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', borderBottomWidth: 1, borderBottomColor: colors.border, paddingHorizontal: 12 },
  modalTitle: { color: colors.text, fontSize: 17, fontWeight: '800' },
  headerAction: { minWidth: 58, minHeight: 42, justifyContent: 'center', paddingHorizontal: 6 },
  headerActionText: { color: colors.accent, fontSize: 14, fontWeight: '700' },
  modalContent: { padding: 20, paddingBottom: 36 },
  dateRow: { flexDirection: 'row', gap: 10, marginBottom: 16 },
  dateField: { flex: 1, gap: 8 },
  input: { minHeight: 46, borderWidth: 1, borderColor: colors.border, borderRadius: 11, backgroundColor: colors.card, color: colors.text, fontSize: 14, paddingHorizontal: 12, paddingVertical: 11 },
  keywordInput: { minHeight: 78, textAlignVertical: 'top' },
  modalErrorSlot: { minHeight: 44, justifyContent: 'center' },
  errorText: { color: colors.errorText, fontSize: 13, lineHeight: 19 },
  applyButton: { minHeight: 50, alignItems: 'center', justifyContent: 'center', borderRadius: 13, backgroundColor: colors.button, padding: 13 },
  applyButtonText: { color: colors.text, fontSize: 15, fontWeight: '900' },
});
