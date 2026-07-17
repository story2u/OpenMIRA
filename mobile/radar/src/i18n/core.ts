import {
  catalogs,
  fallbackLocale,
  type AppLocale,
  type MessageKey,
  type MessageValues,
} from './catalog';

export interface DeviceLocale {
  languageCode?: string | null;
  languageTag?: string | null;
}

export type Translator = (key: MessageKey, values?: MessageValues) => string;

export function resolveAppLocale(locales: readonly DeviceLocale[]): AppLocale {
  for (const locale of locales) {
    const languageCode = locale.languageCode?.toLowerCase()
      ?? locale.languageTag?.split('-')[0]?.toLowerCase();
    if (languageCode === 'zh') return 'zh-CN';
    if (languageCode === 'en') return 'en';
  }
  return fallbackLocale;
}

export function translate(
  locale: AppLocale,
  key: MessageKey,
  values: MessageValues = {},
) {
  return catalogs[locale][key].replace(/\{([a-zA-Z0-9_]+)\}/g, (placeholder, name: string) => {
    const value = values[name];
    return value === undefined ? placeholder : String(value);
  });
}

export function createTranslator(locale: AppLocale): Translator {
  return (key, values) => translate(locale, key, values);
}

export const fallbackTranslator = createTranslator(fallbackLocale);
