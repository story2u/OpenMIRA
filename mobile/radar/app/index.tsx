import { Redirect } from 'expo-router';
import { ActivityIndicator, Pressable, SafeAreaView, StyleSheet, Text, View } from 'react-native';

import { sessionUnavailableMessage, useSession } from '../src/auth/SessionProvider';
import { useI18n } from '../src/i18n/I18nProvider';
import { colors } from '../src/ui/theme';

export default function EntryScreen() {
  const { state, retry } = useSession();
  const { t } = useI18n();

  if (state.status === 'authenticated') return <Redirect href="/(tabs)/dashboard" />;
  if (state.status === 'anonymous' || state.status === 'requires-login') {
    return <Redirect href="/login" />;
  }

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.container}>
        {state.status === 'loading' ? (
          <>
            <ActivityIndicator color={colors.accent} size="large" />
            <Text style={styles.message}>{t('app.starting')}</Text>
          </>
        ) : (
          <>
            <Text accessibilityRole="header" style={styles.title}>{t('app.startError.title')}</Text>
            <Text accessibilityLiveRegion="polite" style={styles.message}>
              {sessionUnavailableMessage(state.reason, t)}
            </Text>
            <Pressable accessibilityRole="button" onPress={retry} style={styles.button}>
              <Text style={styles.buttonText}>{t('common.retry')}</Text>
            </Pressable>
          </>
        )}
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 18, padding: 28 },
  title: { color: colors.text, fontSize: 26, fontWeight: '800' },
  message: { color: colors.mutedText, fontSize: 15, lineHeight: 22, textAlign: 'center' },
  button: { borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 24, paddingVertical: 13 },
  buttonText: { color: colors.text, fontSize: 16, fontWeight: '700' },
});
