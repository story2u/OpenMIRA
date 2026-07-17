import type { NotificationSettings } from '@story2u/radar-contracts/settings';
import { useState } from 'react';
import { Linking, Switch, Text, View } from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import { colors } from '../../ui/theme';
import {
  ActionButton,
  SettingsCard,
  SettingsScaffold,
  StateNotice,
  settingsStyles,
} from './components';
import { useSettings } from './SettingsProvider';

export function NotificationSettingsScreen() {
  const settings = useSettings();
  const { t } = useI18n();
  const bundle = settings.state.bundle;
  if (!bundle) {
    return (
      <SettingsScaffold title={t('settings.notifications.title')}>
        <StateNotice
          error={settings.state.loadError}
          loading={settings.state.isLoading}
          onRetry={settings.retry}
        />
      </SettingsScaffold>
    );
  }
  return (
    <NotificationEditor
      initial={bundle.notifications}
      key={JSON.stringify(bundle.notifications)}
      pushAvailable={bundle.capabilities.pushAvailable}
    />
  );
}

function NotificationEditor({
  initial,
  pushAvailable,
}: {
  initial: NotificationSettings;
  pushAvailable: boolean;
}) {
  const { saveNotifications, state } = useSettings();
  const session = useSession();
  const { t } = useI18n();
  const [prefs, setPrefs] = useState(initial);
  const [showSaveError, setShowSaveError] = useState(false);
  const saving = state.busyAction === 'notifications';

  function toggle(key: keyof NotificationSettings, value: boolean) {
    setPrefs((current) => ({ ...current, [key]: value }));
  }

  async function save() {
    setShowSaveError(false);
    const saved = await saveNotifications(prefs);
    if (!saved) {
      setPrefs(initial);
      setShowSaveError(true);
    }
  }

  return (
    <SettingsScaffold title={t('settings.notifications.title')} subtitle={t('settings.notifications.subtitle')}>
      {!pushAvailable ? (
        <SettingsCard>
          <Text style={settingsStyles.label}>{t('settings.notifications.pushUnavailable')}</Text>
          <Text style={settingsStyles.body}>{t('settings.notifications.pushUnavailableDetail')}</Text>
        </SettingsCard>
      ) : (
        <SettingsCard>
          <Text style={settingsStyles.label}>{t('settings.notifications.pushSyncTitle')}</Text>
          <Text style={settingsStyles.body}>{t(
            `settings.notifications.pushState.${session.pushEnrollmentState}`,
          )}</Text>
          {session.pushEnrollmentState === 'active' ? (
            <ActionButton
              label={t('settings.notifications.pushDisable')}
              onPress={() => void session.disablePush()}
              tone="secondary"
            />
          ) : session.pushEnrollmentState === 'denied' ? (
            <ActionButton
              label={t('settings.notifications.pushOpenSettings')}
              onPress={() => void Linking.openSettings()}
              tone="secondary"
            />
          ) : (
            <ActionButton
              busy={session.pushEnrollmentState === 'registering'}
              label={t('settings.notifications.pushEnable')}
              onPress={() => void session.enablePush()}
            />
          )}
        </SettingsCard>
      )}

      <SettingsCard>
        <ToggleRow
          detail={t('settings.notifications.newOpportunityDetail')}
          label={t('settings.notifications.newOpportunity')}
          onValueChange={(value) => toggle('newOpportunityEnabled', value)}
          value={prefs.newOpportunityEnabled}
        />
        <ToggleRow
          detail={t('settings.notifications.aiRepliedDetail')}
          label={t('settings.notifications.aiReplied')}
          onValueChange={(value) => toggle('aiRepliedEnabled', value)}
          value={prefs.aiRepliedEnabled}
        />
        <ToggleRow
          detail={t('settings.notifications.digestDetail')}
          label={t('settings.notifications.digest')}
          onValueChange={(value) => toggle('dailyDigestEnabled', value)}
          value={prefs.dailyDigestEnabled}
        />
        <ToggleRow
          detail={t('settings.notifications.urgentOnlyDetail')}
          label={t('settings.notifications.urgentOnly')}
          onValueChange={(value) => toggle('urgentOnly', value)}
          value={prefs.urgentOnly}
        />
      </SettingsCard>

      {showSaveError && state.saveError ? (
        <Text accessibilityRole="alert" style={settingsStyles.error}>{state.saveError}</Text>
      ) : null}
      <ActionButton busy={saving} label={saving ? t('common.saving') : t('settings.notifications.save')} onPress={() => void save()} />
    </SettingsScaffold>
  );
}

function ToggleRow({
  detail,
  label,
  onValueChange,
  value,
}: {
  detail: string;
  label: string;
  onValueChange(value: boolean): void;
  value: boolean;
}) {
  return (
    <View style={settingsStyles.rowBetween}>
      <View style={{ flex: 1, gap: 4 }}>
        <Text style={settingsStyles.label}>{label}</Text>
        <Text style={settingsStyles.body}>{detail}</Text>
      </View>
      <Switch
        accessibilityLabel={label}
        onValueChange={onValueChange}
        trackColor={{ false: colors.border, true: colors.button }}
        value={value}
      />
    </View>
  );
}
