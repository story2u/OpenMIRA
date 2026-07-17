import { type Href, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { ActivityIndicator, Pressable, SafeAreaView, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import { IntentMapCanvas } from '../intent-map/IntentMapCanvas';
import { useIntentMap } from '../intent-map/useIntentMap';
import { colors } from '../../ui/theme';

export default function HomeScreen() {
  const router = useRouter();
  const { state } = useSession();
  const { t } = useI18n();
  const { loading, model } = useIntentMap();
  if (state.status !== 'authenticated') return null;
  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container} showsVerticalScrollIndicator={false}>
        <Text style={styles.greeting}>{t('home.greeting', { name: state.user.displayName })}</Text>
        <Text accessibilityRole="header" style={styles.title}>{t('home.title')}</Text>
        <View style={styles.mapCard}>
          <View style={styles.mapHeader}>
            <View style={styles.mapHeaderCopy}>
              <Text style={styles.eyebrow}>{t('intentMap.title')}</Text>
              <Text style={styles.appetiteName}>{model?.preference?.title ?? t('home.appetite.empty')}</Text>
            </View>
            <Pressable accessibilityRole="button" onPress={() => router.push('/intent-map' as Href)}>
              <Text style={styles.mapLink}>{t('home.appetite.openMap')} ›</Text>
            </Pressable>
          </View>
          {loading ? <ActivityIndicator color={colors.accent} style={styles.mapLoading} /> : null}
          {!loading && model?.preference ? (
            <IntentMapCanvas compact model={model} onNodePress={() => router.push('/intent-map' as Href)} t={t} />
          ) : null}
          {!loading && !model?.preference ? (
            <View style={styles.emptyMap}>
              <Text style={styles.emptyGlyph}>✦</Text>
              <Text style={styles.emptyDetail}>{t('home.appetite.emptyDetail')}</Text>
            </View>
          ) : null}
          <Pressable accessibilityRole="button" onPress={() => router.push('/teaching' as Href)} style={styles.teachButton}>
            <Text style={styles.teachText}>{t('home.appetite.teach')}</Text>
          </Pressable>
        </View>
        <View style={styles.flowCard}>
          <Text style={styles.flowTitle}>{t('home.flow.title')}</Text>
          <Text style={styles.flowTotal}>{t('home.flow.total', { count: model?.stats.total ?? 0 })}</Text>
          <View style={styles.statsGrid}>
            {([
              ['immediate', model?.stats.immediate ?? 0, '#fbbf24'],
              ['inbox', model?.stats.inbox ?? 0, '#38bdf8'],
              ['digest', model?.stats.digest ?? 0, '#818cf8'],
              ['suppress', model?.stats.suppress ?? 0, '#94a3b8'],
            ] as const).map(([key, count, color]) => (
              <View key={key} style={styles.stat}>
                <Text style={[styles.statCount, { color }]}>{count}</Text>
                <Text style={styles.statLabel}>{t(`home.flow.${key}`)}</Text>
              </View>
            ))}
          </View>
        </View>
      </ScrollView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: colors.background },
  container: { gap: 18, padding: 20, paddingBottom: 38 },
  greeting: { marginTop: 10, color: colors.accent, fontSize: 12, fontWeight: '800' },
  title: { maxWidth: 330, color: colors.text, fontSize: 29, lineHeight: 37, fontWeight: '900' },
  mapCard: { overflow: 'hidden', gap: 12, borderRadius: 26, borderWidth: 1, borderColor: '#24435d', backgroundColor: colors.card, padding: 15 },
  mapHeader: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  mapHeaderCopy: { flex: 1, gap: 3 },
  eyebrow: { color: colors.accent, fontSize: 10, fontWeight: '900', letterSpacing: 1.2 },
  appetiteName: { color: colors.text, fontSize: 17, fontWeight: '900' },
  mapLink: { color: colors.accent, fontSize: 11, fontWeight: '800' },
  mapLoading: { height: 180 },
  emptyMap: { minHeight: 180, alignItems: 'center', justifyContent: 'center', gap: 12, borderRadius: 20, backgroundColor: '#091827', padding: 24 },
  emptyGlyph: { color: colors.accent, fontSize: 32 },
  emptyDetail: { color: colors.mutedText, fontSize: 13, lineHeight: 20, textAlign: 'center' },
  teachButton: { minHeight: 46, alignItems: 'center', justifyContent: 'center', borderRadius: 14, backgroundColor: colors.button },
  teachText: { color: colors.text, fontSize: 14, fontWeight: '900' },
  flowCard: { gap: 6, borderRadius: 24, backgroundColor: colors.card, padding: 17 },
  flowTitle: { color: colors.text, fontSize: 20, fontWeight: '900' },
  flowTotal: { color: colors.mutedText, fontSize: 12 },
  statsGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 9, marginTop: 10 },
  stat: { width: '48%', flexGrow: 1, minHeight: 82, borderRadius: 16, backgroundColor: '#0a1a2b', padding: 12 },
  statCount: { fontSize: 25, fontWeight: '900', fontVariant: ['tabular-nums'] },
  statLabel: { marginTop: 3, color: colors.mutedText, fontSize: 11, lineHeight: 16 },
});
