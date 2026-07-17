import { normalizeWorkSchedule } from '@story2u/radar-api/settings';
import type { WorkSchedule, WorkScheduleSlot } from '@story2u/radar-contracts/settings';
import { useMemo, useRef, useState } from 'react';
import { Pressable, StyleSheet, Switch, Text, TextInput, View } from 'react-native';

import { colors } from '../../ui/theme';
import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import {
  ActionButton,
  SettingsCard,
  SettingsScaffold,
  StateNotice,
  settingsStyles,
} from './components';
import { useSettings } from './SettingsProvider';

interface EditableSlot extends WorkScheduleSlot {
  key: number;
}

const weekdayKeys: readonly MessageKey[] = [
  'settings.schedule.weekday.1',
  'settings.schedule.weekday.2',
  'settings.schedule.weekday.3',
  'settings.schedule.weekday.4',
  'settings.schedule.weekday.5',
  'settings.schedule.weekday.6',
  'settings.schedule.weekday.7',
];
const commonTimezones = [
  'Asia/Shanghai',
  'Asia/Hong_Kong',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Europe/London',
  'America/New_York',
  'UTC',
];

export function WorkScheduleScreen() {
  const settings = useSettings();
  const { t } = useI18n();
  const initial = settings.state.bundle?.workSchedule;
  if (!initial) {
    return (
      <SettingsScaffold title={t('settings.schedule.title')}>
        <StateNotice
          error={settings.state.loadError}
          loading={settings.state.isLoading}
          onRetry={settings.retry}
        />
      </SettingsScaffold>
    );
  }
  return <WorkScheduleEditor initial={initial} key={JSON.stringify(initial)} />;
}

function WorkScheduleEditor({ initial }: { initial: WorkSchedule }) {
  const { saveSchedule, state } = useSettings();
  const { t } = useI18n();
  const nextKey = useRef(initial.slots.length);
  const [timezone, setTimezone] = useState(initial.timezone);
  const [slots, setSlots] = useState<EditableSlot[]>(() =>
    initial.slots.map((slot, key) => ({ ...slot, key })),
  );
  const [autoReplyOutsideHours, setAutoReplyOutsideHours] = useState(initial.autoReplyOutsideHours);
  const [localError, setLocalError] = useState('');
  const [showSaveError, setShowSaveError] = useState(false);
  const saving = state.busyAction === 'work-schedule';
  const weekdays = useMemo(() => weekdayKeys.map((key) => t(key)), [t]);

  function updateSlot(key: number, patch: Partial<WorkScheduleSlot>) {
    setSlots((current) => current.map((slot) => slot.key === key ? { ...slot, ...patch } : slot));
  }

  function addSlot() {
    const key = nextKey.current++;
    setSlots((current) => [...current, { key, weekday: 1, start: '09:00', end: '18:00' }]);
  }

  function resetToInitial() {
    setTimezone(initial.timezone);
    setSlots(initial.slots.map((slot, key) => ({ ...slot, key })));
    nextKey.current = initial.slots.length;
    setAutoReplyOutsideHours(initial.autoReplyOutsideHours);
  }

  async function save() {
    setLocalError('');
    setShowSaveError(false);
    let input;
    try {
      input = normalizeWorkSchedule({
        timezone,
        slots: slots.map(({ key: _key, ...slot }) => slot),
        autoReplyOutsideHours,
      });
    } catch {
      setLocalError(t('settings.schedule.error.invalid'));
      return;
    }
    const saved = await saveSchedule(input);
    if (!saved) {
      resetToInitial();
      setShowSaveError(true);
    }
  }

  return (
    <SettingsScaffold
      title={t('settings.schedule.title')}
      subtitle={t('settings.schedule.subtitle')}
    >
      <SettingsCard>
        <Text style={settingsStyles.label}>{t('settings.schedule.timezone')}</Text>
        <TextInput
          accessibilityLabel={t('settings.schedule.timezone')}
          autoCapitalize="none"
          autoCorrect={false}
          maxLength={64}
          onChangeText={setTimezone}
          placeholder="Asia/Shanghai"
          placeholderTextColor={colors.placeholder}
          style={settingsStyles.input}
          value={timezone}
        />
        <View style={styles.timezoneChips}>
          {commonTimezones.map((value) => (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected: timezone === value }}
              key={value}
              onPress={() => setTimezone(value)}
              style={[styles.timezoneChip, timezone === value && styles.selectedChip]}
            >
              <Text style={[styles.timezoneText, timezone === value && styles.selectedText]}>{value}</Text>
            </Pressable>
          ))}
        </View>
      </SettingsCard>

      <SettingsCard>
        <View style={settingsStyles.rowBetween}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={settingsStyles.label}>{t('settings.schedule.autoReply')}</Text>
            <Text style={settingsStyles.body}>{t('settings.schedule.autoReplyDetail')}</Text>
          </View>
          <Switch
            accessibilityLabel={t('settings.schedule.autoReply')}
            onValueChange={setAutoReplyOutsideHours}
            trackColor={{ false: colors.border, true: colors.button }}
            value={autoReplyOutsideHours}
          />
        </View>
      </SettingsCard>

      <SettingsCard>
        <View style={settingsStyles.rowBetween}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={settingsStyles.label}>{t('settings.schedule.reviewSlots')}</Text>
            <Text style={settingsStyles.body}>
              {slots.length === 0
                ? t('settings.schedule.emptyCount')
                : t('settings.schedule.slotCount', { count: slots.length })}
            </Text>
          </View>
          <ActionButton label={t('settings.schedule.addSlot')} onPress={addSlot} tone="secondary" />
        </View>
        {slots.length === 0 ? (
          <Text style={styles.empty}>{t('settings.schedule.empty')}</Text>
        ) : null}
        {slots.map((slot, index) => (
          <View key={slot.key} style={styles.slotCard}>
            <View style={settingsStyles.rowBetween}>
              <Text style={settingsStyles.label}>{t('settings.schedule.slot', { index: index + 1 })}</Text>
              <Pressable
                accessibilityLabel={t('settings.schedule.deleteSlot', { index: index + 1 })}
                accessibilityRole="button"
                onPress={() => setSlots((current) => current.filter((item) => item.key !== slot.key))}
              >
                <Text style={styles.delete}>{t('common.delete')}</Text>
              </Pressable>
            </View>
            <View style={styles.weekdays}>
              {weekdays.map((label, weekdayIndex) => {
                const value = weekdayIndex + 1;
                const selected = slot.weekday === value;
                return (
                  <Pressable
                    accessibilityRole="button"
                    accessibilityState={{ selected }}
                    key={label}
                    onPress={() => updateSlot(slot.key, { weekday: value })}
                    style={[styles.weekday, selected && styles.selectedChip]}
                  >
                    <Text style={[styles.weekdayText, selected && styles.selectedText]}>{label}</Text>
                  </Pressable>
                );
              })}
            </View>
            <View style={settingsStyles.row}>
              <TextInput
                accessibilityLabel={t('settings.schedule.startTime', { index: index + 1 })}
                maxLength={5}
                onChangeText={(start) => updateSlot(slot.key, { start })}
                placeholder="09:00"
                placeholderTextColor={colors.placeholder}
                style={[settingsStyles.input, styles.timeInput]}
                value={slot.start}
              />
              <Text style={settingsStyles.body}>{t('settings.schedule.to')}</Text>
              <TextInput
                accessibilityLabel={t('settings.schedule.endTime', { index: index + 1 })}
                maxLength={5}
                onChangeText={(end) => updateSlot(slot.key, { end })}
                placeholder="18:00"
                placeholderTextColor={colors.placeholder}
                style={[settingsStyles.input, styles.timeInput]}
                value={slot.end}
              />
            </View>
          </View>
        ))}
      </SettingsCard>

      {localError ? <Text accessibilityRole="alert" style={settingsStyles.error}>{localError}</Text> : null}
      {showSaveError && state.saveError ? (
        <Text accessibilityRole="alert" style={settingsStyles.error}>{state.saveError}</Text>
      ) : null}
      <ActionButton busy={saving} label={saving ? t('common.saving') : t('settings.schedule.save')} onPress={() => void save()} />
    </SettingsScaffold>
  );
}

const styles = StyleSheet.create({
  timezoneChips: { flexDirection: 'row', flexWrap: 'wrap', gap: 7 },
  timezoneChip: { borderWidth: 1, borderColor: colors.border, borderRadius: 9, paddingHorizontal: 9, paddingVertical: 7 },
  timezoneText: { color: colors.mutedText, fontSize: 11 },
  selectedChip: { borderColor: colors.accent, backgroundColor: colors.accentMuted },
  selectedText: { color: colors.text, fontWeight: '800' },
  empty: { borderRadius: 12, backgroundColor: colors.background, color: colors.mutedText, padding: 13, lineHeight: 20 },
  slotCard: { gap: 12, borderWidth: 1, borderColor: colors.border, borderRadius: 13, padding: 13 },
  delete: { color: colors.danger, fontWeight: '700' },
  weekdays: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
  weekday: { borderWidth: 1, borderColor: colors.border, borderRadius: 8, paddingHorizontal: 8, paddingVertical: 7 },
  weekdayText: { color: colors.mutedText, fontSize: 12 },
  timeInput: { flex: 1, textAlign: 'center', fontVariant: ['tabular-nums'] },
});
