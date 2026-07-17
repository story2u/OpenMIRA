import { Stack } from 'expo-router';

import { SessionProvider } from '../src/auth/SessionProvider';
import { I18nProvider } from '../src/i18n/I18nProvider';
import { AppErrorBoundary } from '../src/ui/AppErrorBoundary';
import { colors } from '../src/ui/theme';

export default function RootLayout() {
  return (
    <I18nProvider>
      <AppErrorBoundary>
        <SessionProvider>
          <Stack
            screenOptions={{
              contentStyle: { backgroundColor: colors.background },
              headerShown: false,
            }}
          />
        </SessionProvider>
      </AppErrorBoundary>
    </I18nProvider>
  );
}
