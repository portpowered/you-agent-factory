/**
 * English is the default authoring locale and the fallback locale for the
 * website. Simplified Mandarin Chinese for China uses canonical BCP 47 locale
 * identifier zh-CN.
 */
export const SUPPORTED_LOCALES = ["en", "zh-CN", "ko", "ja"] as const;

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export const DEFAULT_LOCALE = "en" as const satisfies SupportedLocale;
export const REQUIRED_LOCALES = ["en", "zh-CN"] as const;

export type RequiredLocale = (typeof REQUIRED_LOCALES)[number];

export const NATIVE_LANGUAGE_LABELS = {
  en: "English",
  "zh-CN": "简体中文",
  ko: "한국어",
  ja: "日本語",
} as const satisfies Record<SupportedLocale, string>;

const SUPPORTED_LOCALE_LOOKUP: ReadonlyMap<string, SupportedLocale> = new Map(
  SUPPORTED_LOCALES.map((locale) => [locale.toLowerCase(), locale]),
);

const LOCALE_ALIASES: Partial<Record<string, SupportedLocale>> = {
  zh: "zh-CN",
  "zh-cn": "zh-CN",
  "zh-hans": "zh-CN",
  "zh-hans-cn": "zh-CN",
};

export function isSupportedLocale(locale: string): locale is SupportedLocale {
  return SUPPORTED_LOCALES.includes(locale as SupportedLocale);
}

export function resolveSupportedLocale(
  locale: string | undefined | null,
): SupportedLocale {
  const normalizedLocale = normalizeLocaleInput(locale);
  if (!normalizedLocale) {
    return DEFAULT_LOCALE;
  }

  return (
    resolveNormalizedLocale(normalizedLocale) ??
    resolveNormalizedLocale(normalizedLocale.split("-")[0]) ??
    DEFAULT_LOCALE
  );
}

export function getNativeLanguageLabel(
  locale: string | undefined | null,
): string {
  return NATIVE_LANGUAGE_LABELS[resolveSupportedLocale(locale)];
}

function resolveNormalizedLocale(
  locale: string | undefined,
): SupportedLocale | undefined {
  if (!locale) {
    return undefined;
  }

  return SUPPORTED_LOCALE_LOOKUP.get(locale) ?? LOCALE_ALIASES[locale];
}

function normalizeLocaleInput(
  locale: string | undefined | null,
): string | undefined {
  const normalizedLocale = locale?.trim().replaceAll("_", "-").toLowerCase();

  return normalizedLocale || undefined;
}
