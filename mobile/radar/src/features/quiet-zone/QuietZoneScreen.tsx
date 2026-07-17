import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { ActivityIndicator, Modal, Pressable, SafeAreaView, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { colors } from '../../ui/theme';
import { confidenceBand, type QuietZoneItem } from './quiet-zone-model';
import { useQuietZone } from './useQuietZone';

function evaluatorKey(value: QuietZoneItem['evaluator']): MessageKey {
  return `quietZone.evaluator.${value}`;
}

export default function QuietZoneScreen() {
  const router = useRouter();
  const { locale, t } = useI18n();
  const { error, items, load, loading } = useQuietZone();
  const [selected, setSelected] = useState<QuietZoneItem | null>(null);
  const formatTime = (value: string) => new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.header}>
        <Pressable accessibilityLabel={t('common.back')} accessibilityRole="button" onPress={() => router.back()} style={styles.backButton}>
          <Text style={styles.backText}>‹</Text>
        </Pressable>
        <View style={styles.headerCopy}>
          <Text accessibilityRole="header" style={styles.title}>{t('quietZone.title')}</Text>
          <Text style={styles.subtitle}>{t('quietZone.subtitle')}</Text>
        </View>
      </View>
      {loading ? <ActivityIndicator color={colors.accent} size="large" style={styles.loading} /> : null}
      {error ? <View style={styles.error}><Text accessibilityRole="alert" style={styles.errorText}>{error}</Text><Pressable onPress={() => void load()}><Text style={styles.link}>{t('common.retry')}</Text></Pressable></View> : null}
      {!loading && !error ? (
        <ScrollView contentContainerStyle={styles.list} showsVerticalScrollIndicator={false}>
          <View style={styles.safetyNote}><Text style={styles.safetyGlyph}>◌</Text><Text style={styles.safetyText}>{t('quietZone.safety')}</Text></View>
          {items.map((item) => (
            <Pressable accessibilityHint={t('quietZone.openExplanation')} accessibilityRole="button" key={item.messageId} onPress={() => setSelected(item)} style={styles.messageCard}>
              <View style={styles.messageHeader}><Text style={styles.sender}>{item.senderName}</Text><Text style={styles.time}>{formatTime(item.sentAt)}</Text></View>
              <Text numberOfLines={3} style={styles.body}>{item.body}</Text>
              <View style={styles.reasonRow}><Text style={styles.quietBadge}>{t('quietZone.location')}</Text><Text numberOfLines={1} style={styles.reason}>{item.reasonSummary}</Text></View>
            </Pressable>
          ))}
          {items.length === 0 ? <View style={styles.empty}><Text style={styles.emptyGlyph}>☾</Text><Text style={styles.emptyTitle}>{t('quietZone.empty')}</Text><Text style={styles.emptyDetail}>{t('quietZone.emptyDetail')}</Text></View> : null}
        </ScrollView>
      ) : null}
      <Modal animationType="fade" onRequestClose={() => setSelected(null)} transparent visible={selected !== null}>
        <Pressable onPress={() => setSelected(null)} style={styles.backdrop}>
          <Pressable style={styles.sheet}>
            <View style={styles.handle} />
            <Text style={styles.sheetEyebrow}>{t('quietZone.explanation')}</Text>
            <Text accessibilityRole="header" style={styles.sheetTitle}>{selected?.reasonSummary}</Text>
            <View style={styles.fact}><Text style={styles.factLabel}>{t('quietZone.processing')}</Text><Text style={styles.factValue}>{t('quietZone.location')}</Text></View>
            <View style={styles.fact}><Text style={styles.factLabel}>{t('quietZone.confidence')}</Text><Text style={styles.factValue}>{selected ? t(`quietZone.confidence.${confidenceBand(selected.confidence)}`) : ''}</Text></View>
            <View style={styles.fact}><Text style={styles.factLabel}>{t('quietZone.evaluator')}</Text><Text style={styles.factValue}>{selected ? t(evaluatorKey(selected.evaluator)) : ''}</Text></View>
            <Text style={styles.evidenceTitle}>{t('quietZone.evidence')}</Text>
            {selected?.evidence.map((evidence, index) => <Text key={`${evidence.kind}:${index}`} style={styles.evidence}>• {evidence.label.replaceAll('_', ' ')}</Text>)}
            <Text style={styles.privateNote}>{t('quietZone.privateReasoning')}</Text>
            <Pressable accessibilityRole="button" onPress={() => { setSelected(null); router.push('/(tabs)/agent' as Href); }} style={styles.primaryButton}>
              <Text style={styles.primaryText}>{t('quietZone.correct')}</Text>
            </Pressable>
          </Pressable>
        </Pressable>
      </Modal>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  header: { flexDirection: 'row', alignItems: 'flex-start', gap: 12, paddingHorizontal: 18, paddingTop: 14, paddingBottom: 10 },
  backButton: { width: 40, height: 40, alignItems: 'center', justifyContent: 'center', borderRadius: 13, borderWidth: 1, borderColor: colors.border },
  backText: { color: colors.text, fontSize: 30, lineHeight: 32 },
  headerCopy: { flex: 1, gap: 4 },
  title: { color: colors.text, fontSize: 27, fontWeight: '900' },
  subtitle: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  loading: { flex: 1 },
  error: { gap: 10, margin: 18, borderRadius: 18, backgroundColor: colors.errorBackground, padding: 17 },
  errorText: { color: colors.errorText },
  link: { color: colors.accent, fontWeight: '900' },
  list: { gap: 11, padding: 18, paddingBottom: 44 },
  safetyNote: { flexDirection: 'row', alignItems: 'center', gap: 10, borderRadius: 16, borderWidth: 1, borderColor: '#2a465d', backgroundColor: '#0a1928', padding: 13 },
  safetyGlyph: { color: '#7dd3fc', fontSize: 19 },
  safetyText: { flex: 1, color: colors.mutedText, fontSize: 11, lineHeight: 17 },
  messageCard: { gap: 9, borderRadius: 20, backgroundColor: colors.card, padding: 15 },
  messageHeader: { flexDirection: 'row', justifyContent: 'space-between', gap: 12 },
  sender: { flex: 1, color: colors.text, fontSize: 13, fontWeight: '900' },
  time: { color: colors.subtleText, fontSize: 10 },
  body: { color: '#cbd5e1', fontSize: 13, lineHeight: 19 },
  reasonRow: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  quietBadge: { borderRadius: 99, backgroundColor: '#283344', color: '#cbd5e1', paddingHorizontal: 8, paddingVertical: 4, fontSize: 9, fontWeight: '900' },
  reason: { flex: 1, color: colors.subtleText, fontSize: 10 },
  empty: { minHeight: 340, alignItems: 'center', justifyContent: 'center', gap: 10, padding: 28 },
  emptyGlyph: { color: '#94a3b8', fontSize: 36 },
  emptyTitle: { color: colors.text, fontSize: 20, fontWeight: '900' },
  emptyDetail: { color: colors.mutedText, fontSize: 13, lineHeight: 20, textAlign: 'center' },
  backdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(2, 8, 23, 0.74)' },
  sheet: { gap: 12, borderTopLeftRadius: 28, borderTopRightRadius: 28, backgroundColor: colors.card, padding: 22, paddingBottom: 38 },
  handle: { width: 38, height: 4, alignSelf: 'center', borderRadius: 99, backgroundColor: colors.border },
  sheetEyebrow: { color: colors.accent, fontSize: 10, fontWeight: '900', letterSpacing: 1.1 },
  sheetTitle: { color: colors.text, fontSize: 21, lineHeight: 28, fontWeight: '900' },
  fact: { flexDirection: 'row', justifyContent: 'space-between', gap: 14, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.border, paddingVertical: 7 },
  factLabel: { color: colors.mutedText, fontSize: 12 },
  factValue: { flex: 1, color: colors.text, fontSize: 12, fontWeight: '800', textAlign: 'right' },
  evidenceTitle: { marginTop: 3, color: colors.text, fontSize: 13, fontWeight: '900' },
  evidence: { color: '#cbd5e1', fontSize: 12, lineHeight: 18 },
  privateNote: { color: colors.subtleText, fontSize: 10, lineHeight: 15 },
  primaryButton: { minHeight: 50, alignItems: 'center', justifyContent: 'center', borderRadius: 15, backgroundColor: colors.button },
  primaryText: { color: colors.text, fontSize: 14, fontWeight: '900' },
});

