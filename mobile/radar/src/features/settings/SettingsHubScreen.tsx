import { type Href, useRouter } from 'expo-router';
import { Text, View } from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import {
  NavigationRow,
  SectionTitle,
  SettingsCard,
  SettingsScaffold,
  StateNotice,
  settingsStyles,
} from './components';
import { useSettings } from './SettingsProvider';
import { initializeRadarDatabase } from '../../storage/database';
import { setTeachingOnboardingSeen } from '../../attention/signalAppetiteUiState';

export function SettingsHubScreen() {
  const router = useRouter();
  const { state: session } = useSession();
  const { t } = useI18n();
  const { retry, state } = useSettings();
  const user = session.status === 'authenticated' ? session.user : null;

  return (
    <SettingsScaffold title={t('settings.hub.title')} subtitle={t('settings.hub.subtitle')}>
      <SectionTitle>{t('settings.hub.account')}</SectionTitle>
      <SettingsCard>
        <View style={settingsStyles.rowBetween}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={settingsStyles.label}>{user?.displayName ?? '—'}</Text>
            <Text style={settingsStyles.body}>{user?.email ?? ''}</Text>
          </View>
          <Text style={{ color: '#2dd4bf', fontWeight: '800' }}>{t('settings.hub.ownerIsolation')}</Text>
        </View>
      </SettingsCard>

      <SectionTitle>{t('settings.hub.subscription')}</SectionTitle>
      <SettingsCard>
        <NavigationRow
          detail={t('settings.hub.subscriptionDetail')}
          label={t('settings.hub.subscriptionLabel')}
          onPress={() => router.push('/settings/subscription' as Href)}
        />
      </SettingsCard>

      <SectionTitle>{t('settings.hub.connections')}</SectionTitle>
      <SettingsCard>
        <NavigationRow
          detail={t('settings.hub.telegramDetail')}
          label={t('settings.hub.telegram')}
          onPress={() => router.push('/settings/telegram' as Href)}
        />
        <View style={settingsStyles.rowBetween}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={settingsStyles.label}>{t('settings.hub.wecom')}</Text>
            <Text style={settingsStyles.body}>
              {state.bundle?.capabilities.wecomUserBindingAvailable
                ? t('settings.hub.wecomUserBinding')
                : t('settings.hub.wecomAdmin')}
            </Text>
          </View>
          <Text style={settingsStyles.body}>{t('settings.hub.noFakeConnection')}</Text>
        </View>
      </SettingsCard>

      <SectionTitle>{t('settings.hub.automation')}</SectionTitle>
      {state.bundle ? (
        <SettingsCard>
          <NavigationRow
            detail={t('settings.hub.detectionDetail')}
            label={t('settings.hub.detection')}
            onPress={() => router.push('/settings/detection' as Href)}
            value={t('settings.hub.keywordCount', { count: state.bundle.detection.keywords.length })}
          />
          <NavigationRow
            detail={t('settings.hub.scheduleDetail')}
            label={t('settings.hub.schedule')}
            onPress={() => router.push('/settings/work-schedule' as Href)}
            value={state.bundle.workSchedule.timezone}
          />
          <NavigationRow
            detail={t('settings.hub.notificationsDetail')}
            label={t('settings.hub.notifications')}
            onPress={() => router.push('/settings/notifications' as Href)}
            value={state.bundle.capabilities.pushAvailable
              ? t('settings.hub.pushAvailable')
              : t('settings.hub.pushPending')}
          />
        </SettingsCard>
      ) : (
        <StateNotice error={state.loadError} loading={state.isLoading} onRetry={retry} />
      )}

      <SectionTitle>{t('settings.hub.signalAppetite')}</SectionTitle>
      <SettingsCard>
        <NavigationRow
          detail={t('settings.hub.replayTeachingDetail')}
          label={t('settings.hub.replayTeaching')}
          onPress={() => {
            if (!user) return;
            void initializeRadarDatabase().then(async (database) => {
              await setTeachingOnboardingSeen(database, user.id, false);
              router.push('/teaching' as Href);
            });
          }}
        />
      </SettingsCard>
    </SettingsScaffold>
  );
}
