import type {
  TelegramConnection,
  TelegramConnectionHealth,
  TelegramConnectionStatus,
  TelegramConnectionType,
} from '@story2u/radar-contracts/telegram';
import { ActivityIndicator, StyleSheet, Switch, Text, View } from 'react-native';

import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import { colors } from '../../ui/theme';
import {
  ActionButton,
  SectionTitle,
  SettingsCard,
  SettingsScaffold,
  settingsStyles,
} from './components';
import { useTelegramSettings } from './useTelegramSettings';

const connectionTypeLabelKeys: Record<TelegramConnectionType, MessageKey> = {
  bot_chat: 'settings.telegram.type.bot',
  business: 'settings.telegram.type.business',
  mtproto_qr: 'settings.telegram.type.qr',
};

const connectionStatusLabelKeys: Record<TelegramConnectionStatus, MessageKey> = {
  pending: 'settings.telegram.status.pending',
  connected: 'settings.telegram.status.connected',
  disabled: 'settings.telegram.status.disabled',
  error: 'settings.telegram.status.error',
  expired: 'settings.telegram.status.expired',
};

export function TelegramSettingsScreen() {
  const { expireSession } = useSession();
  const { t } = useI18n();
  const { retry, state, toggle } = useTelegramSettings(expireSession);

  return (
    <SettingsScaffold
      title={t('settings.telegram.title')}
      subtitle={t('settings.telegram.subtitle')}
    >
      <View style={styles.refreshRow}>
        {state.isLoading ? <ActivityIndicator color={colors.accent} /> : null}
        <ActionButton
          busy={state.isLoading}
          label={state.isLoading ? t('common.refreshing') : t('common.refresh')}
          onPress={retry}
          tone="secondary"
        />
      </View>

      {state.health ? <HealthCard health={state.health} /> : null}
      {state.error ? <Text accessibilityRole="alert" style={settingsStyles.error}>{state.error}</Text> : null}

      <SectionTitle>{t('settings.telegram.current')}</SectionTitle>
      {!state.isLoading && state.connections.length === 0 ? (
        <SettingsCard>
          <Text style={settingsStyles.label}>{t('settings.telegram.empty')}</Text>
          <Text style={settingsStyles.body}>{t('settings.telegram.emptyDetail')}</Text>
        </SettingsCard>
      ) : null}
      {state.connections.map((connection) => (
        <ConnectionCard
          actionId={state.actionId}
          connection={connection}
          key={connection.id}
          onToggle={(enabled) => void toggle(connection, enabled)}
        />
      ))}
    </SettingsScaffold>
  );
}

function HealthCard({ health }: { health: TelegramConnectionHealth }) {
  const { t } = useI18n();
  return (
    <SettingsCard>
      <Text style={settingsStyles.label}>{t('settings.telegram.health')}</Text>
      <HealthRow label={t('settings.telegram.mode')} value={health.mode === 'live' ? t('settings.telegram.modeLive') : t('settings.telegram.modeMock')} />
      <HealthRow
        label="Bot"
        value={health.botConfigured
          ? (health.botUsername ? `@${health.botUsername}` : t('common.configure'))
          : t('settings.telegram.adminNotConfigured')}
      />
      <HealthRow label={t('settings.telegram.business')} value={health.businessAvailable ? t('common.available') : t('settings.telegram.adminNotConfigured')} />
      <HealthRow label={t('settings.telegram.qr')} value={health.mtprotoQrAvailable ? t('common.available') : t('settings.telegram.adminNotConfigured')} />
      {health.legacyMonitoringActive ? (
        <Text style={styles.notice}>{t('settings.telegram.legacy', { count: health.legacyActiveSourceCount })}</Text>
      ) : null}
      {health.message ? <Text style={styles.notice}>{health.message}</Text> : null}
    </SettingsCard>
  );
}

function HealthRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={settingsStyles.rowBetween}>
      <Text style={settingsStyles.body}>{label}</Text>
      <Text style={styles.healthValue}>{value}</Text>
    </View>
  );
}

function ConnectionCard({
  actionId,
  connection,
  onToggle,
}: {
  actionId: string | null;
  connection: TelegramConnection;
  onToggle(enabled: boolean): void;
}) {
  const { t } = useI18n();
  const busy = actionId === connection.id;
  return (
    <SettingsCard>
      <View style={settingsStyles.rowBetween}>
        <View style={{ flex: 1, gap: 4 }}>
          <Text style={settingsStyles.label}>{connection.label}</Text>
          <Text style={settingsStyles.body}>
            {t(connectionTypeLabelKeys[connection.connectionType])} · {t(connectionStatusLabelKeys[connection.status])}
          </Text>
        </View>
        {busy ? <ActivityIndicator color={colors.accent} /> : null}
        <Switch
          accessibilityLabel={t('settings.telegram.toggle', {
            action: connection.enabled ? t('common.disable') : t('common.enable'),
            name: connection.label,
          })}
          disabled={busy || actionId !== null}
          onValueChange={onToggle}
          trackColor={{ false: colors.border, true: colors.button }}
          value={connection.enabled}
        />
      </View>
      {connection.status === 'error' || connection.lastError ? (
        <Text style={styles.warning}>{t('settings.telegram.connectionError')}</Text>
      ) : null}
      <View style={styles.divider} />
      <Text style={settingsStyles.label}>{t('settings.telegram.sources')}</Text>
      {connection.sources.length === 0 ? <Text style={settingsStyles.body}>{t('settings.telegram.noSources')}</Text> : null}
      {connection.sources.map((source) => (
        <View key={source.id} style={styles.source}>
          <View style={{ flex: 1, gap: 3 }}>
            <Text style={styles.sourceName}>{source.displayName}</Text>
            <Text style={settingsStyles.body}>
              {t(source.sourceType === 'channel'
                ? 'settings.telegram.source.channel'
                : source.sourceType === 'group'
                  ? 'settings.telegram.source.group'
                  : 'settings.telegram.source.private')}
              {source.username ? ` · @${source.username}` : ''}
            </Text>
          </View>
          <Text style={
            source.quotaPaused
              ? styles.quotaPaused
              : connection.enabled && source.enabled
                ? styles.activeSource
                : styles.inactiveSource
          }>
            {source.quotaPaused
              ? t('settings.telegram.source.quotaPaused')
              : !connection.enabled
                ? t('settings.telegram.source.connectionDisabled')
                : source.enabled
                  ? t('settings.telegram.source.active')
                  : t('settings.telegram.source.disabled')}
          </Text>
          {source.quotaPaused && source.quotaReason ? (
            <Text style={styles.quotaReason}>{source.quotaReason}</Text>
          ) : null}
        </View>
      ))}
    </SettingsCard>
  );
}

const styles = StyleSheet.create({
  refreshRow: { minHeight: 48, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 10 },
  healthValue: { maxWidth: '60%', color: colors.text, textAlign: 'right', fontSize: 13, fontWeight: '700' },
  notice: { borderRadius: 10, backgroundColor: colors.noticeBackground, color: colors.noticeText, padding: 11, lineHeight: 18 },
  warning: { borderRadius: 10, backgroundColor: colors.noticeBackground, color: colors.noticeText, padding: 11, lineHeight: 18 },
  divider: { height: StyleSheet.hairlineWidth, backgroundColor: colors.border },
  source: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: 9, borderRadius: 11, backgroundColor: colors.background, padding: 12 },
  sourceName: { color: colors.text, fontSize: 14, fontWeight: '700' },
  activeSource: { color: colors.success, fontSize: 12, fontWeight: '800' },
  inactiveSource: { color: colors.mutedText, fontSize: 12, fontWeight: '800' },
  quotaPaused: { color: colors.warning, fontSize: 12, fontWeight: '800' },
  quotaReason: { width: '100%', color: colors.noticeText, fontSize: 12, lineHeight: 18 },
});
