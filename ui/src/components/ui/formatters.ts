import type {
  DashboardProviderSession,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import {
  formatDateTime,
  formatDuration,
  formatTime,
} from "../../i18n/formatters";

const LOCAL_JSONL_EXTENSION = ".jsonl";
const RAW_REJECTED_OUTCOME = "REJECTED";
const REPEATER_WORKSTATION_KIND = "repeater";
const SESSION_LOG_HREF_PROTOCOLS = new Set(["file:", "http:", "https:"]);
const UNKNOWN_WORK_LABEL = "Unknown work";

export interface ProviderSessionLogTarget {
  display: string;
  href: string;
}

export interface WorkstationRunOutcomeDisplay {
  label: string;
  rawOutcomeLabel?: string;
}

export interface WorkstationRunOutcomeContext {
  workstationKind?: string;
}

export function formatDurationMillis(durationMillis: number): string {
  return formatDuration(durationMillis, "en", {
    style: "compact",
  });
}

export function formatDurationMillisVerbose(durationMillis: number): string {
  return formatDuration(durationMillis, "en", {
    style: "verbose",
  });
}

export function formatDurationFromISO(startedAt: string, now: number): string {
  const startedAtMs = Date.parse(startedAt);
  if (Number.isNaN(startedAtMs)) {
    return "Unavailable";
  }

  return formatDurationMillis(now - startedAtMs);
}

export function formatTimeOfDay(isoTimestamp: string): string {
  return formatTime(isoTimestamp, "en", {
    fallback: isoTimestamp,
  }).replace(/\s/g, "");
}

export function formatLocalDateTime(
  timestamp: string | undefined,
  unavailableLabel: string,
): string {
  const normalizedTimestamp = timestamp?.trim();
  if (!normalizedTimestamp) {
    return unavailableLabel;
  }

  const timestampMs = Date.parse(normalizedTimestamp);
  if (Number.isNaN(timestampMs)) {
    return unavailableLabel;
  }

  return formatDateTime(timestampMs, "en", {
    fallback: unavailableLabel,
  });
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

export function formatTypedWorkItemLabel(workItem: DashboardWorkItemRef): string {
  const name = formatWorkItemLabel(workItem).replace(/"/g, '\\"');
  const workType = workItem.work_type_id?.trim();

  if (!workType) {
    return `"${name}"`;
  }

  return `${workType}:"${name}"`;
}

export function formatTraceOutcome(outcome: string): string {
  if (outcome === "") {
    return "Unknown";
  }

  return outcome
    .toLowerCase()
    .split("_")
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(" ");
}

export function formatWorkstationRunOutcome(
  outcome: string,
  context: WorkstationRunOutcomeContext,
): WorkstationRunOutcomeDisplay {
  const trimmedOutcome = outcome.trim();
  const rawOutcome = trimmedOutcome.length > 0 ? trimmedOutcome : undefined;
  const workstationKind = context.workstationKind?.trim().toLowerCase();

  if (
    rawOutcome?.toUpperCase() === RAW_REJECTED_OUTCOME &&
    workstationKind === REPEATER_WORKSTATION_KIND
  ) {
    return {
      label: "Repeated work",
      rawOutcomeLabel: `Raw outcome: ${rawOutcome}`,
    };
  }

  return {
    label: formatTraceOutcome(rawOutcome ?? ""),
  };
}

export function formatProviderSession(session: DashboardProviderSession | undefined): string {
  if (!session?.id) {
    return "Unavailable";
  }

  const parts = [session.provider, session.kind].filter(
    (value): value is string => value !== undefined && value !== "",
  );
  if (parts.length === 0) {
    return session.id;
  }
  return `${parts.join(" / ")} / ${session.id}`;
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

  const inferredSessionLogPath = inferCodexSessionLogPath(session?.id, startedAt);
  if (!inferredSessionLogPath) {
    return null;
  }

  return {
    display: inferredSessionLogPath,
    href: localPathToFileHref(inferredSessionLogPath),
  };
}

export function formatList(values: string[] | undefined): string {
  if (!values || values.length === 0) {
    return "None";
  }
  return values.join(", ");
}

function normalizeNonEmptyText(value: string | undefined): string | null {
  const trimmed = value?.trim();
  return trimmed && trimmed.length > 0 ? trimmed : null;
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
  return `~/.codex/sessions/${year}/${month}/${day}/rollout-${normalizedSessionID}.jsonl`;
}
