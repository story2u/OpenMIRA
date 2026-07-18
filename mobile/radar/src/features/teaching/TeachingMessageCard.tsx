import { memo } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import type { TeachingMessageCard as TeachingCard } from '../../attention/teachingService';
import type { Translator } from '../../i18n/core';
import { colors } from '../../ui/theme';

function confidenceLabel(card: TeachingCard, t: Translator) {
  return t(`teaching.confidence.${card.confidenceLevel}`);
}

function decisionLabel(card: TeachingCard, t: Translator) {
  return t(`teaching.decision.${card.initialDecision}`);
}

export const TeachingMessageCard = memo(function TeachingMessageCard({
  card,
  compact = false,
  t,
}: {
  card: TeachingCard;
  compact?: boolean;
  t: Translator;
}) {
  return (
    <View style={[styles.card, compact && styles.cardCompact]}>
      {card.piUncertain ? (
        <View style={styles.uncertainBanner}>
          <Text style={styles.uncertainText}>{t('teaching.card.uncertain')}</Text>
        </View>
      ) : null}
      <View style={styles.sourceRow}>
        <View style={styles.platformMark}><Text style={styles.platformGlyph}>✦</Text></View>
        <View style={styles.sourceCopy}>
          <Text numberOfLines={1} style={styles.conversation}>{card.conversationName}</Text>
          <Text numberOfLines={1} style={styles.platform}>
            {card.platform} · {card.conversationKind === 'group'
              ? t('teaching.card.group')
              : t('teaching.card.private')}
          </Text>
        </View>
        <Text style={styles.time}>{new Date(card.sentAt).toLocaleTimeString([], {
          hour: '2-digit', minute: '2-digit',
        })}</Text>
      </View>
      {card.groupFunctionSummary ? (
        <Text numberOfLines={2} style={styles.groupSummary}>{card.groupFunctionSummary}</Text>
      ) : null}
      <Text style={styles.sender}>{card.senderName}</Text>
      <Text numberOfLines={compact ? 5 : 8} selectable style={styles.body}>{card.body}</Text>
      <View style={styles.chips}>
        {card.topics.slice(0, 4).map((topic) => (
          <View key={topic} style={styles.topicChip}><Text style={styles.topicText}>{topic}</Text></View>
        ))}
        <View style={styles.metaChip}>
          <Text style={styles.metaText}>{t(`teaching.category.${card.category}`)}</Text>
        </View>
        {card.hasLink ? <Text style={styles.flag}>↗ {t('teaching.card.link')}</Text> : null}
        {card.duplicate ? <Text style={styles.flag}>⧉ {t('teaching.card.duplicate')}</Text> : null}
      </View>
      <View style={styles.divider} />
      <View style={styles.judgmentRow}>
        <View>
          <Text style={styles.judgmentLabel}>{t('teaching.card.piJudgment')}</Text>
          <Text style={styles.judgment}>{decisionLabel(card, t)}</Text>
        </View>
        <View style={styles.confidenceBadge}>
          <Text style={styles.confidence}>{t('teaching.card.confidence', {
            level: confidenceLabel(card, t),
          })}</Text>
        </View>
      </View>
    </View>
  );
});

const styles = StyleSheet.create({
  card: {
    minHeight: 390,
    borderRadius: 28,
    borderWidth: 1,
    borderColor: '#31506b',
    backgroundColor: '#10243a',
    padding: 20,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 16 },
    shadowOpacity: 0.28,
    shadowRadius: 26,
    elevation: 12,
  },
  cardCompact: { minHeight: 340, padding: 16 },
  uncertainBanner: {
    alignSelf: 'flex-start',
    marginBottom: 14,
    borderRadius: 999,
    backgroundColor: colors.noticeBackground,
    paddingHorizontal: 11,
    paddingVertical: 6,
  },
  uncertainText: { color: colors.noticeText, fontSize: 12, fontWeight: '700' },
  sourceRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  platformMark: {
    width: 38, height: 38, alignItems: 'center', justifyContent: 'center',
    borderRadius: 13, backgroundColor: colors.accentMuted,
  },
  platformGlyph: { color: colors.accent, fontSize: 17 },
  sourceCopy: { flex: 1, gap: 2 },
  conversation: { color: colors.text, fontSize: 16, fontWeight: '800' },
  platform: { color: colors.mutedText, fontSize: 12 },
  time: { color: colors.subtleText, fontSize: 12 },
  groupSummary: { marginTop: 14, color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  sender: { marginTop: 20, color: colors.accent, fontSize: 13, fontWeight: '800' },
  body: { marginTop: 8, color: colors.text, fontSize: 17, lineHeight: 27 },
  chips: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: 7, marginTop: 18 },
  topicChip: { borderRadius: 999, backgroundColor: '#183b50', paddingHorizontal: 10, paddingVertical: 6 },
  topicText: { color: '#a7f3d0', fontSize: 12, fontWeight: '700' },
  metaChip: { borderRadius: 999, backgroundColor: '#222f4b', paddingHorizontal: 10, paddingVertical: 6 },
  metaText: { color: '#c7d2fe', fontSize: 12, fontWeight: '700' },
  flag: { color: colors.mutedText, fontSize: 11 },
  divider: { height: StyleSheet.hairlineWidth, marginVertical: 18, backgroundColor: colors.border },
  judgmentRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 10 },
  judgmentLabel: { color: colors.subtleText, fontSize: 11, fontWeight: '700' },
  judgment: { marginTop: 3, color: colors.text, fontSize: 14, fontWeight: '800' },
  confidenceBadge: { borderRadius: 999, borderWidth: 1, borderColor: colors.border, paddingHorizontal: 10, paddingVertical: 6 },
  confidence: { color: colors.mutedText, fontSize: 11, fontWeight: '700' },
});
