export const SUPPORTED_LOCALES = ["en", "zh", "ko", "ja"] as const;

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export const DEFAULT_LOCALE: SupportedLocale = "en";

export function isSupportedLocale(locale: string): locale is SupportedLocale {
  return SUPPORTED_LOCALES.includes(locale as SupportedLocale);
}

function normalizeLocaleCandidate(locale: string): string {
  return locale.trim().replaceAll("_", "-").toLowerCase();
}

export function resolveSupportedLocale(locale: string | undefined | null): SupportedLocale {
  if (!locale) {
    return DEFAULT_LOCALE;
  }

  const normalizedLocale = normalizeLocaleCandidate(locale);
  if (isSupportedLocale(normalizedLocale)) {
    return normalizedLocale;
  }

  const primaryLanguage = normalizedLocale.split("-")[0];
  return primaryLanguage && isSupportedLocale(primaryLanguage)
    ? primaryLanguage
    : DEFAULT_LOCALE;
}
