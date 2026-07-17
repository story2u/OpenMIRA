import { useEffect, useState } from 'react';
import {
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { colors } from '../../ui/theme';

const positiveReasons = [
  'important_customer',
  'purchase_intent',
  'needs_reply',
  'suitable_job',
  'current_project',
  'deadline',
  'industry_signal',
] as const;
const negativeReasons = [
  'advertising',
  'training',
  'duplicate',
  'unrelated_chat',
  'expired',
  'untrusted_source',
] as const;

export function TeachingReasonSheet({
  label,
  onClose,
  onSave,
  reduceMotion,
  visible,
}: {
  label: 'positive' | 'negative';
  onClose(): void;
  onSave(reasons: readonly string[], freeform: string | null): void;
  reduceMotion: boolean;
  visible: boolean;
}) {
  const { t } = useI18n();
  const [selected, setSelected] = useState<string[]>([]);
  const [freeform, setFreeform] = useState('');
  useEffect(() => {
    if (!visible) {
      setSelected([]);
      setFreeform('');
    }
  }, [visible]);
  const reasons = label === 'positive' ? positiveReasons : negativeReasons;
  return (
    <Modal animationType={reduceMotion ? 'fade' : 'slide'} onRequestClose={onClose} transparent visible={visible}>
      <Pressable onPress={onClose} style={styles.backdrop}>
        <Pressable style={styles.sheet}>
          <View style={styles.handle} />
          <Text accessibilityRole="header" style={styles.title}>
            {t(label === 'positive' ? 'teaching.reason.positiveTitle' : 'teaching.reason.negativeTitle')}
          </Text>
          <Text style={styles.detail}>{t('teaching.reason.optional')}</Text>
          <ScrollView contentContainerStyle={styles.chips} horizontal={false}>
            {reasons.map((reason) => {
              const active = selected.includes(reason);
              const key = `teaching.reason.${reason}` as MessageKey;
              return (
                <Pressable
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: active }}
                  key={reason}
                  onPress={() => setSelected((current) => active
                    ? current.filter((item) => item !== reason)
                    : [...current, reason])}
                  style={[styles.chip, active && styles.chipActive]}
                >
                  <Text style={[styles.chipText, active && styles.chipTextActive]}>
                    {active ? '✓ ' : ''}{t(key)}
                  </Text>
                </Pressable>
              );
            })}
          </ScrollView>
          <TextInput
            accessibilityLabel={t('teaching.reason.freeform')}
            maxLength={1_000}
            multiline
            onChangeText={setFreeform}
            placeholder={t('teaching.reason.freeform')}
            placeholderTextColor={colors.placeholder}
            style={styles.input}
            value={freeform}
          />
          <Pressable
            accessibilityRole="button"
            onPress={() => {
              onSave(selected, freeform.trim() || null);
              onClose();
            }}
            style={styles.save}
          >
            <Text style={styles.saveText}>{t('teaching.reason.save')}</Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(2, 8, 23, 0.72)' },
  sheet: { maxHeight: '86%', gap: 12, borderTopLeftRadius: 28, borderTopRightRadius: 28, backgroundColor: colors.card, padding: 22, paddingBottom: 34 },
  handle: { width: 38, height: 4, alignSelf: 'center', borderRadius: 99, backgroundColor: colors.border },
  title: { color: colors.text, fontSize: 23, lineHeight: 30, fontWeight: '900' },
  detail: { color: colors.mutedText, fontSize: 13, lineHeight: 19 },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8, paddingVertical: 4 },
  chip: { minHeight: 40, justifyContent: 'center', borderRadius: 999, borderWidth: 1, borderColor: colors.border, paddingHorizontal: 13 },
  chipActive: { borderColor: colors.accent, backgroundColor: colors.accentMuted },
  chipText: { color: colors.mutedText, fontSize: 13, fontWeight: '700' },
  chipTextActive: { color: '#a7f3d0' },
  input: { minHeight: 76, borderRadius: 15, borderWidth: 1, borderColor: colors.border, color: colors.text, padding: 13, textAlignVertical: 'top' },
  save: { minHeight: 50, alignItems: 'center', justifyContent: 'center', borderRadius: 15, backgroundColor: colors.button },
  saveText: { color: colors.text, fontSize: 15, fontWeight: '900' },
});
