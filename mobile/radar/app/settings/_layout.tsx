import { Redirect, Stack } from 'expo-router';

import { useSession } from '../../src/auth/SessionProvider';
import { SettingsProvider } from '../../src/features/settings/SettingsProvider';
import { colors } from '../../src/ui/theme';

export default function SettingsLayout() {
  const { state } = useSession();
  if (state.status !== 'authenticated') return <Redirect href="/" />;
  return (
    <SettingsProvider>
      <Stack
        screenOptions={{
          contentStyle: { backgroundColor: colors.background },
          headerShown: false,
        }}
      />
    </SettingsProvider>
  );
}
