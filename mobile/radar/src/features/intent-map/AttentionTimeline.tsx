import type { AttentionScheduleWindow } from '@story2u/radar-core/attention/model';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import type { Translator } from '../../i18n/core';
import { scheduleWindowAt } from './intent-map-model';

export const timelineMinutes = [360, 540, 720, 1080, 1260, 1439] as const;

function formatMinute(minute: number) {
  if (minute === 1439) return '24';
  return String(Math.floor(minute / 60)).padStart(2, '0');
}

export function AttentionTimeline({
  day,
  onSelect,
  selectedMinute,
  t,
  windows,
}: {
  day: number;
  onSelect(minute: number): void;
  selectedMinute: number;
  t: Translator;
  windows: readonly AttentionScheduleWindow[];
}) {
  const selectedWindow = scheduleWindowAt(windows, selectedMinute, day);
  return (
    <View accessibilityLabel={t('intentMap.timeline.accessibility')} style={styles.container}>
      <View style={styles.headingRow}>
        <View>
          <Text style={styles.eyebrow}>{t('intentMap.timeline.title')}</Text>
          <Text style={styles.summary}>
            {selectedWindow?.label || t('intentMap.timeline.default')}
          </Text>
        </View>
        <Text style={styles.mode}>
          {selectedWindow
            ? t('intentMap.timeline.active', { count: selectedWindow.activeIntentIds.length })
            : t('intentMap.timeline.fallback')}
        </Text>
      </View>
      <View style={styles.rail}>
        <View pointerEvents="none" style={styles.line} />
        {timelineMinutes.map((minute) => {
          const active = minute === selectedMinute;
          const scheduled = scheduleWindowAt(windows, minute, day) !== null;
          return (
            <Pressable
              accessibilityLabel={t('intentMap.timeline.hour', { hour: formatMinute(minute) })}
              accessibilityRole="button"
              accessibilityState={{ selected: active }}
              key={minute}
              onPress={() => onSelect(minute)}
              style={styles.stop}
            >
              <View style={[styles.dot, scheduled && styles.scheduledDot, active && styles.activeDot]} />
              <Text style={[styles.hour, active && styles.activeHour]}>{formatMinute(minute)}</Text>
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: 13, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: '#24435d', backgroundColor: '#091827', paddingHorizontal: 14, paddingTop: 14, paddingBottom: 11 },
  headingRow: { flexDirection: 'row', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 },
  eyebrow: { color: '#7dd3fc', fontSize: 10, fontWeight: '900', letterSpacing: 1.2, textTransform: 'uppercase' },
  summary: { maxWidth: 220, marginTop: 3, color: '#e2e8f0', fontSize: 12, fontWeight: '800' },
  mode: { maxWidth: 100, color: '#94a3b8', fontSize: 10, lineHeight: 14, textAlign: 'right' },
  rail: { minHeight: 42, flexDirection: 'row', alignItems: 'flex-start', justifyContent: 'space-between' },
  line: { position: 'absolute', top: 8, right: 11, left: 11, height: 1, backgroundColor: '#31536d' },
  stop: { minWidth: 31, alignItems: 'center', gap: 5 },
  dot: { width: 7, height: 7, marginTop: 5, borderRadius: 99, backgroundColor: '#31536d' },
  scheduledDot: { backgroundColor: '#38bdf8' },
  activeDot: { width: 15, height: 15, marginTop: 1, borderWidth: 3, borderColor: '#164e63', backgroundColor: '#67e8f9' },
  hour: { color: '#64748b', fontSize: 9, fontVariant: ['tabular-nums'] },
  activeHour: { color: '#e0f2fe', fontWeight: '900' },
});
