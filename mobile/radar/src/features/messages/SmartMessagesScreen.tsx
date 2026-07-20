import type { AttentionSnapshot } from '@story2u/radar-core/briefing/model';
import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { getAttentionSnapshot } from '../../briefing/briefingService';
import { useI18n } from '../../i18n/I18nProvider';
import { initializeRadarDatabase } from '../../storage/database';
import { colors } from '../../ui/theme';
import { createDefaultDashboardFilters } from '../dashboard/filters';
import { useDashboard } from '../dashboard/useDashboard';
import {
  buildSmartMessageSections,
  type SmartMessageSection,
  type SmartMessageTarget,
} from './smartViews';

const READ_ONLY_DEVICE_ID = '00000000-0000-4000-8000-000000000000';

function targetHref(target: SmartMessageTarget): Href {
  if (target === 'quiet') return '/quiet-zone' as Href;
  if (target === 'judgment') return '/teaching' as Href;
  if (target === 'digest') {
    const prompt = 'Summarize my digest items for today using only local data, then ask before changing any preference.';
    return `/(tabs)/agent?prompt=${encodeURIComponent(prompt)}` as Href;
  }
  return `/(tabs)/dashboard?view=list&category=${target}` as Href;
}

function SmartCard({
  onPress,
  section,
}: {
  onPress(target: SmartMessageTarget): void;
  section: SmartMessageSection;
}) {
  const { t } = useI18n();
  return (
    <Pressable
      accessibilityRole="button"
      onPress={() => onPress(section.target)}
      style={({ pressed }) => [
        styles.smartCard,
        section.tone === 'action' && styles.actionCard,
        section.tone === 'focus' && styles.focusCard,
        section.tone === 'muted' && styles.mutedCard,
        pressed && styles.pressed,
      ]}
    >
      <View style={styles.smartCardHeader}>
        <Text style={styles.smartCardTitle}>{t(section.titleKey)}</Text>
        <Text style={styles.smartCardCount}>{section.count}</Text>
      </View>
      <Text style={styles.smartCardDetail}>{t(section.detailKey)}</Text>
      <Text style={styles.smartCardAction}>{t('messages.smart.open')} ›</Text>
    </Pressable>
  );
}

export default function SmartMessagesScreen() {
  const router = useRouter();
  const {
    capabilities,
    commandSummary,
    expireSession,
    state: sessionState,
    synchronize,
  } = useSession();
  const { t } = useI18n();
  const ownerId = sessionState.status === 'authenticated' ? sessionState.user.id : '';
  const defaultFilters = useMemo(() => createDefaultDashboardFilters(), []);
  const { retry, state } = useDashboard(
    defaultFilters,
    0,
    ownerId,
    capabilities.syncAvailable,
    expireSession,
    synchronize,
  );
  const [snapshot, setSnapshot] = useState<AttentionSnapshot | null>(null);

  useEffect(() => {
    if (sessionState.status !== 'authenticated') return;
    let active = true;
    void (async () => {
      const database = await initializeRadarDatabase();
      const next = await getAttentionSnapshot(database, {
        ownerId: sessionState.user.id,
        deviceId: READ_ONLY_DEVICE_ID,
      });
      if (active) setSnapshot(next);
    })().catch(() => {
      if (active) setSnapshot(null);
    });
    return () => {
      active = false;
    };
  }, [sessionState]);

  if (sessionState.status !== 'authenticated') return null;

  const dashboard = state.data;
  const sections = buildSmartMessageSections(dashboard, snapshot);
  const firstNames = dashboard?.attentionItems?.slice(0, 3).map((item) => item.contactName).join('、') ?? '';
  const processed = snapshot?.totalProcessed ?? 0;
  const openTarget = (target: SmartMessageTarget) => router.push(targetHref(target));

  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container} showsVerticalScrollIndicator={false}>
        <View style={styles.titleRow}>
          <View style={styles.titleCopy}>
            <Text style={styles.eyebrow}>MIRA MESSAGE CENTER</Text>
            <Text accessibilityRole="header" style={styles.title}>{t('messages.smart.title')}</Text>
            <Text style={styles.subtitle}>
              {processed > 0
                ? t('messages.smart.subtitleWithSnapshot', { count: processed })
                : t('messages.smart.subtitle')}
            </Text>
          </View>
          <View style={styles.statusBadge}>
            <Text style={styles.statusBadgeValue}>{dashboard?.pendingCount ?? '—'}</Text>
            <Text style={styles.statusBadgeLabel}>{t('dashboard.pending')}</Text>
          </View>
        </View>

        {commandSummary.pendingCount > 0 ? (
          <View accessibilityLiveRegion="polite" style={styles.notice}>
            <Text style={styles.noticeText}>{t('dashboard.commands.pending', { count: commandSummary.pendingCount })}</Text>
          </View>
        ) : null}

        {state.loading && !dashboard ? (
          <View style={styles.loadingState}>
            <ActivityIndicator color={colors.accent} />
            <Text style={styles.loadingText}>{t('messages.smart.loading')}</Text>
          </View>
        ) : null}

        {state.error && !dashboard ? (
          <View accessibilityRole="alert" style={styles.errorState}>
            <Text style={styles.errorTitle}>{t('dashboard.loadError.title')}</Text>
            <Text style={styles.errorText}>{state.error}</Text>
            <Pressable accessibilityRole="button" onPress={retry} style={styles.retryButton}>
              <Text style={styles.retryText}>{t('common.retry')}</Text>
            </Pressable>
          </View>
        ) : null}

        {dashboard ? (
          <View style={styles.briefCard}>
            <Text style={styles.briefTitle}>{t('messages.smart.brief.title')}</Text>
            <Text style={styles.briefText}>
              {firstNames
                ? t('messages.smart.brief.attention', { names: firstNames })
                : t('messages.smart.brief.empty')}
            </Text>
          </View>
        ) : null}

        <View style={styles.grid}>
          {sections.map((section) => (
            <SmartCard key={section.id} onPress={openTarget} section={section} />
          ))}
        </View>
      </ScrollView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { gap: 16, padding: 18, paddingBottom: 34 },
  titleRow: { flexDirection: 'row', alignItems: 'flex-end', gap: 14, paddingTop: 14 },
  titleCopy: { flex: 1 },
  eyebrow: { color: colors.accent, fontSize: 10, fontWeight: '900', letterSpacing: 1.2 },
  title: { marginTop: 6, color: colors.text, fontSize: 28, fontWeight: '900', lineHeight: 34 },
  subtitle: { marginTop: 8, color: colors.mutedText, fontSize: 13, lineHeight: 19 },
  statusBadge: { minWidth: 72, alignItems: 'center', borderRadius: 14, backgroundColor: colors.card, padding: 10 },
  statusBadgeValue: { color: colors.warning, fontSize: 23, fontWeight: '900', fontVariant: ['tabular-nums'] },
  statusBadgeLabel: { color: colors.mutedText, fontSize: 11 },
  notice: { borderRadius: 12, backgroundColor: colors.noticeBackground, padding: 12 },
  noticeText: { color: colors.noticeText, fontSize: 12, lineHeight: 18 },
  loadingState: { minHeight: 120, alignItems: 'center', justifyContent: 'center', gap: 10 },
  loadingText: { color: colors.mutedText, fontSize: 13 },
  errorState: { gap: 10, borderRadius: 14, backgroundColor: colors.errorBackground, padding: 14 },
  errorTitle: { color: colors.errorText, fontSize: 15, fontWeight: '900' },
  errorText: { color: colors.errorText, fontSize: 12, lineHeight: 18 },
  retryButton: { alignSelf: 'flex-start', borderRadius: 10, backgroundColor: colors.button, paddingHorizontal: 14, paddingVertical: 9 },
  retryText: { color: colors.text, fontSize: 12, fontWeight: '800' },
  briefCard: { gap: 6, borderWidth: 1, borderColor: colors.border, borderRadius: 16, backgroundColor: colors.card, padding: 15 },
  briefTitle: { color: colors.text, fontSize: 15, fontWeight: '900' },
  briefText: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  smartCard: { width: '48%', flexGrow: 1, minHeight: 132, justifyContent: 'space-between', borderWidth: 1, borderColor: colors.border, borderRadius: 14, backgroundColor: colors.card, padding: 14 },
  actionCard: { borderColor: colors.warning, backgroundColor: '#172434' },
  focusCard: { borderColor: colors.accent, backgroundColor: '#0b2731' },
  mutedCard: { opacity: 0.88 },
  pressed: { opacity: 0.72 },
  smartCardHeader: { flexDirection: 'row', alignItems: 'flex-start', gap: 8 },
  smartCardTitle: { flex: 1, color: colors.text, fontSize: 14, fontWeight: '900', lineHeight: 18 },
  smartCardCount: { color: colors.accent, fontSize: 24, fontWeight: '900', fontVariant: ['tabular-nums'] },
  smartCardDetail: { color: colors.mutedText, fontSize: 11, lineHeight: 16 },
  smartCardAction: { color: colors.accent, fontSize: 12, fontWeight: '900' },
});
