import { resolveSupportedLocale } from "./locales";

type DateInput = Date | number | string;
type DateLikeInput = DateInput | null | undefined;
type RelativeTimeValue = number | null | undefined;
type DurationUnit = "hour" | "minute" | "second" | "millisecond";

type DateTimeFormatterOptions = Intl.DateTimeFormatOptions & {
  fallback?: string;
};

type RelativeTimeFormatterOptions = Intl.RelativeTimeFormatOptions & {
  fallback?: string;
};

export type DurationFormatStyle = "compact" | "verbose";

export interface DurationFormatterOptions {
  fallback?: string;
  style?: DurationFormatStyle;
}

export type CountLabels = Partial<Record<Intl.LDMLPluralRule, string>> & {
  other: string;
};

export function formatDate(
  value: DateLikeInput,
  locale?: string | null,
  options?: DateTimeFormatterOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return formatInvalidDateValue(value, options?.fallback);
  }

  const { fallback: _fallback, ...dateTimeOptions } = options ?? {};

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    dateStyle: "medium",
    ...dateTimeOptions,
  }).format(date);
}

export function formatTime(
  value: DateLikeInput,
  locale?: string | null,
  options?: DateTimeFormatterOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return formatInvalidDateValue(value, options?.fallback);
  }

  const { fallback: _fallback, ...dateTimeOptions } = options ?? {};

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    hour: "numeric",
    minute: "2-digit",
    ...dateTimeOptions,
  }).format(date);
}

export function formatDateTime(
  value: DateLikeInput,
  locale?: string | null,
  options?: DateTimeFormatterOptions,
): string {
  const date = toValidDate(value);
  if (!date) {
    return formatInvalidDateValue(value, options?.fallback);
  }

  const { fallback: _fallback, ...dateTimeOptions } = options ?? {};

  return new Intl.DateTimeFormat(resolveSupportedLocale(locale), {
    dateStyle: "medium",
    timeStyle: "short",
    ...dateTimeOptions,
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
  value: RelativeTimeValue,
  unit: Intl.RelativeTimeFormatUnit,
  locale?: string | null,
  options?: RelativeTimeFormatterOptions,
): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return options?.fallback ?? "";
  }

  const { fallback: _fallback, ...relativeTimeOptions } = options ?? {};

  return new Intl.RelativeTimeFormat(resolveSupportedLocale(locale), {
    numeric: "auto",
    ...relativeTimeOptions,
  }).format(value, unit);
}

export function formatDuration(
  value: number | null | undefined,
  locale?: string | null,
  options?: DurationFormatterOptions,
): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return options?.fallback ?? "";
  }

  const resolvedLocale = resolveSupportedLocale(locale);
  const style = options?.style ?? "compact";
  const safeDurationMillis = Math.max(0, Math.floor(value));
  const units = toDurationUnits(safeDurationMillis);

  if (style === "verbose") {
    return formatVerboseDurationParts(units, resolvedLocale);
  }

  return formatCompactDurationParts(units, resolvedLocale);
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

function toValidDate(value: DateLikeInput): Date | null {
  if (value === null || value === undefined) {
    return null;
  }

  const date = value instanceof Date ? value : new Date(value);

  return Number.isNaN(date.getTime()) ? null : date;
}

function formatInvalidDateValue(
  value: DateLikeInput,
  fallback?: string,
): string {
  if (fallback !== undefined) {
    return fallback;
  }

  if (value === null || value === undefined) {
    return "";
  }

  return String(value);
}

interface DurationUnits {
  milliseconds?: number;
  seconds?: number;
  minutes?: number;
  hours?: number;
}

const COMPACT_DURATION_LABELS: Record<
  string,
  Record<DurationUnit, string>
> = {
  en: {
    hour: "h",
    minute: "m",
    second: "s",
    millisecond: "ms",
  },
  "zh-CN": {
    hour: "小时",
    minute: "分",
    second: "秒",
    millisecond: "毫秒",
  },
  ja: {
    hour: "時間",
    minute: "分",
    second: "秒",
    millisecond: "ミリ秒",
  },
  ko: {
    hour: "시간",
    minute: "분",
    second: "초",
    millisecond: "밀리초",
  },
};

const VERBOSE_DURATION_LABELS: Record<
  string,
  Record<DurationUnit, string | CountLabels>
> = {
  en: {
    hour: {
      one: "hour",
      other: "hours",
    },
    minute: {
      one: "minute",
      other: "minutes",
    },
    second: {
      one: "second",
      other: "seconds",
    },
    millisecond: {
      one: "millisecond",
      other: "milliseconds",
    },
  },
  "zh-CN": {
    hour: "小时",
    minute: "分钟",
    second: "秒",
    millisecond: "毫秒",
  },
  ja: {
    hour: "時間",
    minute: "分",
    second: "秒",
    millisecond: "ミリ秒",
  },
  ko: {
    hour: "시간",
    minute: "분",
    second: "초",
    millisecond: "밀리초",
  },
};

function toDurationUnits(durationMillis: number): DurationUnits {
  if (durationMillis < 1000) {
    return { milliseconds: durationMillis };
  }

  const durationSeconds = Math.floor(durationMillis / 1000);
  const hours = Math.floor(durationSeconds / 3600);
  const minutes = Math.floor((durationSeconds % 3600) / 60);
  const seconds = durationSeconds % 60;

  if (hours > 0) {
    return {
      hours,
      minutes,
    };
  }

  if (minutes > 0) {
    return {
      minutes,
      seconds,
    };
  }

  return {
    seconds: durationSeconds,
  };
}

function formatCompactDurationParts(
  units: DurationUnits,
  locale: string,
): string {
  return getDurationParts(units)
    .map(({ unit, value }) => formatCompactDurationPart(value, unit, locale))
    .join(" ");
}

function formatVerboseDurationParts(units: DurationUnits, locale: string): string {
  return getDurationParts(units, {
    omitZeroValues: true,
  })
    .map(({ unit, value }) => formatVerboseDurationPart(value, unit, locale))
    .join(locale === "zh-CN" ? "" : ", ");
}

function getDurationParts(
  units: DurationUnits,
  options?: {
    omitZeroValues?: boolean;
  },
): Array<{
  unit: DurationUnit;
  value: number;
}> {
  const parts: Array<{ unit: DurationUnit; value: number }> = [];
  const omitZeroValues = options?.omitZeroValues ?? false;

  if (units.hours !== undefined && (!omitZeroValues || units.hours > 0)) {
    parts.push({ unit: "hour", value: units.hours });
  }
  if (units.minutes !== undefined && (!omitZeroValues || units.minutes > 0)) {
    parts.push({ unit: "minute", value: units.minutes });
  }
  if (units.seconds !== undefined && (!omitZeroValues || units.seconds > 0)) {
    parts.push({ unit: "second", value: units.seconds });
  }
  if (
    units.milliseconds !== undefined &&
    (!omitZeroValues || units.milliseconds > 0)
  ) {
    parts.push({ unit: "millisecond", value: units.milliseconds });
  }

  return parts;
}

function formatCompactDurationPart(
  value: number,
  unit: DurationUnit,
  locale: string,
): string {
  return `${formatNumber(value, locale)}${getCompactDurationLabel(unit, locale)}`;
}

function formatVerboseDurationPart(
  value: number,
  unit: DurationUnit,
  locale: string,
): string {
  if (shouldOmitDurationUnitSpacing(locale)) {
    return `${formatNumber(value, locale)}${getVerboseDurationLabel(unit, locale, value)}`;
  }

  return `${formatNumber(value, locale)} ${getVerboseDurationLabel(unit, locale, value)}`;
}

function shouldOmitDurationUnitSpacing(locale: string): boolean {
  return locale === "zh-CN" || locale === "ko";
}

function getCompactDurationLabel(
  unit: DurationUnit,
  locale: string,
): string {
  return COMPACT_DURATION_LABELS[locale]?.[unit] ?? COMPACT_DURATION_LABELS.en[unit];
}

function getVerboseDurationLabel(
  unit: DurationUnit,
  locale: string,
  value: number,
): string {
  const labels = VERBOSE_DURATION_LABELS[locale]?.[unit] ?? VERBOSE_DURATION_LABELS.en[unit];
  if (typeof labels === "string") {
    return labels;
  }

  const pluralCategory = new Intl.PluralRules(locale).select(value);

  return labels[pluralCategory] ?? labels.other;
}
