import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import { colors } from '../../ui/theme';
import { displayConcept, IntentMapCanvas } from './IntentMapCanvas';
import { AttentionTimeline, timelineMinutes } from './AttentionTimeline';
import { scheduleWindowAt, type IntentMapNode } from './intent-map-model';
import { useIntentMap } from './useIntentMap';

export default function IntentMapScreen() {
  const router = useRouter();
  const { t } = useI18n();
  const { error, load, loading, model } = useIntentMap();
  const [selected, setSelected] = useState<IntentMapNode | null>(null);
  const [showCandidate, setShowCandidate] = useState(false);
  const now = new Date();
  const [selectedMinute, setSelectedMinute] = useState<number>(() => {
    const minute = now.getHours() * 60 + now.getMinutes();
    return timelineMinutes.reduce((best, item) => (
      Math.abs(item - minute) < Math.abs(best - minute) ? item : best
    ), timelineMinutes[0]);
  });
  const day = now.getDay() === 0 ? 7 : now.getDay();
  const shownModel = showCandidate && model?.candidate
    ? { ...model, ...model.candidate }
    : model;
  const selectedWindow = shownModel?.preference
    ? scheduleWindowAt(shownModel.preference.schedule, selectedMinute, day)
    : null;
  const activeIntentIds = selectedWindow ? new Set(selectedWindow.activeIntentIds) : null;
  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container} showsVerticalScrollIndicator={false}>
        <View style={styles.header}>
          <Pressable accessibilityLabel={t('common.back')} accessibilityRole="button" onPress={() => router.back()} style={styles.backButton}>
            <Text style={styles.backText}>‹</Text>
          </Pressable>
          <View style={styles.headerCopy}>
            <Text accessibilityRole="header" style={styles.title}>{t('intentMap.title')}</Text>
            <Text style={styles.subtitle}>{t('intentMap.subtitle')}</Text>
          </View>
        </View>
        {loading ? <ActivityIndicator color={colors.accent} size="large" style={styles.loading} /> : null}
        {error ? (
          <View style={styles.errorCard}>
            <Text accessibilityRole="alert" style={styles.errorText}>{error}</Text>
            <Pressable accessibilityRole="button" onPress={() => void load()}><Text style={styles.link}>{t('common.retry')}</Text></Pressable>
          </View>
        ) : null}
        {!loading && !error && model?.preference ? (
          <>
            {model.shadow && model.candidate ? (
              <View style={styles.shadowCard}>
                <View style={styles.shadowHeader}>
                  <View><Text style={styles.shadowEyebrow}>{t('intentMap.shadow.eyebrow')}</Text><Text style={styles.shadowTitle}>{t('intentMap.shadow.title')}</Text></View>
                  <Text style={styles.shadowRemaining}>{t('intentMap.shadow.until', { time: new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(new Date(model.shadow.endsAt)) })}</Text>
                </View>
                <Text style={styles.shadowRisk}>{model.shadow.diffSummary.riskSummary}</Text>
                <View style={styles.segmented}>
                  <Pressable accessibilityRole="button" accessibilityState={{ selected: !showCandidate }} onPress={() => setShowCandidate(false)} style={[styles.segment, !showCandidate && styles.segmentActive]}><Text style={styles.segmentText}>{t('intentMap.shadow.current')}</Text></Pressable>
                  <Pressable accessibilityRole="button" accessibilityState={{ selected: showCandidate }} onPress={() => setShowCandidate(true)} style={[styles.segment, showCandidate && styles.segmentActive]}><Text style={styles.segmentText}>{t('intentMap.shadow.candidate')}</Text></Pressable>
                </View>
              </View>
            ) : null}
            <View style={styles.mapFrame}>
              {shownModel ? <IntentMapCanvas activeIntentIds={activeIntentIds} model={shownModel} onNodePress={setSelected} t={t} /> : null}
              <View pointerEvents="none" style={styles.reduceLabel}>
                <Text style={styles.reduceText}>· · · {t('intentMap.reduceZone')} · · ·</Text>
              </View>
              <AttentionTimeline
                day={day}
                onSelect={setSelectedMinute}
                selectedMinute={selectedMinute}
                t={t}
                windows={shownModel?.preference?.schedule ?? model.preference.schedule}
              />
            </View>
            <View style={styles.impactCard}>
              <Text style={styles.sectionTitle}>{t('home.flow.title')}</Text>
              <Text style={styles.sectionMeta}>{t('home.flow.total', { count: model.stats.total })}</Text>
              <View style={styles.impactRow}>
                {([
                  ['immediate', shownModel?.stats.immediate ?? 0, '#fbbf24'],
                  ['inbox', shownModel?.stats.inbox ?? 0, '#38bdf8'],
                  ['digest', shownModel?.stats.digest ?? 0, '#818cf8'],
                  ['suppress', shownModel?.stats.suppress ?? 0, '#94a3b8'],
                ] as const).map(([key, count, color]) => (
                  <View key={key} style={styles.impactStat}>
                    <Text style={[styles.impactCount, { color }]}>{count}</Text>
                    <Text style={styles.impactLabel}>{t(`teaching.preview.${key}`)}</Text>
                  </View>
                ))}
              </View>
            </View>
            <View style={styles.linearCard}>
              <Text style={styles.sectionTitle}>{t('intentMap.linearView')}</Text>
              {model.nodes.slice(1).map((node) => (
                <Pressable
                  accessibilityRole="button"
                  key={node.id}
                  onPress={() => setSelected(node)}
                  style={styles.linearRow}
                >
                  <View style={[styles.linearDot, node.kind === 'reduce' && styles.reduceDot, node.kind === 'temporary' && styles.temporaryDot]} />
                  <Text style={styles.linearLabel}>{displayConcept(node.label, t)}</Text>
                  <Text style={styles.linearMeta}>{node.confirmed ? t('intentMap.confirmed') : t('intentMap.inferred')}</Text>
                </Pressable>
              ))}
            </View>
          </>
        ) : null}
        {!loading && !error && !model?.preference ? (
          <View style={styles.emptyCard}>
            <Text style={styles.emptyGlyph}>✦</Text>
            <Text accessibilityRole="header" style={styles.emptyTitle}>{t('intentMap.empty')}</Text>
            <Text style={styles.emptyDetail}>{t('intentMap.emptyDetail')}</Text>
            <Pressable accessibilityRole="button" onPress={() => router.push('/teaching' as Href)} style={styles.primaryButton}>
              <Text style={styles.primaryText}>{t('home.appetite.teach')}</Text>
            </Pressable>
          </View>
        ) : null}
      </ScrollView>
      <Modal animationType="fade" onRequestClose={() => setSelected(null)} transparent visible={selected !== null}>
        <Pressable onPress={() => setSelected(null)} style={styles.modalBackdrop}>
          <Pressable style={styles.detailSheet}>
            <View style={styles.handle} />
            <Text accessibilityRole="header" style={styles.detailTitle}>{selected ? displayConcept(selected.label, t) : ''}</Text>
            <Text style={styles.detailMeta}>{selected?.confirmed ? t('intentMap.confirmed') : t('intentMap.inferred')}</Text>
            {selected?.badge ? <Text style={styles.temporaryBadge}>{t('intentMap.temporary')}</Text> : null}
            {selected?.deliveryMode ? (
              <Text style={styles.detailBody}>{t('intentMap.delivery', {
                mode: t(`teaching.decision.${selected.deliveryMode}`),
              })}</Text>
            ) : null}
            {selected?.kind !== 'self' ? (
              <Pressable accessibilityRole="button" onPress={() => {
                setSelected(null);
                const prompt = t('agent.context.adjustIntent', { concept: selected ? displayConcept(selected.label, t) : '' });
                router.push(`/(tabs)/agent?prompt=${encodeURIComponent(prompt)}` as Href);
              }} style={styles.primaryButton}>
                <Text style={styles.primaryText}>{t('intentMap.adjust')}</Text>
              </Pressable>
            ) : null}
          </Pressable>
        </Pressable>
      </Modal>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { gap: 16, padding: 18, paddingBottom: 42 },
  header: { flexDirection: 'row', alignItems: 'flex-start', gap: 12, marginTop: 8 },
  backButton: { width: 40, height: 40, alignItems: 'center', justifyContent: 'center', borderRadius: 13, borderWidth: 1, borderColor: colors.border },
  backText: { color: colors.text, fontSize: 30, lineHeight: 32 },
  headerCopy: { flex: 1, gap: 5 },
  title: { color: colors.text, fontSize: 27, fontWeight: '900' },
  subtitle: { color: colors.mutedText, fontSize: 13, lineHeight: 19 },
  loading: { minHeight: 300 },
  errorCard: { gap: 12, borderRadius: 18, backgroundColor: colors.errorBackground, padding: 17 },
  errorText: { color: colors.errorText, lineHeight: 20 },
  link: { color: colors.accent, fontWeight: '900' },
  mapFrame: { overflow: 'hidden', borderRadius: 26, borderWidth: 1, borderColor: '#24435d' },
  reduceLabel: { position: 'absolute', right: 0, bottom: 11, left: 0, alignItems: 'center' },
  reduceText: { color: '#9f6479', fontSize: 10, fontWeight: '800', letterSpacing: 1 },
  shadowCard: { gap: 11, borderRadius: 20, borderWidth: 1, borderColor: '#5b4b89', backgroundColor: '#18152c', padding: 15 },
  shadowHeader: { flexDirection: 'row', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 },
  shadowEyebrow: { color: '#c4b5fd', fontSize: 9, fontWeight: '900', letterSpacing: 1.2 },
  shadowTitle: { marginTop: 3, color: colors.text, fontSize: 16, fontWeight: '900' },
  shadowRemaining: { color: '#a78bfa', fontSize: 10, fontWeight: '800' },
  shadowRisk: { color: colors.mutedText, fontSize: 11, lineHeight: 17 },
  segmented: { flexDirection: 'row', gap: 4, borderRadius: 12, backgroundColor: '#0d1020', padding: 4 },
  segment: { flex: 1, minHeight: 36, alignItems: 'center', justifyContent: 'center', borderRadius: 9 },
  segmentActive: { backgroundColor: '#463b72' },
  segmentText: { color: '#ede9fe', fontSize: 11, fontWeight: '900' },
  impactCard: { gap: 5, borderRadius: 22, backgroundColor: colors.card, padding: 16 },
  sectionTitle: { color: colors.text, fontSize: 17, fontWeight: '900' },
  sectionMeta: { color: colors.mutedText, fontSize: 11 },
  impactRow: { flexDirection: 'row', gap: 7, marginTop: 10 },
  impactStat: { flex: 1, gap: 3, borderRadius: 13, backgroundColor: '#091827', padding: 9 },
  impactCount: { fontSize: 20, fontWeight: '900', fontVariant: ['tabular-nums'] },
  impactLabel: { color: colors.mutedText, fontSize: 9, lineHeight: 13 },
  linearCard: { gap: 2, borderRadius: 22, backgroundColor: colors.card, padding: 16 },
  linearRow: { minHeight: 48, flexDirection: 'row', alignItems: 'center', gap: 10, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.border },
  linearDot: { width: 9, height: 9, borderRadius: 5, backgroundColor: '#38bdf8' },
  reduceDot: { backgroundColor: '#9f6479' },
  temporaryDot: { backgroundColor: '#a78bfa' },
  linearLabel: { flex: 1, color: colors.text, fontSize: 13, fontWeight: '800' },
  linearMeta: { maxWidth: 120, color: colors.subtleText, fontSize: 10, textAlign: 'right' },
  emptyCard: { minHeight: 390, alignItems: 'center', justifyContent: 'center', gap: 14, borderRadius: 26, backgroundColor: colors.card, padding: 26 },
  emptyGlyph: { color: colors.accent, fontSize: 38 },
  emptyTitle: { color: colors.text, fontSize: 23, fontWeight: '900', textAlign: 'center' },
  emptyDetail: { color: colors.mutedText, fontSize: 14, lineHeight: 21, textAlign: 'center' },
  primaryButton: { minHeight: 50, alignItems: 'center', justifyContent: 'center', borderRadius: 15, backgroundColor: colors.button, paddingHorizontal: 18 },
  primaryText: { color: colors.text, fontSize: 14, fontWeight: '900' },
  modalBackdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(2, 8, 23, 0.72)' },
  detailSheet: { gap: 12, borderTopLeftRadius: 28, borderTopRightRadius: 28, backgroundColor: colors.card, padding: 22, paddingBottom: 38 },
  handle: { width: 38, height: 4, alignSelf: 'center', borderRadius: 99, backgroundColor: colors.border },
  detailTitle: { color: colors.text, fontSize: 24, fontWeight: '900' },
  detailMeta: { color: colors.mutedText, fontSize: 12 },
  detailBody: { color: colors.text, fontSize: 14, lineHeight: 21 },
  temporaryBadge: { alignSelf: 'flex-start', borderRadius: 99, backgroundColor: '#3b2562', color: '#ddd6fe', paddingHorizontal: 10, paddingVertical: 5, fontSize: 11, fontWeight: '800' },
});
