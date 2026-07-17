import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { ActivityIndicator, Pressable, SafeAreaView, StyleSheet, Text, View } from 'react-native';

import { sessionUnavailableMessage, useSession } from '../../src/auth/SessionProvider';
import OpportunityDetailScreen from '../../src/features/opportunity/OpportunityDetailScreen';
import { useI18n } from '../../src/i18n/I18nProvider';
import { opportunityIdFromRoute } from '../../src/navigation/returnTo';
import { colors } from '../../src/ui/theme';

export default function OpportunityRoute() {
  const router = useRouter();
  const params = useLocalSearchParams<{ id?: string | string[] }>();
  const { retry, state } = useSession();
  const { t } = useI18n();
  const opportunityId = opportunityIdFromRoute(params.id);

  if (!opportunityId) {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.container}>
          <Text accessibilityRole="header" style={styles.title}>{t('opportunity.invalidLink.title')}</Text>
          <Text accessibilityRole="alert" style={styles.description}>{t('opportunity.invalidLink.message')}</Text>
          <Pressable accessibilityRole="button" onPress={() => router.replace('/(tabs)/dashboard')} style={styles.button}>
            <Text style={styles.buttonText}>{t('opportunity.backToDashboard')}</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    );
  }
  if (state.status === 'loading') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.container}>
          <ActivityIndicator color={colors.accent} size="large" />
          <Text style={styles.description}>{t('app.starting')}</Text>
        </View>
      </SafeAreaView>
    );
  }
  if (state.status === 'unavailable') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.container}>
          <Text accessibilityRole="header" style={styles.title}>{t('opportunity.openError.title')}</Text>
          <Text accessibilityRole="alert" style={styles.description}>
            {sessionUnavailableMessage(state.reason, t)}
          </Text>
          <Pressable accessibilityRole="button" onPress={retry} style={styles.button}>
            <Text style={styles.buttonText}>{t('common.retry')}</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    );
  }
  if (state.status !== 'authenticated') {
    return (
      <Redirect
        href={{ pathname: '/login', params: { returnTo: `/opportunity/${opportunityId}` } }}
      />
    );
  }
  return <OpportunityDetailScreen opportunityId={opportunityId} />;
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 16, padding: 28 },
  title: { color: colors.text, fontSize: 24, fontWeight: '800', textAlign: 'center' },
  description: { color: colors.mutedText, fontSize: 14, lineHeight: 21, textAlign: 'center' },
  button: { borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 22, paddingVertical: 12 },
  buttonText: { color: colors.text, fontSize: 14, fontWeight: '800' },
});
