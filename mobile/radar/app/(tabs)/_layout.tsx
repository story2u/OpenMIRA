import { Redirect, Tabs } from 'expo-router';
import { Text } from 'react-native';

import { useSession } from '../../src/auth/SessionProvider';
import { useI18n } from '../../src/i18n/I18nProvider';
import { colors } from '../../src/ui/theme';

export default function AuthenticatedTabsLayout() {
  const { capabilities, state } = useSession();
  const { t } = useI18n();
  if (state.status !== 'authenticated') return <Redirect href="/" />;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.mutedText,
        tabBarStyle: {
          borderTopColor: colors.border,
          backgroundColor: colors.card,
        },
      }}
    >
      <Tabs.Screen
        name="dashboard"
        options={{
          title: t('tabs.dashboard'),
          tabBarAccessibilityLabel: t('tabs.dashboard.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>◫</Text>,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          title: t('tabs.settings'),
          tabBarAccessibilityLabel: t('tabs.settings.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>●</Text>,
        }}
      />
      <Tabs.Screen
        name="agent"
        options={{
          href: capabilities.agentToolsAvailable ? undefined : null,
          title: t('tabs.agent'),
          tabBarAccessibilityLabel: t('tabs.agent.accessibility'),
          tabBarIcon: ({ color }) => <Text style={{ color, fontSize: 17 }}>✦</Text>,
        }}
      />
    </Tabs>
  );
}
