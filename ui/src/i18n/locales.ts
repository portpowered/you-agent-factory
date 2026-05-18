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

const LOCALE_ALIASES: Partial<Record<string, SupportedLocale>> = {
  zh: "zh-CN",
  "zh-Hans": "zh-CN",
  "zh-cn": "zh-CN",
  "zh-hans": "zh-CN",
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

  if (isSupportedLocale(normalizedLocale)) {
    return normalizedLocale;
  }

  return LOCALE_ALIASES[normalizedLocale] ?? DEFAULT_LOCALE;
}

function normalizeLocaleInput(
  locale: string | undefined | null,
): string | undefined {
  const normalizedLocale = locale?.trim().replaceAll("_", "-");

  return normalizedLocale || undefined;
}
