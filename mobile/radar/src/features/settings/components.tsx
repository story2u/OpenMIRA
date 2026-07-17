import { useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import type { ReactNode } from 'react';
import {
  ActivityIndicator,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import { colors } from '../../ui/theme';

export function SettingsScaffold({
  children,
  subtitle,
  title,
}: {
  children: ReactNode;
  subtitle?: string;
  title: string;
}) {
  const router = useRouter();
  const { t } = useI18n();
  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
        <View style={styles.headerRow}>
          <Pressable
            accessibilityLabel={t('common.back')}
            accessibilityRole="button"
            hitSlop={12}
            onPress={() => router.back()}
            style={styles.backButton}
          >
            <Text style={styles.backText}>‹</Text>
          </Pressable>
          <View style={styles.headerCopy}>
            <Text accessibilityRole="header" style={styles.title}>{title}</Text>
            {subtitle ? <Text style={styles.subtitle}>{subtitle}</Text> : null}
          </View>
        </View>
        {children}
      </ScrollView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

export function SettingsCard({ children }: { children: ReactNode }) {
  return <View style={styles.card}>{children}</View>;
}

export function SectionTitle({ children }: { children: ReactNode }) {
  return <Text style={styles.sectionTitle}>{children}</Text>;
}

export function ActionButton({
  busy = false,
  disabled = false,
  label,
  onPress,
  tone = 'primary',
}: {
  busy?: boolean;
  disabled?: boolean;
  label: string;
  onPress(): void;
  tone?: 'primary' | 'secondary' | 'danger';
}) {
  const isDisabled = disabled || busy;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ busy, disabled: isDisabled }}
      disabled={isDisabled}
      onPress={onPress}
      style={[
        styles.actionButton,
        tone === 'secondary' && styles.secondaryButton,
        tone === 'danger' && styles.dangerButton,
        isDisabled && styles.disabled,
      ]}
    >
      {busy ? <ActivityIndicator color={colors.text} /> : null}
      <Text style={styles.actionText}>{label}</Text>
    </Pressable>
  );
}

export function StateNotice({
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
      <SettingsCard>
        <View style={styles.inlineState}>
          <ActivityIndicator color={colors.accent} />
          <Text style={styles.mutedText}>{t('settings.loading')}</Text>
        </View>
      </SettingsCard>
    );
  }
  if (!error) return null;
  return (
    <SettingsCard>
      <Text accessibilityRole="alert" style={styles.errorText}>{error}</Text>
      <ActionButton label={t('common.retry')} onPress={onRetry} tone="secondary" />
    </SettingsCard>
  );
}

export function NavigationRow({
  detail,
  label,
  onPress,
  value,
}: {
  detail: string;
  label: string;
  onPress(): void;
  value?: string;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [styles.navigationRow, pressed && styles.pressed]}
    >
      <View style={styles.navigationCopy}>
        <Text style={styles.navigationLabel}>{label}</Text>
        <Text style={styles.navigationDetail}>{detail}</Text>
      </View>
      {value ? <Text style={styles.navigationValue}>{value}</Text> : null}
      <Text accessibilityElementsHidden style={styles.chevron}>›</Text>
    </Pressable>
  );
}

export const settingsStyles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  rowBetween: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12 },
  label: { color: colors.text, fontSize: 15, fontWeight: '700' },
  body: { color: colors.mutedText, fontSize: 13, lineHeight: 20 },
  input: {
    minHeight: 46,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 12,
    color: colors.text,
    paddingHorizontal: 13,
    paddingVertical: 10,
  },
  error: {
    borderRadius: 12,
    backgroundColor: colors.errorBackground,
    color: colors.errorText,
    padding: 12,
    lineHeight: 19,
  },
});

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { flexGrow: 1, gap: 16, padding: 20, paddingBottom: 48 },
  headerRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 12, marginTop: 8 },
  headerCopy: { flex: 1, gap: 5 },
  backButton: {
    width: 40,
    height: 40,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
  },
  backText: { color: colors.text, fontSize: 30, lineHeight: 32 },
  title: { color: colors.text, fontSize: 26, fontWeight: '800' },
  subtitle: { color: colors.mutedText, fontSize: 14, lineHeight: 21 },
  sectionTitle: { marginTop: 6, color: colors.subtleText, fontSize: 12, fontWeight: '800', letterSpacing: 1.2 },
  card: { gap: 13, borderRadius: 16, backgroundColor: colors.card, padding: 16 },
  actionButton: {
    minHeight: 48,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 9,
    borderRadius: 12,
    backgroundColor: colors.button,
    paddingHorizontal: 16,
  },
  secondaryButton: { borderWidth: 1, borderColor: colors.border, backgroundColor: 'transparent' },
  dangerButton: { backgroundColor: colors.errorBackground },
  disabled: { opacity: 0.55 },
  actionText: { color: colors.text, fontSize: 15, fontWeight: '800' },
  inlineState: { minHeight: 48, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 10 },
  mutedText: { color: colors.mutedText },
  errorText: { color: colors.errorText, lineHeight: 20 },
  navigationRow: {
    minHeight: 66,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.border,
    paddingVertical: 10,
  },
  navigationCopy: { flex: 1, gap: 4 },
  navigationLabel: { color: colors.text, fontSize: 16, fontWeight: '700' },
  navigationDetail: { color: colors.mutedText, fontSize: 12, lineHeight: 18 },
  navigationValue: { color: colors.accent, fontSize: 12, fontWeight: '700' },
  chevron: { color: colors.subtleText, fontSize: 26 },
  pressed: { opacity: 0.65 },
});
