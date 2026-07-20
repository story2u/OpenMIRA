import { Redirect, Tabs } from 'expo-router';
import { Text } from 'react-native';

import { useSession } from '../../src/auth/SessionProvider';
import { isMiraConciergeUiEnabled } from '../../src/config/miraConciergeFlag';
import { useI18n } from '../../src/i18n/I18nProvider';
import { colors } from '../../src/ui/theme';

export default function AuthenticatedTabsLayout() {
  const { capabilities, state } = useSession();
  const { t } = useI18n();
  const miraEnabled = isMiraConciergeUiEnabled();
  if (state.status !== 'authenticated') return <Redirect href="/" />;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.mutedText,
        tabBarStyle: {
          borderTopWidth: 0,
          borderTopColor: colors.border,
          backgroundColor: colors.card,
          minHeight: 64,
          paddingBottom: 8,
          paddingTop: 7,
        },
      }}
    >
      <Tabs.Screen
        name="home"
        options={{
          title: t('tabs.home'),
          tabBarAccessibilityLabel: t('tabs.home.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>✦</Text>,
        }}
      />
      <Tabs.Screen
        name="dashboard"
        options={{
          title: t('tabs.opportunities'),
          tabBarAccessibilityLabel: t('tabs.opportunities.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>◫</Text>,
        }}
      />
      <Tabs.Screen
        name="agent"
        options={{
          title: t('tabs.agent'),
          tabBarAccessibilityLabel: t('tabs.agent.accessibility'),
          href: miraEnabled || capabilities.agentToolsAvailable ? undefined : null,
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>✦</Text>,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          href: miraEnabled ? null : undefined,
          title: t('tabs.settings'),
          tabBarAccessibilityLabel: t('tabs.settings.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>●</Text>,
        }}
      />
    </Tabs>
  );
}
