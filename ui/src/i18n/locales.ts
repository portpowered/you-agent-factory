export const SUPPORTED_LOCALES = ["en", "zh", "ko", "ja"] as const;

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export const DEFAULT_LOCALE: SupportedLocale = "en";

export function isSupportedLocale(locale: string): locale is SupportedLocale {
  return SUPPORTED_LOCALES.includes(locale as SupportedLocale);
}

export function resolveSupportedLocale(locale: string | undefined | null): SupportedLocale {
  if (!locale) {
    return DEFAULT_LOCALE;
  }

  return isSupportedLocale(locale) ? locale : DEFAULT_LOCALE;
}
