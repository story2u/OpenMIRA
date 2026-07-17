import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { ActivityIndicator, Pressable, SafeAreaView, StyleSheet, Text, View } from 'react-native';

import { useSession } from '../../src/auth/SessionProvider';
import { useI18n } from '../../src/i18n/I18nProvider';
import { colors } from '../../src/ui/theme';

export default function AccountScreen() {
  const router = useRouter();
  const { logout, state } = useSession();
  const { t } = useI18n();
  const [loggingOut, setLoggingOut] = useState(false);
  const [error, setError] = useState('');

  if (state.status !== 'authenticated') return null;

  async function submitLogout() {
    setLoggingOut(true);
    setError('');
    try {
      await logout();
    } catch {
      setError(t('account.logoutError'));
      setLoggingOut(false);
    }
  }

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.container}>
        <Text style={styles.eyebrow}>ACCOUNT</Text>
        <Text accessibilityRole="header" style={styles.title}>
          {t('account.greeting', { name: state.user.displayName })}
        </Text>
        <Text style={styles.description}>{t('account.sessionRestored')}</Text>

        <View style={styles.card}>
          <Text style={styles.cardLabel}>{t('account.email')}</Text>
          <Text style={styles.cardValue}>{state.user.email}</Text>
          <Text style={styles.cardMeta}>{t('account.serverTruth')}</Text>
        </View>

        <Pressable
          accessibilityRole="button"
          onPress={() => router.push('/settings' as Href)}
          style={styles.settingsButton}
        >
          <View style={{ flex: 1, gap: 3 }}>
            <Text style={styles.settingsButtonText}>{t('account.openSettings')}</Text>
            <Text style={styles.settingsButtonDetail}>{t('account.settingsDetail')}</Text>
          </View>
          <Text style={styles.settingsChevron}>›</Text>
        </Pressable>

        {__DEV__ ? (
          <Pressable accessibilityRole="button" onPress={() => router.push('/lab')} style={styles.secondaryButton}>
            <Text style={styles.secondaryButtonText}>{t('account.openLab')}</Text>
          </Pressable>
        ) : null}

        <Pressable
          accessibilityRole="button"
          accessibilityState={{ busy: loggingOut, disabled: loggingOut }}
          disabled={loggingOut}
          onPress={() => void submitLogout()}
          style={styles.logoutButton}
        >
          {loggingOut ? <ActivityIndicator color={colors.text} /> : null}
          <Text style={styles.logoutText}>
            {loggingOut ? t('account.loggingOut') : t('account.logout')}
          </Text>
        </Pressable>
        <View accessibilityLiveRegion="polite">
          {error ? <Text accessibilityRole="alert" style={styles.error}>{error}</Text> : null}
        </View>
      </View>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { flex: 1, padding: 24, gap: 16 },
  eyebrow: { marginTop: 24, color: colors.accent, fontSize: 12, fontWeight: '800', letterSpacing: 2 },
  title: { color: colors.text, fontSize: 30, fontWeight: '800' },
  description: { color: colors.mutedText, fontSize: 15, lineHeight: 23 },
  card: { marginTop: 14, gap: 8, borderRadius: 16, backgroundColor: colors.card, padding: 18 },
  cardLabel: { color: colors.mutedText, fontSize: 13 },
  cardValue: { color: colors.text, fontSize: 17, fontWeight: '700' },
  cardMeta: { marginTop: 8, color: colors.subtleText, fontSize: 13, lineHeight: 20 },
  secondaryButton: { alignItems: 'center', borderWidth: 1, borderColor: colors.border, borderRadius: 12, padding: 14 },
  secondaryButtonText: { color: colors.accent, fontWeight: '700' },
  settingsButton: { minHeight: 68, flexDirection: 'row', alignItems: 'center', gap: 10, borderWidth: 1, borderColor: colors.border, borderRadius: 14, backgroundColor: colors.card, padding: 15 },
  settingsButtonText: { color: colors.text, fontSize: 16, fontWeight: '800' },
  settingsButtonDetail: { color: colors.mutedText, fontSize: 12 },
  settingsChevron: { color: colors.accent, fontSize: 28 },
  logoutButton: { marginTop: 'auto', minHeight: 50, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 10, borderRadius: 12, backgroundColor: colors.button, padding: 14 },
  logoutText: { color: colors.text, fontSize: 16, fontWeight: '800' },
  error: { borderRadius: 12, backgroundColor: colors.errorBackground, color: colors.errorText, padding: 13 },
});
