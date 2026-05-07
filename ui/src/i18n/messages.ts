import { DEFAULT_LOCALE, type SupportedLocale, resolveSupportedLocale } from "./locales";

export type LocalizedMessages<T> = Record<SupportedLocale, T>;

export function resolveLocalizedMessages<T>(
  messages: LocalizedMessages<T>,
  locale?: string | null,
): T {
  const resolvedLocale = resolveSupportedLocale(locale);

  return messages[resolvedLocale] ?? messages[DEFAULT_LOCALE];
}
