import { useLocales } from 'expo-localization';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react';

import type { AppLocale, MessageKey, MessageValues } from './catalog';
import { resolveAppLocale, translate, type Translator } from './core';

interface I18nContextValue {
  locale: AppLocale;
  t: Translator;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const deviceLocales = useLocales();
  const locale = useMemo(() => resolveAppLocale(deviceLocales), [deviceLocales]);
  const t = useCallback(
    (key: MessageKey, values?: MessageValues) => translate(locale, key, values),
    [locale],
  );
  const value = useMemo(() => ({ locale, t }), [locale, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error('useI18n must be used within I18nProvider');
  return context;
}
