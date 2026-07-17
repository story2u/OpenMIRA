import { describe, expect, it } from 'vitest';

import { catalogs, fallbackLocale } from './catalog';
import { resolveAppLocale, translate } from './core';

describe('resolveAppLocale', () => {
  it('chooses the first supported preferred language', () => {
    expect(resolveAppLocale([
      { languageCode: 'fr', languageTag: 'fr-FR' },
      { languageCode: 'en', languageTag: 'en-US' },
      { languageCode: 'zh', languageTag: 'zh-Hans-CN' },
    ])).toBe('en');
  });

  it('normalizes any Chinese locale to simplified Chinese copy', () => {
    expect(resolveAppLocale([{ languageCode: 'zh', languageTag: 'zh-Hant-TW' }]))
      .toBe('zh-CN');
  });

  it('uses the explicit fallback for empty or unsupported locale lists', () => {
    expect(resolveAppLocale([])).toBe(fallbackLocale);
    expect(resolveAppLocale([{ languageCode: 'ja', languageTag: 'ja-JP' }]))
      .toBe(fallbackLocale);
  });
});

describe('catalogs', () => {
  it('interpolates values without exposing missing values as undefined', () => {
    expect(translate('en', 'account.greeting', { name: 'Ada' })).toBe('Hello, Ada');
    expect(translate('zh-CN', 'account.greeting')).toBe('你好，{name}');
  });

  it('contains meaningful copy for every locale', () => {
    for (const catalog of Object.values(catalogs)) {
      expect(Object.values(catalog).every((message) => message.trim().length > 0)).toBe(true);
    }
  });

  it('keeps interpolation placeholders aligned across locales', () => {
    const placeholders = (message: string) => [...message.matchAll(/\{([a-zA-Z0-9_]+)\}/g)]
      .map((match) => match[1])
      .sort();
    for (const key of Object.keys(catalogs[fallbackLocale]) as (keyof typeof catalogs.en)[]) {
      expect(placeholders(catalogs.en[key]), key)
        .toEqual(placeholders(catalogs[fallbackLocale][key]));
    }
  });
});
