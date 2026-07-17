import type {
  BillingStore,
  PlanCode,
  SubscriptionCatalogPlan,
  SubscriptionUsage,
} from '@story2u/radar-contracts/subscriptions';
import { useMemo } from 'react';
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import type { MobileBillingPackage } from '../../billing/revenueCat';
import { useSession } from '../../auth/SessionProvider';
import { useI18n } from '../../i18n/I18nProvider';
import type { MessageKey } from '../../i18n/catalog';
import type { Translator } from '../../i18n/core';
import { colors } from '../../ui/theme';
import {
  ActionButton,
  SectionTitle,
  SettingsCard,
  SettingsScaffold,
  settingsStyles,
} from '../settings/components';
import { useSubscription } from './useSubscription';

const planNames: Record<PlanCode, string> = {
  free: 'Free',
  plus: 'Plus',
  pro: 'Pro',
  max: 'Max',
};
const storeNames: Partial<Record<BillingStore, string>> = {
  app_store: 'Apple App Store',
  play_store: 'Google Play',
  paddle: 'Web / Paddle',
};
const subscriptionStatusKeys: Record<SubscriptionUsage['subscriptionStatus'], MessageKey> = {
  active: 'subscription.status.active',
  trialing: 'subscription.status.trialing',
  past_due: 'subscription.status.pastDue',
  canceled: 'subscription.status.canceled',
  inactive: 'subscription.status.inactive',
};

function storeName(store: BillingStore, t: Translator) {
  if (store === 'test_store') return t('subscription.store.test');
  if (store === 'unknown') return t('subscription.store.unknown');
  return storeNames[store] ?? t('subscription.store.unknown');
}

export function SubscriptionScreen() {
  const { expireSession, state: session } = useSession();
  if (session.status !== 'authenticated') return null;
  return <AuthenticatedSubscriptionScreen expireSession={expireSession} userId={session.user.id} />;
}

function AuthenticatedSubscriptionScreen({
  expireSession,
  userId,
}: {
  expireSession(): Promise<void>;
  userId: string;
}) {
  const subscription = useSubscription(userId, expireSession);
  const { t } = useI18n();
  const { state } = subscription;
  const packagesById = useMemo(() => new Map(
    state.billing?.status === 'ready'
      ? state.billing.packages.map((item) => [item.identifier, item] as const)
      : [],
  ), [state.billing]);

  return (
    <SettingsScaffold
      title={t('subscription.title')}
      subtitle={t('subscription.subtitle')}
    >
      <View style={styles.toolbar}>
        {state.isLoading ? <ActivityIndicator color={colors.accent} /> : null}
        <ActionButton
          busy={state.isRestoring}
          disabled={state.billing?.status !== 'ready' || state.busyPackageId !== null}
          label={state.isRestoring ? t('subscription.restoring') : t('subscription.restore')}
          onPress={() => void subscription.restore()}
          tone="secondary"
        />
        <ActionButton
          busy={state.isLoading}
          disabled={state.busyPackageId !== null || state.isRestoring}
          label={state.isLoading ? t('common.refreshing') : t('common.refresh')}
          onPress={subscription.retry}
          tone="secondary"
        />
      </View>

      {state.error && !state.usage ? (
        <SettingsCard>
          <Text accessibilityRole="alert" style={settingsStyles.error}>{state.error}</Text>
          <ActionButton label={t('common.retry')} onPress={subscription.retry} tone="secondary" />
        </SettingsCard>
      ) : null}
      {state.usage ? (
        <>
          <CurrentPlanCard
            onOpenManagement={() => void subscription.openManagement()}
            usage={state.usage}
            managementInstruction={state.management?.instruction ?? ''}
            managementReady={Boolean(
              state.management?.canOpenInCurrentClient && state.management.managementUrl,
            )}
          />
          <SubscriptionNotices usage={state.usage} />
          {state.message ? <Text style={styles.message}>{state.message}</Text> : null}
          {state.error ? <Text accessibilityRole="alert" style={settingsStyles.error}>{state.error}</Text> : null}
          <UsageSection usage={state.usage} />
          <SectionTitle>{t('subscription.comparison')}</SectionTitle>
          <IntervalSelector
            interval={state.interval}
            onSelect={subscription.selectInterval}
          />
          <BillingNotice status={state.billing?.status ?? null} />
          {state.catalog.map((plan) => {
            const item = packagesById.get(`${plan.planCode}_${state.interval}`);
            return (
              <PlanCard
                busyPackageId={state.busyPackageId}
                currentPlan={state.usage!.planCode}
                item={item}
                key={plan.planCode}
                onPurchase={(selected) => void subscription.purchase(selected)}
                plan={plan}
                restoring={state.isRestoring}
              />
            );
          })}
          {state.usage.planCode !== 'free' ? (
            <Text style={styles.notice}>
              {t('subscription.existing')}
            </Text>
          ) : null}
        </>
      ) : null}
    </SettingsScaffold>
  );
}

function CurrentPlanCard({
  managementInstruction,
  managementReady,
  onOpenManagement,
  usage,
}: {
  managementInstruction: string;
  managementReady: boolean;
  onOpenManagement(): void;
  usage: SubscriptionUsage;
}) {
  const { locale, t } = useI18n();
  const periodEnd = new Intl.DateTimeFormat(locale, { dateStyle: 'medium' })
    .format(new Date(usage.usagePeriodEnd));
  return (
    <SettingsCard>
      <View style={settingsStyles.rowBetween}>
        <View style={styles.flexCopy}>
          <Text style={styles.eyebrow}>{t('subscription.current')}</Text>
          <Text style={styles.planTitle}>{planNames[usage.planCode]}</Text>
          <Text style={settingsStyles.body}>
            {t('subscription.periodEnd', { date: periodEnd })}
            {usage.effectiveStore ? ` · ${storeName(usage.effectiveStore, t)}` : ''}
          </Text>
        </View>
        <View style={styles.statusBadge}>
          <Text style={styles.statusBadgeText}>{t(subscriptionStatusKeys[usage.subscriptionStatus])}</Text>
        </View>
      </View>
      {usage.planCode !== 'free' ? (
        <ActionButton
          disabled={!managementReady}
          label={managementReady ? t('subscription.manage') : t('subscription.manageOriginal')}
          onPress={onOpenManagement}
          tone="secondary"
        />
      ) : null}
      {!managementReady && usage.planCode !== 'free' && managementInstruction ? (
        <Text style={settingsStyles.body}>{managementInstruction}</Text>
      ) : null}
    </SettingsCard>
  );
}

function SubscriptionNotices({ usage }: { usage: SubscriptionUsage }) {
  const { t } = useI18n();
  return (
    <>
      {usage.multipleActiveSubscriptions ? (
        <Text style={styles.warning}>
          {t('subscription.notice.multiple')}
        </Text>
      ) : null}
      {usage.billingIssue ? (
        <Text style={styles.errorNotice}>
          {t('subscription.notice.billingIssue')}
        </Text>
      ) : null}
      {usage.cancelAtPeriodEnd ? (
        <Text style={styles.notice}>{t('subscription.notice.cancelled')}</Text>
      ) : null}
    </>
  );
}

function UsageSection({ usage }: { usage: SubscriptionUsage }) {
  const { locale, t } = useI18n();
  const allocated = usage.aiAnalysesConsumed + usage.aiAnalysesReserved;
  return (
    <>
      <SectionTitle>{t('subscription.usage.title')}</SectionTitle>
      <SettingsCard>
        <UsageRow
          detail={t('subscription.usage.aiDetail', {
            consumed: usage.aiAnalysesConsumed.toLocaleString(locale),
            reserved: usage.aiAnalysesReserved.toLocaleString(locale),
          })}
          label={t('subscription.usage.ai')}
          limit={usage.entitlements.piAgentAnalysisMonthlyLimit}
          value={allocated}
        />
        <UsageRow
          detail={t('subscription.usage.groupsDetail', {
            telegram: usage.telegramGroupsUsed.toLocaleString(locale),
            wecom: usage.wecomGroupsUsed.toLocaleString(locale),
          })}
          label={t('subscription.usage.groups')}
          limit={usage.entitlements.combinedGroupLimit}
          value={usage.combinedGroupsUsed}
        />
      </SettingsCard>
    </>
  );
}

function UsageRow({
  detail,
  label,
  limit,
  value,
}: {
  detail: string;
  label: string;
  limit: number;
  value: number;
}) {
  const { locale } = useI18n();
  const percentage = limit > 0 ? Math.min((value / limit) * 100, 100) : 0;
  return (
    <View style={styles.usageBlock}>
      <View style={settingsStyles.rowBetween}>
        <Text style={settingsStyles.label}>{label}</Text>
        <Text style={styles.usageValue}>{value.toLocaleString(locale)} / {limit.toLocaleString(locale)}</Text>
      </View>
      <View
        accessibilityLabel={`${label} ${value} / ${limit}`}
        accessibilityRole="progressbar"
        accessibilityValue={{ min: 0, max: limit, now: value }}
        style={styles.progressTrack}
      >
        <View style={[styles.progressValue, { width: `${percentage}%` }]} />
      </View>
      <Text style={settingsStyles.body}>{detail}</Text>
    </View>
  );
}

function IntervalSelector({
  interval,
  onSelect,
}: {
  interval: 'monthly' | 'annual';
  onSelect(value: 'monthly' | 'annual'): void;
}) {
  const { t } = useI18n();
  const intervals = [
    ['monthly', t('subscription.interval.monthly')],
    ['annual', t('subscription.interval.annual')],
  ] as const;
  return (
    <View accessibilityRole="tablist" style={styles.intervalTabs}>
      {intervals.map(([value, label]) => (
        <Pressable
          accessibilityRole="tab"
          accessibilityState={{ selected: interval === value }}
          key={value}
          onPress={() => onSelect(value)}
          style={[styles.intervalTab, interval === value && styles.intervalTabSelected]}
        >
          <Text style={[styles.intervalText, interval === value && styles.intervalTextSelected]}>{label}</Text>
        </Pressable>
      ))}
    </View>
  );
}

function BillingNotice({ status }: { status: string | null }) {
  const { t } = useI18n();
  if (status === 'ready' || status === null) return null;
  const message = status === 'unconfigured'
    ? t('subscription.billing.unconfigured')
    : status === 'preview-unavailable'
      ? t('subscription.billing.preview')
      : status === 'missing-offering'
        ? t('subscription.billing.missing')
        : t('subscription.billing.unavailable');
  return <Text style={styles.notice}>{message}</Text>;
}

function PlanCard({
  busyPackageId,
  currentPlan,
  item,
  onPurchase,
  plan,
  restoring,
}: {
  busyPackageId: string | null;
  currentPlan: PlanCode;
  item: MobileBillingPackage | undefined;
  onPurchase(item: MobileBillingPackage): void;
  plan: SubscriptionCatalogPlan;
  restoring: boolean;
}) {
  const { locale, t } = useI18n();
  const current = plan.planCode === currentPlan;
  const free = plan.planCode === 'free';
  const paid = currentPlan !== 'free';
  const busy = item ? busyPackageId === item.identifier : false;
  const blocked = paid && !current;
  return (
    <SettingsCard>
      <View style={settingsStyles.rowBetween}>
        <View style={styles.flexCopy}>
          <Text style={styles.planName}>{plan.displayName}</Text>
          <Text style={styles.price}>{free ? t('subscription.free') : item?.localizedPrice ?? t('subscription.priceUnavailable')}</Text>
        </View>
        {current ? <Text style={styles.currentBadge}>{t('subscription.plan.current')}</Text> : null}
      </View>
      <Text style={settingsStyles.body}>
        {t('subscription.plan.aiLimit', {
          count: plan.entitlements.piAgentAnalysisMonthlyLimit.toLocaleString(locale),
        })}
      </Text>
      <Text style={settingsStyles.body}>
        {free
          ? t('subscription.plan.freeGroups', {
            telegram: (plan.entitlements.telegramGroupLimit ?? 0).toLocaleString(locale),
            wecom: (plan.entitlements.wecomGroupLimit ?? 0).toLocaleString(locale),
          })
          : t('subscription.plan.groupLimit', {
            count: plan.entitlements.combinedGroupLimit.toLocaleString(locale),
          })}
      </Text>
      {!free ? (
        <ActionButton
          busy={busy}
          disabled={current || blocked || !item || busyPackageId !== null || restoring}
          label={busy
            ? t('subscription.plan.processing')
            : current
              ? t('subscription.current')
              : blocked
                ? t('subscription.plan.manageFirst')
                : item
                  ? t('subscription.plan.purchase')
                  : t('subscription.plan.unavailable')}
          onPress={() => item && onPurchase(item)}
        />
      ) : null}
    </SettingsCard>
  );
}

const styles = StyleSheet.create({
  toolbar: { minHeight: 48, flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'flex-end', gap: 10 },
  flexCopy: { flex: 1, gap: 4 },
  eyebrow: { color: colors.subtleText, fontSize: 12, fontWeight: '800', letterSpacing: 0.8 },
  planTitle: { color: colors.text, fontSize: 28, fontWeight: '900' },
  statusBadge: { borderRadius: 999, backgroundColor: colors.accentMuted, paddingHorizontal: 10, paddingVertical: 6 },
  statusBadgeText: { color: colors.accent, fontSize: 12, fontWeight: '800' },
  message: { borderRadius: 12, backgroundColor: colors.accentMuted, color: colors.text, padding: 12, lineHeight: 19 },
  warning: { borderRadius: 12, backgroundColor: colors.noticeBackground, color: colors.noticeText, padding: 12, lineHeight: 19 },
  errorNotice: { borderRadius: 12, backgroundColor: colors.errorBackground, color: colors.errorText, padding: 12, lineHeight: 19 },
  notice: { borderRadius: 12, backgroundColor: colors.noticeBackground, color: colors.noticeText, padding: 12, lineHeight: 19 },
  usageBlock: { gap: 9 },
  usageValue: { color: colors.text, fontSize: 14, fontWeight: '800' },
  progressTrack: { height: 8, overflow: 'hidden', borderRadius: 999, backgroundColor: colors.border },
  progressValue: { height: '100%', borderRadius: 999, backgroundColor: colors.accent },
  intervalTabs: { flexDirection: 'row', borderRadius: 12, backgroundColor: colors.card, padding: 4 },
  intervalTab: { flex: 1, minHeight: 42, alignItems: 'center', justifyContent: 'center', borderRadius: 9 },
  intervalTabSelected: { backgroundColor: colors.button },
  intervalText: { color: colors.mutedText, fontWeight: '700' },
  intervalTextSelected: { color: colors.text },
  planName: { color: colors.text, fontSize: 18, fontWeight: '800' },
  price: { color: colors.text, fontSize: 21, fontWeight: '900' },
  currentBadge: { borderRadius: 999, backgroundColor: colors.accentMuted, color: colors.accent, paddingHorizontal: 10, paddingVertical: 5, fontSize: 12, fontWeight: '800' },
});
