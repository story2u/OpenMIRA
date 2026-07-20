import type {
  AttentionSnapshot,
  Briefing,
  BriefingType,
} from '@story2u/radar-core/briefing/model';
import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ActivityIndicator,
  Animated,
  Easing,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { getAttentionSnapshot, listBriefings } from '../../briefing/briefingService';
import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { initializeRadarDatabase } from '../../storage/database';
import { colors } from '../../ui/theme';
import { useReducedMotion } from '../../ui/useReducedMotion';
import { IntentMapCanvas } from '../intent-map/IntentMapCanvas';
import { useIntentMap } from '../intent-map/useIntentMap';

const READ_ONLY_DEVICE_ID = '00000000-0000-4000-8000-000000000000';
const briefingTitleKeys: Record<BriefingType, MessageKey> = {
  morning: 'briefing.title.morning',
  midday: 'briefing.title.midday',
  evening: 'briefing.title.evening',
  ad_hoc: 'briefing.title.adHoc',
  urgent: 'briefing.title.urgent',
};
const defaultBriefings: readonly { type: BriefingType; time: string }[] = [
  { type: 'morning', time: '08:30' },
  { type: 'midday', time: '12:00' },
  { type: 'evening', time: '18:30' },
];

interface BriefingHomeState {
  briefings: Briefing[];
  loading: boolean;
  snapshot: AttentionSnapshot | null;
}

function formatClock(iso: string | null | undefined) {
  if (!iso) return '--:--';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '--:--';
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
}

function MiraOrb() {
  const reducedMotion = useReducedMotion();
  const pulse = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    if (reducedMotion) {
      pulse.stopAnimation();
      pulse.setValue(0);
      return;
    }
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: 1700,
          easing: Easing.out(Easing.quad),
          toValue: 1,
          useNativeDriver: true,
        }),
        Animated.timing(pulse, {
          duration: 1700,
          easing: Easing.in(Easing.quad),
          toValue: 0,
          useNativeDriver: true,
        }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [pulse, reducedMotion]);

  const scale = pulse.interpolate({ inputRange: [0, 1], outputRange: [0.9, 1.08] });
  const opacity = pulse.interpolate({ inputRange: [0, 1], outputRange: [0.35, 0.72] });
  return (
    <View accessibilityElementsHidden importantForAccessibility="no-hide-descendants" style={styles.orb}>
      <Animated.View style={[styles.orbRing, { opacity, transform: [{ scale }] }]} />
      <View style={styles.orbCore} />
    </View>
  );
}

export default function HomeScreen() {
  const router = useRouter();
  const { state } = useSession();
  const { t } = useI18n();
  const { loading: mapLoading, model } = useIntentMap();
  const [briefingState, setBriefingState] = useState<BriefingHomeState>({
    briefings: [],
    loading: true,
    snapshot: null,
  });

  useEffect(() => {
    if (state.status !== 'authenticated') return;
    let active = true;
    setBriefingState((current) => ({ ...current, loading: true }));
    void (async () => {
      const database = await initializeRadarDatabase();
      const context = {
        ownerId: state.user.id,
        deviceId: READ_ONLY_DEVICE_ID,
      };
      const [snapshot, briefings] = await Promise.all([
        getAttentionSnapshot(database, context),
        listBriefings(database, context, { limit: 6 }),
      ]);
      if (active) setBriefingState({ briefings, loading: false, snapshot });
    })().catch(() => {
      if (active) setBriefingState({ briefings: [], loading: false, snapshot: null });
    });
    return () => {
      active = false;
    };
  }, [state]);

  const latestBriefing = useMemo(
    () => briefingState.briefings.find((briefing) => briefing.status === 'ready') ?? null,
    [briefingState.briefings],
  );
  const snapshot = briefingState.snapshot;
  const immediateCount = snapshot?.immediateCount ?? model?.stats.immediate ?? 0;
  const inboxCount = snapshot?.inboxCount ?? model?.stats.inbox ?? 0;
  const digestCount = snapshot?.digestCount ?? model?.stats.digest ?? 0;
  const suppressedCount = snapshot?.suppressedCount ?? model?.stats.suppress ?? 0;
  const needsUserInputCount = snapshot?.needsUserInputCount ?? 0;
  const totalProcessed = snapshot?.totalProcessed ?? model?.stats.total ?? 0;
  const localProcessed = snapshot?.localProcessed ?? totalProcessed;
  const deepAnalyzed = snapshot?.deepAnalyzed ?? 0;
  const focusCount = immediateCount + inboxCount + needsUserInputCount;
  const statusTime = formatClock(latestBriefing?.generatedAt ?? snapshot?.generatedAt);
  const oneLineSummary = latestBriefing?.summary
    ?? (totalProcessed > 0
      ? t('home.summary.generated', {
        digest: digestCount,
        immediate: immediateCount,
        suppressed: suppressedCount,
      })
      : t('home.summary.empty'));

  if (state.status !== 'authenticated') return null;
  const avatarLetter = (state.user.displayName || state.user.email || 'M').trim().slice(0, 1).toUpperCase();
  const briefingCards = briefingState.briefings.length > 0
    ? briefingState.briefings.slice(0, 3).map((briefing) => ({
      briefing,
      key: briefing.id,
      time: formatClock(briefing.generatedAt),
      type: briefing.type,
    }))
    : defaultBriefings.map((entry) => ({
      briefing: null,
      key: entry.type,
      time: entry.time,
      type: entry.type,
    }));

  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container} showsVerticalScrollIndicator={false}>
        <View style={styles.statusHeader}>
          <View style={styles.statusCopy}>
            <Text style={styles.greeting}>{t('home.greeting', { name: state.user.displayName })}</Text>
            <Text accessibilityRole="header" style={styles.statusTitle}>
              {statusTime === '--:--'
                ? t('home.mira.organizing')
                : t('home.mira.organizedAt', { time: statusTime })}
            </Text>
            <Text style={styles.statusMeta}>
              {t('home.mira.processing', {
                cloud: deepAnalyzed,
                local: localProcessed,
              })}
            </Text>
          </View>
          <View style={styles.statusActions}>
            <MiraOrb />
            <Pressable
              accessibilityLabel={t('home.settings.accessibility')}
              accessibilityRole="button"
              onPress={() => router.push('/account' as Href)}
              style={({ pressed }) => [styles.avatarButton, pressed && styles.pressed]}
            >
              <Text style={styles.avatarText}>{avatarLetter}</Text>
            </Pressable>
          </View>
        </View>

        <View style={styles.heroCard}>
          <Text style={styles.heroEyebrow}>OpenMIRA</Text>
          <Text style={styles.heroTitle}>{t('home.hero.title', { count: focusCount })}</Text>
          <View style={styles.heroStats}>
            <View style={styles.heroStat}>
              <Text style={styles.heroStatCount}>{immediateCount}</Text>
              <Text style={styles.heroStatLabel}>{t('home.hero.immediate')}</Text>
            </View>
            <View style={styles.heroStat}>
              <Text style={styles.heroStatCount}>{inboxCount}</Text>
              <Text style={styles.heroStatLabel}>{t('home.hero.worthAttention')}</Text>
            </View>
            <View style={styles.heroStat}>
              <Text style={styles.heroStatCount}>{needsUserInputCount}</Text>
              <Text style={styles.heroStatLabel}>{t('home.hero.needsJudgment')}</Text>
            </View>
          </View>
          <View style={styles.heroActions}>
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push('/dashboard' as Href)}
              style={({ pressed }) => [styles.primaryButton, pressed && styles.pressed]}
            >
              <Text style={styles.primaryButtonText}>{t('home.hero.start')}</Text>
            </Pressable>
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push('/agent' as Href)}
              style={({ pressed }) => [styles.secondaryButton, pressed && styles.pressed]}
            >
              <Text style={styles.secondaryButtonText}>{t('home.hero.why')}</Text>
            </Pressable>
          </View>
        </View>

        <View style={styles.summaryCard}>
          <Text style={styles.sectionEyebrow}>{t('home.summary.eyebrow')}</Text>
          <Text style={styles.summaryText}>{oneLineSummary}</Text>
        </View>

        <View style={styles.briefingSection}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>{t('home.briefings.title')}</Text>
            {briefingState.loading ? <ActivityIndicator color={colors.accent} /> : null}
          </View>
          {briefingCards.map(({ briefing, key, time, type }) => {
            return (
              <View key={key} style={styles.briefingCard}>
                <View style={styles.briefingHeader}>
                  <Text style={styles.briefingTitle}>{t(briefingTitleKeys[type])} · {time}</Text>
                  <Text style={styles.briefingState}>
                    {briefing ? t('home.briefings.ready') : t('home.briefings.pending')}
                  </Text>
                </View>
                <Text style={styles.briefingBody}>
                  {briefing
                    ? t('home.briefings.body', {
                      immediate: briefing.immediateCount,
                      suppressed: briefing.suppressedCount,
                      total: briefing.totalMessages,
                    })
                    : t('home.briefings.pendingBody')}
                </Text>
              </View>
            );
          })}
        </View>

        <View style={styles.mapCard}>
          <View style={styles.mapHeader}>
            <View style={styles.mapHeaderCopy}>
              <Text style={styles.sectionEyebrow}>{t('intentMap.title')}</Text>
              <Text style={styles.appetiteName}>{model?.preference?.title ?? t('home.appetite.empty')}</Text>
            </View>
            <Pressable accessibilityRole="button" onPress={() => router.push('/intent-map' as Href)}>
              <Text style={styles.mapLink}>{t('home.appetite.openMap')} ›</Text>
            </Pressable>
          </View>
          {mapLoading ? <ActivityIndicator color={colors.accent} style={styles.mapLoading} /> : null}
          {!mapLoading && model?.preference ? (
            <IntentMapCanvas compact model={model} onNodePress={() => router.push('/intent-map' as Href)} t={t} />
          ) : null}
          {!mapLoading && !model?.preference ? (
            <View style={styles.emptyMap}>
              <Text style={styles.emptyGlyph}>✦</Text>
              <Text style={styles.emptyDetail}>{t('home.appetite.emptyDetail')}</Text>
            </View>
          ) : null}
          <Pressable accessibilityRole="button" onPress={() => router.push('/teaching' as Href)} style={styles.teachButton}>
            <Text style={styles.teachText}>{t('home.appetite.teach')}</Text>
          </Pressable>
          {model?.shadow ? (
            <Pressable accessibilityRole="button" onPress={() => router.push('/intent-map' as Href)} style={styles.shadowBanner}>
              <Text style={styles.shadowBannerText}>{t('home.shadow.running')}</Text><Text style={styles.shadowBannerArrow}>›</Text>
            </Pressable>
          ) : null}
        </View>

        <View style={styles.flowCard}>
          <Text style={styles.flowTitle}>{t('home.flow.title')}</Text>
          <Text style={styles.flowTotal}>{t('home.flow.total', { count: totalProcessed })}</Text>
          <View style={styles.statsGrid}>
            {([
              ['immediate', immediateCount, '#fbbf24'],
              ['inbox', inboxCount, '#38bdf8'],
              ['digest', digestCount, '#818cf8'],
              ['suppress', suppressedCount, '#94a3b8'],
            ] as const).map(([key, count, color]) => (
              <View key={key} style={styles.stat}>
                <Text style={[styles.statCount, { color }]}>{count}</Text>
                <Text style={styles.statLabel}>{t(`home.flow.${key}`)}</Text>
              </View>
            ))}
          </View>
          <Pressable accessibilityRole="button" onPress={() => router.push('/quiet-zone' as Href)} style={styles.quietZoneButton}>
            <View><Text style={styles.quietZoneTitle}>{t('home.quietZone.title')}</Text><Text style={styles.quietZoneDetail}>{t('home.quietZone.detail')}</Text></View>
            <Text style={styles.quietZoneArrow}>›</Text>
          </Pressable>
        </View>
      </ScrollView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { gap: 16, padding: 20, paddingBottom: 38 },
  statusHeader: { flexDirection: 'row', alignItems: 'center', gap: 14, paddingTop: 10 },
  statusCopy: { flex: 1, gap: 4 },
  greeting: { color: colors.accent, fontSize: 12, fontWeight: '800' },
  statusTitle: { color: colors.text, fontSize: 21, fontWeight: '900' },
  statusMeta: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  statusActions: { flexDirection: 'row', alignItems: 'center', gap: 11 },
  orb: { width: 34, height: 34, alignItems: 'center', justifyContent: 'center' },
  orbRing: { position: 'absolute', width: 34, height: 34, borderRadius: 17, borderWidth: 1, borderColor: '#818cf8', backgroundColor: '#312e81' },
  orbCore: { width: 14, height: 14, borderRadius: 7, backgroundColor: '#38bdf8', shadowColor: '#38bdf8', shadowOpacity: 0.4, shadowRadius: 9 },
  avatarButton: { width: 38, height: 38, alignItems: 'center', justifyContent: 'center', borderRadius: 19, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card },
  avatarText: { color: colors.text, fontSize: 14, fontWeight: '900' },
  pressed: { opacity: 0.72 },
  heroCard: { gap: 15, borderRadius: 26, borderWidth: 1, borderColor: '#234a66', backgroundColor: '#0b2238', padding: 18 },
  heroEyebrow: { color: '#7dd3fc', fontSize: 11, fontWeight: '900' },
  heroTitle: { maxWidth: 330, color: colors.text, fontSize: 30, lineHeight: 37, fontWeight: '900' },
  heroStats: { flexDirection: 'row', gap: 9 },
  heroStat: { flex: 1, minHeight: 74, justifyContent: 'center', borderRadius: 18, backgroundColor: '#102b45', padding: 12 },
  heroStatCount: { color: colors.text, fontSize: 23, fontWeight: '900', fontVariant: ['tabular-nums'] },
  heroStatLabel: { marginTop: 3, color: colors.mutedText, fontSize: 11, lineHeight: 15 },
  heroActions: { flexDirection: 'row', gap: 10 },
  primaryButton: { flex: 1, minHeight: 48, alignItems: 'center', justifyContent: 'center', borderRadius: 16, backgroundColor: '#0ea5e9', paddingHorizontal: 14 },
  primaryButtonText: { color: '#02111c', fontSize: 14, fontWeight: '900' },
  secondaryButton: { flex: 1, minHeight: 48, alignItems: 'center', justifyContent: 'center', borderRadius: 16, borderWidth: 1, borderColor: colors.border, paddingHorizontal: 14 },
  secondaryButtonText: { color: colors.text, fontSize: 14, fontWeight: '800' },
  summaryCard: { gap: 8, borderRadius: 22, backgroundColor: colors.card, padding: 17 },
  sectionEyebrow: { color: colors.accent, fontSize: 10, fontWeight: '900' },
  summaryText: { color: colors.text, fontSize: 17, lineHeight: 25, fontWeight: '700' },
  briefingSection: { gap: 9 },
  sectionHeader: { minHeight: 30, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  sectionTitle: { color: colors.text, fontSize: 19, fontWeight: '900' },
  briefingCard: { gap: 8, borderRadius: 18, borderWidth: 1, borderColor: colors.border, backgroundColor: '#0d1d31', padding: 14 },
  briefingHeader: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 10 },
  briefingTitle: { flex: 1, color: colors.text, fontSize: 14, fontWeight: '900' },
  briefingState: { color: colors.mutedText, fontSize: 11, fontWeight: '800' },
  briefingBody: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  mapCard: { overflow: 'hidden', gap: 12, borderRadius: 24, borderWidth: 1, borderColor: '#24435d', backgroundColor: colors.card, padding: 15 },
  mapHeader: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  mapHeaderCopy: { flex: 1, gap: 3 },
  appetiteName: { color: colors.text, fontSize: 17, fontWeight: '900' },
  mapLink: { color: colors.accent, fontSize: 11, fontWeight: '800' },
  mapLoading: { height: 180 },
  emptyMap: { minHeight: 180, alignItems: 'center', justifyContent: 'center', gap: 12, borderRadius: 20, backgroundColor: '#091827', padding: 24 },
  emptyGlyph: { color: colors.accent, fontSize: 32 },
  emptyDetail: { color: colors.mutedText, fontSize: 13, lineHeight: 20, textAlign: 'center' },
  teachButton: { minHeight: 46, alignItems: 'center', justifyContent: 'center', borderRadius: 14, backgroundColor: colors.button },
  teachText: { color: colors.text, fontSize: 14, fontWeight: '900' },
  shadowBanner: { minHeight: 44, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', borderRadius: 13, backgroundColor: '#2d2750', paddingHorizontal: 13 },
  shadowBannerText: { color: '#ddd6fe', fontSize: 11, fontWeight: '900' },
  shadowBannerArrow: { color: '#c4b5fd', fontSize: 22 },
  flowCard: { gap: 6, borderRadius: 24, backgroundColor: colors.card, padding: 17 },
  flowTitle: { color: colors.text, fontSize: 20, fontWeight: '900' },
  flowTotal: { color: colors.mutedText, fontSize: 12 },
  statsGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 9, marginTop: 10 },
  stat: { width: '48%', flexGrow: 1, minHeight: 82, borderRadius: 16, backgroundColor: '#0a1a2b', padding: 12 },
  statCount: { fontSize: 25, fontWeight: '900', fontVariant: ['tabular-nums'] },
  statLabel: { marginTop: 3, color: colors.mutedText, fontSize: 11, lineHeight: 16 },
  quietZoneButton: { minHeight: 64, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginTop: 8, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: colors.border, paddingTop: 14 },
  quietZoneTitle: { color: colors.text, fontSize: 14, fontWeight: '900' },
  quietZoneDetail: { marginTop: 3, color: colors.mutedText, fontSize: 10 },
  quietZoneArrow: { color: colors.accent, fontSize: 27 },
});
