import { Component, type ErrorInfo, type ReactNode } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { useI18n } from '../i18n/I18nProvider';
import { logEvent } from '../logging/redactedLogger';
import { colors } from './theme';

interface Props {
  children: ReactNode;
  message: string;
  title: string;
}

interface State {
  failed: boolean;
}

class ErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, _info: ErrorInfo) {
    logEvent('mobile.render_failed', { errorName: error.name });
  }

  render() {
    if (this.state.failed) {
      return (
        <View style={styles.container}>
          <Text accessibilityRole="header" style={styles.title}>{this.props.title}</Text>
          <Text style={styles.message}>{this.props.message}</Text>
        </View>
      );
    }
    return this.props.children;
  }
}

export function AppErrorBoundary({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  return (
    <ErrorBoundary message={t('app.error.message')} title={t('app.error.title')}>
      {children}
    </ErrorBoundary>
  );
}

const styles = StyleSheet.create({
  container: {
    alignItems: 'center',
    backgroundColor: colors.background,
    flex: 1,
    gap: 12,
    justifyContent: 'center',
    padding: 32,
  },
  message: { color: colors.mutedText, fontSize: 15, lineHeight: 22, textAlign: 'center' },
  title: { color: colors.text, fontSize: 22, fontWeight: '700' },
});
