import { useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useRef, useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  useWindowDimensions,
  View,
} from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { colors } from '../../ui/theme';
import { useReducedMotion } from '../../ui/useReducedMotion';
import { TeachingCardDeck, type TeachingCardDeckHandle } from './TeachingCardDeck';
import { TeachingReasonSheet } from './TeachingReasonSheet';
import { useTeachingSession } from './useTeachingSession';

const reasonCodes = new Set([
  'important_customer', 'purchase_intent', 'needs_reply', 'suitable_job',
  'current_project', 'deadline', 'industry_signal', 'advertising', 'training',
  'duplicate', 'unrelated_chat', 'expired', 'untrusted_source',
]);

function TeachingOnboarding({
  onClose,
  onStart,
}: {
  onClose(): void;
  onStart(): void;
}) {
  const { t } = useI18n();
  const [step, setStep] = useState(0);
  const steps = [
    { title: t('teaching.onboarding.step1.title'), body: t('teaching.onboarding.step1.body'), icon: '↙︎ ↗︎' },
    { title: t('teaching.onboarding.step2.title'), body: t('teaching.onboarding.step2.body'), icon: '✦' },
    { title: t('teaching.onboarding.step3.title'), body: t('teaching.onboarding.step3.body'), icon: '↶' },
  ];
  const current = steps[step];
  return (
    <View style={styles.onboarding}>
      <Text style={styles.eyebrow}>{t('teaching.onboarding.eyebrow')}</Text>
      <View style={styles.onboardingVisual}><Text style={styles.onboardingIcon}>{current.icon}</Text></View>
      <Text accessibilityRole="header" style={styles.onboardingTitle}>{current.title}</Text>
      <Text style={styles.onboardingBody}>{current.body}</Text>
      <View style={styles.dots}>
        {steps.map((_, index) => <View key={index} style={[styles.dot, index === step && styles.dotActive]} />)}
      </View>
      <Pressable
        accessibilityRole="button"
        onPress={() => step === steps.length - 1 ? onStart() : setStep((value) => value + 1)}
        style={styles.primaryButton}
      >
        <Text style={styles.primaryButtonText}>
          {step === steps.length - 1 ? t('teaching.onboarding.start') : t('teaching.onboarding.next')}
        </Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={onClose} style={styles.textButton}>
        <Text style={styles.textButtonText}>{t('teaching.onboarding.later')}</Text>
      </Pressable>
    </View>
  );
}

export default function TeachingScreen() {
  const router = useRouter();
  const { t } = useI18n();
  const reduceMotion = useReducedMotion();
  const { height } = useWindowDimensions();
  const compact = height < 750;
  const deck = useRef<TeachingCardDeckHandle>(null);
  const [contextVisible, setContextVisible] = useState(false);
  const [reasonVisible, setReasonVisible] = useState(false);
  const [applyConfirmVisible, setApplyConfirmVisible] = useState(false);
  const {
    annotate,
    apply,
    applied,
    begin,
    capture,
    cards,
    complete,
    currentCard,
    lastExampleId,
    proposal,
    preparing,
    setDragging,
    state,
    summary,
    simulation,
    undo,
  } = useTeachingSession();
  const lastAction = state.completedActions.at(-1)?.label;

  if (state.phase === 'onboarding') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <TeachingOnboarding onClose={() => router.back()} onStart={() => void begin()} />
        <StatusBar style="light" />
      </SafeAreaView>
    );
  }

  if (state.phase === 'loading' || preparing) {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.centerState}>
          <ActivityIndicator color={colors.accent} size="large" />
          <Text style={styles.loadingText}>{t('teaching.loading')}</Text>
        </View>
      </SafeAreaView>
    );
  }

  if (state.phase === 'completed' && !summary) {
    const reasonLabel = lastAction === 'positive' || lastAction === 'negative' ? lastAction : null;
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.reviewContainer}>
          <Text style={styles.eyebrow}>{t('teaching.summary.eyebrow')}</Text>
          <View style={styles.summaryMark}><Text style={styles.summaryMarkText}>✓</Text></View>
          <Text accessibilityRole="header" style={styles.summaryTitle}>{t('teaching.review.title')}</Text>
          <Text style={styles.onboardingBody}>{t('teaching.review.body')}</Text>
          {reasonLabel && lastExampleId ? (
            <Pressable accessibilityRole="button" onPress={() => setReasonVisible(true)} style={styles.reasonButton}>
              <Text style={styles.reasonButtonText}>{t('teaching.action.reason')}</Text>
            </Pressable>
          ) : null}
          <Pressable accessibilityRole="button" onPress={() => void complete()} style={styles.primaryButton}>
            <Text style={styles.primaryButtonText}>{t('teaching.review.preview')}</Text>
          </Pressable>
          <Pressable accessibilityRole="button" onPress={() => void undo()} style={styles.textButton}>
            <Text style={styles.textButtonText}>↶ {t('teaching.action.undo')}</Text>
          </Pressable>
        </View>
        {reasonLabel ? (
          <TeachingReasonSheet
            label={reasonLabel}
            onClose={() => setReasonVisible(false)}
            onSave={(reasons, freeform) => void annotate(reasons, freeform)}
            reduceMotion={reduceMotion}
            visible={reasonVisible}
          />
        ) : null}
      </SafeAreaView>
    );
  }

  if (state.phase === 'error') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.centerState}>
          <Text style={styles.errorIcon}>!</Text>
          <Text accessibilityRole="alert" style={styles.errorText}>{t('teaching.error')}</Text>
          <Pressable accessibilityRole="button" onPress={() => void begin(false)} style={styles.primaryButton}>
            <Text style={styles.primaryButtonText}>{t('teaching.retry')}</Text>
          </Pressable>
          <Pressable accessibilityRole="button" onPress={() => router.back()} style={styles.textButton}>
            <Text style={styles.textButtonText}>{t('common.back')}</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    );
  }

  if (state.phase === 'completed' && summary) {
    return (
      <SafeAreaView style={styles.safeArea}>
        <ScrollView contentContainerStyle={styles.summaryContainer}>
          <Text style={styles.eyebrow}>{t('teaching.summary.eyebrow')}</Text>
          <View style={styles.summaryMark}><Text style={styles.summaryMarkText}>✦</Text></View>
          <Text accessibilityRole="header" style={styles.summaryTitle}>{t('teaching.summary.title')}</Text>
          {summary.increase.length || summary.reduce.length ? (
            <View style={styles.summaryGrid}>
              <View style={[styles.summaryCard, styles.increaseCard]}>
                <Text style={styles.summaryLabel}>{t('teaching.summary.increase')}</Text>
                <Text style={styles.summaryItems}>{summary.increase.map((reason) => (
                  reasonCodes.has(reason) ? t(`teaching.reason.${reason}` as MessageKey) : reason
                )).join(' · ') || '—'}</Text>
              </View>
              <View style={[styles.summaryCard, styles.reduceCard]}>
                <Text style={styles.summaryLabel}>{t('teaching.summary.reduce')}</Text>
                <Text style={styles.summaryItems}>{summary.reduce.map((reason) => (
                  reasonCodes.has(reason) ? t(`teaching.reason.${reason}` as MessageKey) : reason
                )).join(' · ') || '—'}</Text>
              </View>
            </View>
          ) : <Text style={styles.onboardingBody}>{t('teaching.summary.empty')}</Text>}
          {simulation && proposal ? (
            <View style={styles.previewCard}>
              <View style={styles.previewHeader}>
                <Text style={styles.previewTitle}>{t('teaching.preview.title')}</Text>
                <Text style={styles.notApplied}>{applied
                  ? t('teaching.preview.applied')
                  : t('teaching.preview.notApplied')}</Text>
              </View>
              <View style={styles.previewGrid}>
                {([
                  ['original', simulation.originalCount],
                  ['immediate', simulation.immediateCount],
                  ['inbox', simulation.inboxCount],
                  ['digest', simulation.digestCount],
                  ['suppress', simulation.suppressCount],
                ] as const).map(([key, count]) => (
                  <View key={key} style={styles.previewStat}>
                    <Text style={styles.previewCount}>{count}</Text>
                    <Text style={styles.previewLabel}>{t(`teaching.preview.${key}`)}</Text>
                  </View>
                ))}
              </View>
              {!applied ? (
                <Pressable accessibilityRole="button" onPress={() => setApplyConfirmVisible(true)} style={styles.primaryButton}>
                  <Text style={styles.primaryButtonText}>{t('teaching.preview.apply')}</Text>
                </Pressable>
              ) : null}
            </View>
          ) : null}
          {!applied ? (
            <Pressable accessibilityRole="button" onPress={() => void begin(false)} style={styles.textButton}>
              <Text style={styles.textButtonText}>{t('teaching.summary.more')}</Text>
            </Pressable>
          ) : null}
          <Pressable accessibilityRole="button" onPress={() => router.back()} style={styles.textButton}>
            <Text style={styles.textButtonText}>{t('teaching.summary.close')}</Text>
          </Pressable>
        </ScrollView>
        <Modal animationType="fade" onRequestClose={() => setApplyConfirmVisible(false)} transparent visible={applyConfirmVisible}>
          <View style={styles.confirmBackdrop}>
            <View accessibilityViewIsModal style={styles.confirmCard}>
              <Text accessibilityRole="header" style={styles.contextTitle}>{t('teaching.preview.confirmTitle')}</Text>
              <Text style={styles.onboardingBody}>{t('teaching.preview.confirmBody')}</Text>
              <Pressable accessibilityRole="button" onPress={() => {
                setApplyConfirmVisible(false);
                void apply();
              }} style={styles.primaryButton}>
                <Text style={styles.primaryButtonText}>{t('teaching.preview.confirm')}</Text>
              </Pressable>
              <Pressable accessibilityRole="button" onPress={() => setApplyConfirmVisible(false)} style={styles.textButton}>
                <Text style={styles.textButtonText}>{t('common.cancel')}</Text>
              </Pressable>
            </View>
          </View>
        </Modal>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.container}>
        <View style={styles.header}>
          <Pressable accessibilityLabel={t('common.back')} accessibilityRole="button" onPress={() => router.back()} style={styles.backButton}>
            <Text style={styles.backText}>‹</Text>
          </Pressable>
          <View style={styles.headerCopy}>
            <Text accessibilityRole="header" style={styles.title}>{t('teaching.title')}</Text>
            <Text numberOfLines={1} style={styles.subtitle}>{t('teaching.subtitle')}</Text>
          </View>
          <Pressable accessibilityRole="button" onPress={() => void complete()} style={styles.finishButton}>
            <Text style={styles.finishText}>{t('teaching.finishEarly')}</Text>
          </Pressable>
        </View>
        <View style={styles.progressRow}>
          <View style={styles.progressTrack}>
            <View style={[styles.progressFill, { width: `${Math.min(100, (state.cardIndex / cards.length) * 100)}%` }]} />
          </View>
          <Text style={styles.progressText}>{t('teaching.progress', {
            current: Math.min(state.cardIndex + 1, cards.length), total: cards.length,
          })}</Text>
        </View>
        {currentCard ? (
          <TeachingCardDeck
            ref={deck}
            card={currentCard}
            compact={compact}
            nextCard={cards[state.cardIndex + 1] ?? null}
            onCommit={(label) => void capture(label)}
            onDragging={setDragging}
            reduceMotion={reduceMotion}
            t={t}
          />
        ) : null}
        <View style={styles.semanticRow}>
          <Pressable accessibilityRole="button" onPress={() => deck.current?.commit('positive')} style={[styles.semanticButton, styles.positiveButton]}>
            <Text style={styles.positiveButtonText}>← ★ {t('teaching.action.positive')}</Text>
          </Pressable>
          <Pressable accessibilityRole="button" onPress={() => deck.current?.commit('negative')} style={[styles.semanticButton, styles.negativeButton]}>
            <Text style={styles.negativeButtonText}>{t('teaching.action.negative')} ◌ →</Text>
          </Pressable>
        </View>
        <View style={styles.secondaryRow}>
          <Pressable accessibilityRole="button" onPress={() => void capture('skipped')} style={styles.secondaryButton}>
            <Text style={styles.secondaryText}>↓ {t('teaching.action.skip')}</Text>
          </Pressable>
          <Pressable accessibilityRole="button" onPress={() => setContextVisible(true)} style={styles.secondaryButton}>
            <Text style={styles.secondaryText}>↗ {t('teaching.action.context')}</Text>
          </Pressable>
          <Pressable accessibilityRole="button" onPress={() => setContextVisible(true)} style={styles.secondaryButton}>
            <Text style={styles.secondaryText}>••• {t('teaching.action.more')}</Text>
          </Pressable>
        </View>
        {lastAction ? (
          <View accessibilityLiveRegion="polite" style={styles.feedbackBar}>
            <Text style={styles.feedbackText}>{t(`teaching.feedback.${lastAction}`)}</Text>
            {(lastAction === 'positive' || lastAction === 'negative') && lastExampleId ? (
              <Pressable accessibilityRole="button" onPress={() => setReasonVisible(true)}>
                <Text style={styles.reasonInline}>{t('teaching.action.reason')}</Text>
              </Pressable>
            ) : null}
            <Pressable accessibilityRole="button" onPress={() => void undo()}>
              <Text style={styles.undoText}>↶ {t('teaching.action.undo')}</Text>
            </Pressable>
          </View>
        ) : <Text style={styles.privacy}>{t('teaching.privacy')}</Text>}
      </View>
      <Modal animationType={reduceMotion ? 'fade' : 'slide'} onRequestClose={() => setContextVisible(false)} transparent visible={contextVisible}>
        <Pressable onPress={() => setContextVisible(false)} style={styles.modalBackdrop}>
          <Pressable style={styles.contextSheet}>
            <View style={styles.sheetHandle} />
            <Text style={styles.contextTitle}>{currentCard?.conversationName}</Text>
            <Text style={styles.contextMeta}>{currentCard?.senderName} · {currentCard?.platform}</Text>
            <Text selectable style={styles.contextBody}>{currentCard?.body}</Text>
            <Pressable accessibilityRole="button" onPress={() => setContextVisible(false)} style={styles.primaryButton}>
              <Text style={styles.primaryButtonText}>{t('common.back')}</Text>
            </Pressable>
          </Pressable>
        </Pressable>
      </Modal>
      {lastAction === 'positive' || lastAction === 'negative' ? (
        <TeachingReasonSheet
          label={lastAction}
          onClose={() => setReasonVisible(false)}
          onSave={(reasons, freeform) => void annotate(reasons, freeform)}
          reduceMotion={reduceMotion}
          visible={reasonVisible}
        />
      ) : null}
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { flex: 1, paddingHorizontal: 18, paddingTop: 10, paddingBottom: 12 },
  header: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  backButton: { width: 40, height: 40, alignItems: 'center', justifyContent: 'center', borderRadius: 13, borderWidth: 1, borderColor: colors.border },
  backText: { color: colors.text, fontSize: 30, lineHeight: 32 },
  headerCopy: { flex: 1 },
  title: { color: colors.text, fontSize: 23, fontWeight: '900' },
  subtitle: { marginTop: 2, color: colors.mutedText, fontSize: 11 },
  finishButton: { minHeight: 40, justifyContent: 'center', paddingHorizontal: 6 },
  finishText: { color: colors.accent, fontSize: 12, fontWeight: '800' },
  progressRow: { flexDirection: 'row', alignItems: 'center', gap: 10, marginVertical: 12 },
  progressTrack: { flex: 1, height: 4, overflow: 'hidden', borderRadius: 99, backgroundColor: colors.card },
  progressFill: { height: '100%', borderRadius: 99, backgroundColor: colors.accent },
  progressText: { color: colors.subtleText, fontSize: 11, fontVariant: ['tabular-nums'] },
  semanticRow: { flexDirection: 'row', gap: 9, marginTop: 14 },
  semanticButton: { flex: 1, minHeight: 48, alignItems: 'center', justifyContent: 'center', borderRadius: 15, borderWidth: 1, paddingHorizontal: 8 },
  positiveButton: { borderColor: '#236b5d', backgroundColor: '#12372f' },
  negativeButton: { borderColor: '#704151', backgroundColor: '#38232c' },
  positiveButtonText: { color: '#a7f3d0', fontSize: 12, fontWeight: '900', textAlign: 'center' },
  negativeButtonText: { color: '#fecdd3', fontSize: 12, fontWeight: '900', textAlign: 'center' },
  secondaryRow: { flexDirection: 'row', justifyContent: 'center', gap: 8, marginTop: 8 },
  secondaryButton: { minHeight: 42, justifyContent: 'center', borderRadius: 12, paddingHorizontal: 10 },
  secondaryText: { color: colors.mutedText, fontSize: 12, fontWeight: '700' },
  feedbackBar: { minHeight: 44, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: 8, borderRadius: 14, backgroundColor: colors.card, paddingHorizontal: 14 },
  feedbackText: { flex: 1, color: colors.text, fontSize: 12, fontWeight: '700' },
  reasonInline: { marginHorizontal: 10, color: colors.mutedText, fontSize: 11, fontWeight: '800' },
  undoText: { color: colors.accent, fontSize: 12, fontWeight: '900' },
  privacy: { marginTop: 8, color: colors.subtleText, fontSize: 10, lineHeight: 15, textAlign: 'center' },
  centerState: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 18, padding: 32 },
  loadingText: { color: colors.mutedText, fontSize: 14, textAlign: 'center' },
  errorIcon: { width: 52, height: 52, borderRadius: 26, overflow: 'hidden', backgroundColor: colors.errorBackground, color: colors.errorText, fontSize: 34, textAlign: 'center' },
  errorText: { color: colors.errorText, fontSize: 15, lineHeight: 22, textAlign: 'center' },
  onboarding: { flex: 1, justifyContent: 'center', padding: 28 },
  eyebrow: { color: colors.accent, fontSize: 11, fontWeight: '900', letterSpacing: 1.5, textTransform: 'uppercase' },
  onboardingVisual: { height: 170, alignItems: 'center', justifyContent: 'center', marginVertical: 28, borderRadius: 32, backgroundColor: colors.card },
  onboardingIcon: { color: colors.accent, fontSize: 48, fontWeight: '300' },
  onboardingTitle: { color: colors.text, fontSize: 27, lineHeight: 35, fontWeight: '900' },
  onboardingBody: { marginTop: 14, color: colors.mutedText, fontSize: 15, lineHeight: 24 },
  dots: { flexDirection: 'row', gap: 6, marginVertical: 24 },
  dot: { width: 7, height: 7, borderRadius: 4, backgroundColor: colors.border },
  dotActive: { width: 22, backgroundColor: colors.accent },
  primaryButton: { minHeight: 50, alignItems: 'center', justifyContent: 'center', borderRadius: 15, backgroundColor: colors.button, paddingHorizontal: 20 },
  primaryButtonText: { color: colors.text, fontSize: 15, fontWeight: '900' },
  textButton: { minHeight: 46, alignItems: 'center', justifyContent: 'center' },
  textButtonText: { color: colors.mutedText, fontSize: 14, fontWeight: '700' },
  summaryContainer: { flexGrow: 1, justifyContent: 'center', gap: 18, padding: 28 },
  reviewContainer: { flex: 1, justifyContent: 'center', gap: 18, padding: 28 },
  reasonButton: { minHeight: 48, alignItems: 'center', justifyContent: 'center', borderRadius: 15, borderWidth: 1, borderColor: colors.accent, backgroundColor: colors.accentMuted },
  reasonButtonText: { color: '#a7f3d0', fontSize: 14, fontWeight: '900' },
  summaryMark: { width: 66, height: 66, alignItems: 'center', justifyContent: 'center', borderRadius: 24, backgroundColor: colors.accentMuted },
  summaryMarkText: { color: colors.accent, fontSize: 30 },
  summaryTitle: { color: colors.text, fontSize: 31, lineHeight: 39, fontWeight: '900' },
  summaryGrid: { gap: 12 },
  summaryCard: { gap: 8, borderRadius: 18, padding: 17 },
  increaseCard: { backgroundColor: '#12372f' },
  reduceCard: { backgroundColor: '#322832' },
  summaryLabel: { color: colors.mutedText, fontSize: 12, fontWeight: '800' },
  summaryItems: { color: colors.text, fontSize: 16, lineHeight: 24, fontWeight: '800' },
  previewCard: { gap: 15, borderRadius: 20, borderWidth: 1, borderColor: colors.border, backgroundColor: '#0b1b2d', padding: 16 },
  previewHeader: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 10 },
  previewTitle: { color: colors.text, fontSize: 16, fontWeight: '900' },
  notApplied: { color: colors.warning, fontSize: 11, fontWeight: '900' },
  previewGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  previewStat: { minWidth: '29%', flexGrow: 1, borderRadius: 13, backgroundColor: colors.card, padding: 10 },
  previewCount: { color: colors.text, fontSize: 20, fontWeight: '900', fontVariant: ['tabular-nums'] },
  previewLabel: { marginTop: 2, color: colors.mutedText, fontSize: 10, fontWeight: '700' },
  confirmBackdrop: { flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(2, 8, 23, 0.78)', padding: 24 },
  confirmCard: { width: '100%', maxWidth: 440, gap: 16, borderRadius: 24, backgroundColor: colors.card, padding: 22 },
  modalBackdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(2, 8, 23, 0.72)' },
  contextSheet: { maxHeight: '76%', gap: 12, borderTopLeftRadius: 28, borderTopRightRadius: 28, backgroundColor: colors.card, padding: 22, paddingBottom: 36 },
  sheetHandle: { width: 38, height: 4, alignSelf: 'center', borderRadius: 99, backgroundColor: colors.border },
  contextTitle: { color: colors.text, fontSize: 20, fontWeight: '900' },
  contextMeta: { color: colors.mutedText, fontSize: 12 },
  contextBody: { color: colors.text, fontSize: 16, lineHeight: 25 },
});
