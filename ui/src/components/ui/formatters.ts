import type {
  DashboardProviderSession,
  DashboardTraceMutation,
  DashboardTraceToken,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

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

const TIME_OF_DAY_FORMATTER = new Intl.DateTimeFormat(undefined, {
  hour: "numeric",
  minute: "2-digit",
});

export function formatDurationMillis(durationMillis: number): string {
  const safeDurationMillis = Math.max(0, Math.floor(durationMillis));

  if (safeDurationMillis < 1000) {
    return `${safeDurationMillis}ms`;
  }

  const durationSeconds = Math.floor(safeDurationMillis / 1000);
  const hours = Math.floor(durationSeconds / 3600);
  const minutes = Math.floor((durationSeconds % 3600) / 60);
  const seconds = durationSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

export function formatDurationMillisVerbose(durationMillis: number): string {
  const safeDurationMillis = Math.max(0, Math.floor(durationMillis));

  if (safeDurationMillis < 1000) {
    return formatDurationUnit(safeDurationMillis, "millisecond");
  }

  const durationSeconds = Math.floor(safeDurationMillis / 1000);
  const hours = Math.floor(durationSeconds / 3600);
  const minutes = Math.floor((durationSeconds % 3600) / 60);
  const seconds = durationSeconds % 60;

  if (hours > 0) {
    return [formatDurationUnit(hours, "hour"), formatOptionalDurationUnit(minutes, "minute")]
      .filter((part): part is string => part !== null)
      .join(", ");
  }
  if (minutes > 0) {
    return [formatDurationUnit(minutes, "minute"), formatOptionalDurationUnit(seconds, "second")]
      .filter((part): part is string => part !== null)
      .join(", ");
  }
  return formatDurationUnit(durationSeconds, "second");
}

export function formatDurationFromISO(startedAt: string, now: number): string {
  const startedAtMs = Date.parse(startedAt);
  if (Number.isNaN(startedAtMs)) {
    return "Unavailable";
  }

  return formatDurationMillis(now - startedAtMs);
}

export function formatTimeOfDay(isoTimestamp: string): string {
  const timestampMs = Date.parse(isoTimestamp);
  if (Number.isNaN(timestampMs)) {
    return isoTimestamp;
  }

  return TIME_OF_DAY_FORMATTER.format(timestampMs).replace(/\s/g, "");
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

export function formatTraceToken(token: DashboardTraceToken): string {
  return `${token.work_id || token.token_id} · ${token.place_id}`;
}

export function formatTraceMutation(mutation: DashboardTraceMutation): string {
  const movement =
    mutation.from_place && mutation.to_place
      ? `${mutation.from_place} -> ${mutation.to_place}`
      : mutation.to_place || mutation.from_place || "place unchanged";

  const tokenLabel =
    mutation.resulting_token?.work_id || mutation.resulting_token?.token_id || mutation.token_id;

  if (mutation.reason) {
    return `${mutation.type} ${tokenLabel} (${movement}) · ${mutation.reason}`;
  }

  return `${mutation.type} ${tokenLabel} (${movement})`;
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

function formatDurationUnit(value: number, unit: string): string {
  return `${value} ${unit}${value === 1 ? "" : "s"}`;
}

function formatOptionalDurationUnit(value: number, unit: string): string | null {
  if (value === 0) {
    return null;
  }

  return formatDurationUnit(value, unit);
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
