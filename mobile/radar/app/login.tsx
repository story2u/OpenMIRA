import { RadarApiError } from '@story2u/radar-api/client';
import * as AppleAuthentication from 'expo-apple-authentication';
import { Redirect, useLocalSearchParams, type Href } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useRef, useState } from 'react';
import {
  ActivityIndicator,
  Image,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';

import { sessionUnavailableMessage, useSession } from '../src/auth/SessionProvider';
import { loginErrorMessage, nativeLoginErrorMessage } from '../src/auth/loginError';
import {
  isAppleNativeLoginAvailable,
  isGoogleNativeLoginAvailable,
  requestNativeIdentity,
  type NativeIdentityProvider,
} from '../src/auth/nativeIdentity';
import { logEvent } from '../src/logging/redactedLogger';
import { useI18n } from '../src/i18n/I18nProvider';
import { safeReturnTo } from '../src/navigation/returnTo';
import { colors } from '../src/ui/theme';

const emailPattern = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;
const googleButtonSource = Platform.OS === 'ios'
  ? require('../assets/google-signin/ios-dark-pill.png')
  : require('../assets/google-signin/android-dark-pill.png');

type LoginMethod = 'apple' | 'google' | 'password';

export default function LoginScreen() {
  const params = useLocalSearchParams<{ returnTo?: string | string[] }>();
  const { loginWithNativeToken, loginWithPassword, retry, state } = useSession();
  const { t } = useI18n();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [activeMethod, setActiveMethod] = useState<LoginMethod | null>(null);
  const [appleAvailable, setAppleAvailable] = useState(false);
  const [error, setError] = useState('');
  const activeMethodRef = useRef<LoginMethod | null>(null);

  useEffect(() => {
    let active = true;
    void isAppleNativeLoginAvailable().then((available) => {
      if (active) setAppleAvailable(available);
    });
    return () => {
      active = false;
    };
  }, []);

  if (state.status === 'authenticated') {
    return <Redirect href={safeReturnTo(params.returnTo) as Href} />;
  }

  const normalizedEmail = email.trim().toLowerCase();
  const submitting = activeMethod !== null;
  const canSubmit = emailPattern.test(normalizedEmail) && password.length > 0 && !submitting;
  const sessionNotice =
    state.status === 'requires-login'
      ? state.reason === 'expired'
        ? t('auth.session.expired')
        : t('auth.session.migrationFailed')
      : null;

  function begin(method: LoginMethod) {
    if (activeMethodRef.current) return false;
    activeMethodRef.current = method;
    setActiveMethod(method);
    setError('');
    return true;
  }

  function finish() {
    activeMethodRef.current = null;
    setActiveMethod(null);
  }

  async function submitPassword() {
    if (!canSubmit || !begin('password')) return;
    try {
      await loginWithPassword(normalizedEmail, password);
      setPassword('');
    } catch (caught) {
      setError(loginErrorMessage(caught, t));
    } finally {
      finish();
    }
  }

  async function submitNative(provider: NativeIdentityProvider) {
    if (!begin(provider)) return;
    try {
      const identity = await requestNativeIdentity(provider);
      if (identity.type === 'cancelled') return;
      await loginWithNativeToken(provider, identity.idToken);
    } catch (caught) {
      logEvent('auth.native_login_failed', {
        provider,
        errorClass: caught instanceof Error ? caught.name : 'UnknownError',
        status: caught instanceof RadarApiError ? caught.status : undefined,
      });
      setError(nativeLoginErrorMessage(provider, caught, t));
    } finally {
      finish();
    }
  }

  if (state.status === 'loading') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.centered}>
          <ActivityIndicator color={colors.accent} size="large" />
          <Text style={styles.help}>{t('auth.session.restoring')}</Text>
        </View>
      </SafeAreaView>
    );
  }

  if (state.status === 'unavailable') {
    return (
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.centered}>
          <Text accessibilityRole="header" style={styles.title}>{t('auth.unavailable.title')}</Text>
          <Text accessibilityLiveRegion="polite" style={styles.help}>
            {sessionUnavailableMessage(state.reason, t)}
          </Text>
          <Pressable accessibilityRole="button" onPress={retry} style={styles.primaryButton}>
            <Text style={styles.primaryButtonText}>{t('common.retry')}</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safeArea}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.flex}
      >
        <ScrollView
          contentContainerStyle={styles.container}
          keyboardShouldPersistTaps="handled"
        >
          <Text style={styles.eyebrow}>OPPORTUNITY RADAR</Text>
          <Text accessibilityRole="header" style={styles.title}>{t('auth.title')}</Text>
          <Text style={styles.description}>{t('auth.description')}</Text>

          {sessionNotice ? <Text style={styles.notice}>{sessionNotice}</Text> : null}

          {appleAvailable || isGoogleNativeLoginAvailable() ? (
            <View style={styles.nativeLoginGroup}>
              {appleAvailable ? (
                <View
                  pointerEvents={submitting ? 'none' : 'auto'}
                  style={submitting && activeMethod !== 'apple' ? styles.disabledButton : undefined}
                >
                  <AppleAuthentication.AppleAuthenticationButton
                    accessibilityLabel={t('auth.apple')}
                    accessibilityState={{ busy: activeMethod === 'apple', disabled: submitting }}
                    buttonStyle={AppleAuthentication.AppleAuthenticationButtonStyle.BLACK}
                    buttonType={AppleAuthentication.AppleAuthenticationButtonType.SIGN_IN}
                    cornerRadius={22}
                    onPress={() => void submitNative('apple')}
                    style={styles.appleButton}
                  />
                </View>
              ) : null}

              {isGoogleNativeLoginAvailable() ? (
                <Pressable
                  accessibilityLabel={t('auth.google')}
                  accessibilityRole="button"
                  accessibilityState={{ busy: activeMethod === 'google', disabled: submitting }}
                  disabled={submitting}
                  onPress={() => void submitNative('google')}
                  style={({ pressed }) => [
                    styles.googleButton,
                    submitting && activeMethod !== 'google' && styles.disabledButton,
                    pressed && styles.pressedButton,
                  ]}
                >
                  <Image source={googleButtonSource} style={styles.googleButtonImage} />
                  {activeMethod === 'google' ? (
                    <View style={styles.googleButtonBusy}>
                      <ActivityIndicator color="#ffffff" />
                    </View>
                  ) : null}
                </Pressable>
              ) : null}

              <View style={styles.dividerRow}>
                <View style={styles.divider} />
                <Text style={styles.dividerText}>{t('auth.emailDivider')}</Text>
                <View style={styles.divider} />
              </View>
            </View>
          ) : null}

          <View style={styles.form}>
            <Text style={styles.label}>{t('auth.email')}</Text>
            <TextInput
              accessibilityLabel={t('auth.email')}
              autoCapitalize="none"
              autoComplete="email"
              autoCorrect={false}
              editable={!submitting}
              inputMode="email"
              onChangeText={setEmail}
              placeholder="name@example.com"
              placeholderTextColor={colors.placeholder}
              returnKeyType="next"
              style={styles.input}
              textContentType="emailAddress"
              value={email}
            />

            <Text style={styles.label}>{t('auth.password')}</Text>
            <TextInput
              accessibilityLabel={t('auth.password')}
              autoCapitalize="none"
              autoComplete="current-password"
              editable={!submitting}
              onChangeText={setPassword}
              onSubmitEditing={() => void submitPassword()}
              placeholder={t('auth.passwordPlaceholder')}
              placeholderTextColor={colors.placeholder}
              returnKeyType="go"
              secureTextEntry
              style={styles.input}
              textContentType="password"
              value={password}
            />

            <Pressable
              accessibilityRole="button"
              accessibilityState={{ disabled: !canSubmit, busy: submitting }}
              disabled={!canSubmit}
              onPress={() => void submitPassword()}
              style={({ pressed }) => [
                styles.primaryButton,
                !canSubmit && styles.disabledButton,
                pressed && styles.pressedButton,
              ]}
            >
              {activeMethod === 'password' ? <ActivityIndicator color={colors.text} /> : null}
              <Text style={styles.primaryButtonText}>
                {activeMethod === 'password' ? t('auth.submitting') : t('auth.submit')}
              </Text>
            </Pressable>
          </View>

          <View accessibilityLiveRegion="polite" style={styles.errorSlot}>
            {error ? <Text accessibilityRole="alert" style={styles.error}>{error}</Text> : null}
          </View>
          <Text style={styles.privacy}>{t('auth.privacy')}</Text>
        </ScrollView>
      </KeyboardAvoidingView>
      <StatusBar style="light" />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  safeArea: { flex: 1, backgroundColor: colors.background },
  centered: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 18, padding: 28 },
  container: { flexGrow: 1, justifyContent: 'center', padding: 24 },
  eyebrow: { color: colors.accent, fontSize: 12, fontWeight: '800', letterSpacing: 2 },
  title: { marginTop: 10, color: colors.text, fontSize: 32, fontWeight: '800' },
  description: { marginTop: 12, color: colors.mutedText, fontSize: 15, lineHeight: 23 },
  notice: { marginTop: 20, borderRadius: 12, backgroundColor: colors.noticeBackground, color: colors.noticeText, padding: 13 },
  nativeLoginGroup: { alignItems: 'center', gap: 12, marginTop: 28 },
  appleButton: { width: 188, height: 44 },
  googleButton: { width: Platform.OS === 'ios' ? 188 : 180, height: Platform.OS === 'ios' ? 44 : 40 },
  googleButtonImage: { width: '100%', height: '100%', resizeMode: 'contain' },
  googleButtonBusy: { ...StyleSheet.absoluteFill, alignItems: 'center', justifyContent: 'center', borderRadius: 22, backgroundColor: 'rgba(19, 19, 20, 0.82)' },
  dividerRow: { width: '100%', flexDirection: 'row', alignItems: 'center', gap: 12, marginTop: 10 },
  divider: { flex: 1, height: StyleSheet.hairlineWidth, backgroundColor: colors.border },
  dividerText: { color: colors.subtleText, fontSize: 12 },
  form: { marginTop: 30, gap: 10 },
  label: { marginTop: 8, color: colors.text, fontSize: 14, fontWeight: '700' },
  input: { borderWidth: 1, borderColor: colors.border, borderRadius: 12, backgroundColor: colors.card, color: colors.text, fontSize: 16, paddingHorizontal: 15, paddingVertical: 14 },
  primaryButton: { marginTop: 16, minHeight: 50, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 10, borderRadius: 12, backgroundColor: colors.button, paddingHorizontal: 24, paddingVertical: 13 },
  primaryButtonText: { color: colors.text, fontSize: 16, fontWeight: '800' },
  disabledButton: { opacity: 0.45 },
  pressedButton: { opacity: 0.75 },
  errorSlot: { minHeight: 58, marginTop: 18 },
  error: { borderRadius: 12, backgroundColor: colors.errorBackground, color: colors.errorText, padding: 13, lineHeight: 21 },
  help: { color: colors.mutedText, fontSize: 15, lineHeight: 22, textAlign: 'center' },
  privacy: { color: colors.subtleText, fontSize: 12, lineHeight: 19, textAlign: 'center' },
});
