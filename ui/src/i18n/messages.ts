import {
  DEFAULT_LOCALE,
  REQUIRED_LOCALES,
  type RequiredLocale,
  resolveSupportedLocale,
  type SupportedLocale,
} from "./locales";

export type LocalizedMessages<T> = Record<SupportedLocale, T>;
export type LocalizedMessageCatalog<T> = Partial<Record<SupportedLocale, T>> &
  Record<typeof DEFAULT_LOCALE, T>;

export interface MessageCatalogValidationIssue {
  locale: RequiredLocale;
  path: string;
}

export function resolveLocalizedMessages<T>(
  messages: LocalizedMessageCatalog<T>,
  locale?: string | null,
): T {
  const resolvedLocale = resolveSupportedLocale(locale);

  return messages[resolvedLocale] ?? messages[DEFAULT_LOCALE];
}

export function validateRequiredLocaleMessages<T>(
  messages: LocalizedMessageCatalog<T>,
): MessageCatalogValidationIssue[] {
  const defaultMessages = messages[DEFAULT_LOCALE];

  return REQUIRED_LOCALES.flatMap((locale) => {
    if (locale === DEFAULT_LOCALE) {
      return [];
    }

    return collectMissingMessageFields(
      defaultMessages,
      messages[locale],
      locale,
      locale,
    );
  });
}

function collectMissingMessageFields(
  defaultValue: unknown,
  localizedValue: unknown,
  locale: RequiredLocale,
  path: string,
): MessageCatalogValidationIssue[] {
  if (localizedValue === undefined || localizedValue === null) {
    return [{ locale, path }];
  }

  if (!isPlainObject(defaultValue)) {
    return [];
  }

  if (!isPlainObject(localizedValue)) {
    return [{ locale, path }];
  }

  return Object.keys(defaultValue).flatMap((key) =>
    collectMissingMessageFields(
      defaultValue[key],
      localizedValue[key],
      locale,
      `${path}.${key}`,
    ),
  );
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
