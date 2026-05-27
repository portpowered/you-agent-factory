import type {
  DashboardProviderSession,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import {
  formatDateTime,
  formatDuration,
  formatRelativeTime,
  formatTime,
} from "../../i18n/formatters";
import { getSharedPrimitiveMessages } from "./messages/shared-primitives";

const LOCAL_JSONL_EXTENSION = ".jsonl";
const SESSION_LOG_HREF_PROTOCOLS = new Set(["file:", "http:", "https:"]);
const UNKNOWN_WORK_LABEL = "Unknown work";

export interface ProviderSessionLogTarget {
  display: string;
  href: string;
}

export interface LocalDateTimeDisplay {
  label: string;
  rawTimestamp: string | null;
}

export function formatDurationMillis(
  durationMillis: number,
  locale?: string | null,
): string {
  return formatDuration(durationMillis, locale, {
    style: "compact",
  });
}

export function formatDurationMillisVerbose(
  durationMillis: number,
  locale?: string | null,
): string {
  return formatDuration(durationMillis, locale, {
    style: "verbose",
  });
}

export function formatDurationFromISO(
  startedAt: string,
  now: number,
  locale?: string | null,
  fallback = "Unavailable",
): string {
  const startedAtMs = Date.parse(startedAt);
  if (Number.isNaN(startedAtMs)) {
    return fallback;
  }

  return formatDurationMillis(now - startedAtMs, locale);
}

export function formatRelativeTimeFromISO(
  timestamp: string,
  now: number,
  locale?: string | null,
  fallback = "Unavailable",
): string {
  const timestampMs = Date.parse(timestamp);
  if (Number.isNaN(timestampMs)) {
    return fallback;
  }

  const relativeMillis = timestampMs - now;
  const absoluteMillis = Math.abs(relativeMillis);

  if (absoluteMillis < 60_000) {
    return formatRelativeTime(
      Math.round(relativeMillis / 1_000),
      "second",
      locale,
      { fallback },
    );
  }

  if (absoluteMillis < 3_600_000) {
    return formatRelativeTime(
      Math.round(relativeMillis / 60_000),
      "minute",
      locale,
      { fallback },
    );
  }

  if (absoluteMillis < 86_400_000) {
    return formatRelativeTime(
      Math.round(relativeMillis / 3_600_000),
      "hour",
      locale,
      { fallback },
    );
  }

  return formatRelativeTime(
    Math.round(relativeMillis / 86_400_000),
    "day",
    locale,
    { fallback },
  );
}

export function formatTimeOfDay(
  isoTimestamp: string,
  locale?: string | null,
): string {
  return formatTime(isoTimestamp, locale, {
    fallback: isoTimestamp,
  }).replace(/\s/g, "");
}

export function formatLocalDateTime(
  timestamp: string | undefined,
  unavailableLabel: string,
  locale?: string | null,
): string {
  const normalizedTimestamp = normalizeValidTimestamp(timestamp);
  if (!normalizedTimestamp) {
    return unavailableLabel;
  }

  return formatDateTime(normalizedTimestamp, locale, {
    fallback: unavailableLabel,
  });
}

export function getLocalDateTimeDisplay(
  timestamp: string | null | undefined,
  unavailableLabel: string,
  locale?: string | null,
  options?: {
    missingLabel?: string;
  },
): LocalDateTimeDisplay {
  if (!timestamp?.trim()) {
    return {
      label: options?.missingLabel ?? unavailableLabel,
      rawTimestamp: null,
    };
  }

  const normalizedTimestamp = normalizeValidTimestamp(timestamp);
  if (!normalizedTimestamp) {
    return {
      label: unavailableLabel,
      rawTimestamp: null,
    };
  }

  return {
    label: formatLocalDateTime(normalizedTimestamp, unavailableLabel, locale),
    rawTimestamp: normalizedTimestamp,
  };
}

export function formatLocalTimezoneContext(
  timezoneLabel: string,
  locale?: string | null,
): string {
  const timeZone =
    new Intl.DateTimeFormat(locale ?? undefined).resolvedOptions().timeZone ||
    "local timezone";

  return `${timezoneLabel}: ${timeZone}`;
}

export function formatWorkItemLabel(workItem: DashboardWorkItemRef): string {
  const displayName = workItem.display_name?.trim();
  if (displayName) {
    return displayName;
  }

  const workID = workItem.work_id?.trim();
  if (workID) {
    return workID;
  }

  return UNKNOWN_WORK_LABEL;
}

export function formatTypedWorkItemLabel(
  workItem: DashboardWorkItemRef,
): string {
  const name = formatWorkItemLabel(workItem).replace(/"/g, '\\"');
  const workType = workItem.work_type_id?.trim();

  if (!workType) {
    return `"${name}"`;
  }

  return `(${workType}):${name}`;
}

export function formatTraceOutcome(
  outcome: string,
  locale?: string | null,
): string {
  if (outcome === "") {
    return getSharedPrimitiveMessages(locale).formatTraceUnknownLabel;
  }

  return outcome
    .toLowerCase()
    .split("_")
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(" ");
}

export function getProviderSessionLogTarget(
  session: DashboardProviderSession | undefined,
  startedAt?: string,
): ProviderSessionLogTarget | null {
  const explicitURL = normalizeNonEmptyText(session?.session_log_url);
  if (explicitURL && isAllowedSessionLogURL(explicitURL)) {
    return {
      display: explicitURL,
      href: explicitURL,
    };
  }

  const localJSONLPath = normalizeNonEmptyText(session?.local_jsonl_path);
  if (localJSONLPath?.toLowerCase().endsWith(LOCAL_JSONL_EXTENSION)) {
    return {
      display: localJSONLPath,
      href: localPathToFileHref(localJSONLPath),
    };
  }

  const inferredSessionLogPath = inferCodexSessionLogPath(
    session?.id,
    startedAt,
  );
  if (!inferredSessionLogPath) {
    return null;
  }

  return {
    display: inferredSessionLogPath,
    href: localPathToFileHref(inferredSessionLogPath),
  };
}

export function formatList(
  values: string[] | undefined,
  locale?: string | null,
): string {
  if (!values || values.length === 0) {
    return getSharedPrimitiveMessages(locale).formatListEmptyLabel;
  }
  return values.join(", ");
}

function normalizeNonEmptyText(value: string | undefined): string | null {
  const trimmed = value?.trim();
  return trimmed && trimmed.length > 0 ? trimmed : null;
}

function normalizeValidTimestamp(timestamp?: string | null): string | null {
  const normalizedTimestamp = timestamp?.trim();
  if (!normalizedTimestamp) {
    return null;
  }

  return Number.isNaN(Date.parse(normalizedTimestamp))
    ? null
    : normalizedTimestamp;
}

function isAllowedSessionLogURL(value: string): boolean {
  try {
    const url = new URL(value);
    return SESSION_LOG_HREF_PROTOCOLS.has(url.protocol);
  } catch {
    return false;
  }
}

function localPathToFileHref(path: string): string {
  const normalizedPath = path.replace(/\\/g, "/");
  if (normalizedPath.startsWith("/")) {
    return `file://${encodeURI(normalizedPath)}`;
  }
  return `file:///${encodeURI(normalizedPath)}`;
}

function inferCodexSessionLogPath(
  sessionID: string | undefined,
  startedAt: string | undefined,
): string | null {
  const normalizedSessionID = normalizeNonEmptyText(sessionID);
  if (!normalizedSessionID || !startedAt) {
    return null;
  }

  const timestamp = new Date(startedAt);
  if (Number.isNaN(timestamp.getTime())) {
    return null;
  }

  const year = timestamp.getFullYear().toString().padStart(4, "0");
  const month = `${timestamp.getMonth() + 1}`.padStart(2, "0");
  const day = `${timestamp.getDate()}`.padStart(2, "0");
  return getSharedPrimitiveMessages()
    .sessionLogPathTemplate.replace("{{year}}", year)
    .replace("{{month}}", month)
    .replace("{{day}}", day)
    .replace("{{sessionID}}", normalizedSessionID);
}
