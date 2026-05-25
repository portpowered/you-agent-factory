import { resolveSupportedLocale } from "./locales";

type DateInput = Date | number | string;

export type CountLabels = Partial<Record<Intl.LDMLPluralRule, string>> & {
  other: string;
};

export function formatDate(
  value: DateInput,
  locale?: string | null,
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return String(value);
  }

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    dateStyle: "medium",
    ...options,
  }).format(date);
}

export function formatTime(
  value: DateInput,
  locale?: string | null,
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return String(value);
  }

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    hour: "numeric",
    minute: "2-digit",
    ...options,
  }).format(date);
}

export function formatDateTime(
  value: DateInput,
  locale?: string | null,
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return String(value);
  }

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    dateStyle: "medium",
    timeStyle: "short",
    ...options,
  }).format(date);
}

export function formatNumber(
  value: number,
  locale?: string | null,
  options?: Intl.NumberFormatOptions,
): string {
  return new Intl.NumberFormat(resolveSupportedLocale(locale), options).format(
    value,
  );
}

export function formatPercent(
  value: number,
  locale?: string | null,
  options?: Intl.NumberFormatOptions,
): string {
  return formatNumber(value, locale, {
    style: "percent",
    ...options,
  });
}

export function formatList(
  values: string[],
  locale?: string | null,
  options?: Intl.ListFormatOptions,
): string {
  return new Intl.ListFormat(resolveSupportedLocale(locale), {
    style: "long",
    type: "conjunction",
    ...options,
  }).format(values);
}

export function formatRelativeTime(
  value: number,
  unit: Intl.RelativeTimeFormatUnit,
  locale?: string | null,
  options?: Intl.RelativeTimeFormatOptions,
): string {
  return new Intl.RelativeTimeFormat(resolveSupportedLocale(locale), {
    numeric: "auto",
    ...options,
  }).format(value, unit);
}

export function formatCount(
  value: number,
  labels: CountLabels,
  locale?: string | null,
  options?: Intl.NumberFormatOptions,
): string {
  const resolvedLocale = resolveSupportedLocale(locale);
  const pluralCategory = new Intl.PluralRules(resolvedLocale).select(value);
  const label = labels[pluralCategory] ?? labels.other;

  return `${formatNumber(value, resolvedLocale, options)} ${label}`;
}

function toValidDate(value: DateInput): Date | null {
  const date = value instanceof Date ? value : new Date(value);

  return Number.isNaN(date.getTime()) ? null : date;
}
