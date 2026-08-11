import {
  type FactorySessionSummary,
  FactorySessionsAPIError,
  type FactorySessionsAPIErrorTarget,
  type FactorySessionTarget,
} from "../../../api/factory-sessions";
import type { getHeaderControlsMessages } from "../messages/header-controls";

export const SESSION_TAB_PATH_MAX_LENGTH = 48;
export const CANONICAL_NESTED_FACTORY_DIR = "factory";

export type FactorySessionJourney = "open" | "new";

export type FolderValidationState =
  | { status: "idle" }
  | { status: "pending" }
  | { status: "error"; reason: FolderValidationErrorReason }
  | { status: "init_ready"; folderPath: string }
  | { status: "ready"; targets: FactorySessionTarget[] };

export type FolderValidationErrorReason =
  | "config_load_failed"
  | "required"
  | "missing"
  | "not_directory"
  | "not_runnable"
  | "open_no_target"
  | "new_target_exists"
  | "target_not_found"
  | "unreadable"
  | "unknown";

export function sessionTabLabel(session: FactorySessionSummary): string {
  const folderBasename = normalizeSessionLabelPart(
    basename(session.folderPath),
  );
  if (isCanonicalNestedFactorySession(session) && folderBasename) {
    return folderBasename;
  }

  const namedTarget =
    session.target.kind === "named" ? session.target.name : "";
  return (
    normalizeSessionLabelPart(namedTarget) ||
    normalizeSessionLabelPart(basename(session.factoryDir)) ||
    folderBasename ||
    normalizeSessionLabelPart(session.project) ||
    CANONICAL_NESTED_FACTORY_DIR
  );
}

export function initNewFactoryNestedPath(folderPath: string): string {
  const normalizedFolderPath = normalizePathForCompare(folderPath);
  if (!normalizedFolderPath) {
    return CANONICAL_NESTED_FACTORY_DIR;
  }
  return `${normalizedFolderPath}/${CANONICAL_NESTED_FACTORY_DIR}`;
}

export function isCanonicalNestedFactorySession(
  session: Pick<FactorySessionSummary, "folderPath" | "factoryDir">,
): boolean {
  const folderPath = normalizePathForCompare(session.folderPath);
  const factoryDir = normalizePathForCompare(session.factoryDir);
  if (!folderPath || !factoryDir || folderPath === factoryDir) {
    return false;
  }
  return factoryDir === `${folderPath}/${CANONICAL_NESTED_FACTORY_DIR}`;
}

export function sessionTabSecondaryPath(
  path: string,
  maxLength = SESSION_TAB_PATH_MAX_LENGTH,
): string {
  const normalizedPath = path.trim();
  if (normalizedPath.length <= maxLength) {
    return normalizedPath;
  }
  if (maxLength <= 0) {
    return "";
  }
  if (maxLength <= 3) {
    return normalizedPath.slice(-maxLength);
  }

  return `...${normalizedPath.slice(-(maxLength - 3))}`;
}

export function sessionCloseLabel(
  session: FactorySessionSummary,
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string {
  return replaceSessionLabelToken(
    messages.sessionTabCloseLabelTemplate,
    session,
  );
}

export function sessionStreamToggleLabel(
  session: FactorySessionSummary,
  paused: boolean,
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string {
  return replaceSessionLabelToken(
    paused
      ? messages.resumeSessionStreamLabelTemplate
      : messages.pauseSessionStreamLabelTemplate,
    session,
  );
}

export function sessionTabID(sessionTabsID: string, sessionID: string): string {
  return `${sessionTabsID}-tab-${sessionDOMIDFragment(sessionID)}`;
}

export function sessionPanelID(
  sessionTabsID: string,
  sessionID: string,
): string {
  return `${sessionTabsID}-panel-${sessionDOMIDFragment(sessionID)}`;
}

export function orderFactorySessions(
  sessions: FactorySessionSummary[],
  orderedSessionIDs: string[],
): FactorySessionSummary[] {
  const sessionByID = new Map(sessions.map((session) => [session.id, session]));
  const orderedSessions: FactorySessionSummary[] = [];

  for (const sessionID of orderedSessionIDs) {
    const session = sessionByID.get(sessionID);
    if (!session) {
      continue;
    }
    orderedSessions.push(session);
    sessionByID.delete(sessionID);
  }

  for (const session of sessions) {
    if (sessionByID.has(session.id)) {
      orderedSessions.push(session);
    }
  }

  return orderedSessions;
}

export function moveSessionTabOrder(
  orderedSessionIDs: string[],
  draggedSessionID: string,
  targetIndex: number,
): string[] {
  const currentIndex = orderedSessionIDs.indexOf(draggedSessionID);
  if (currentIndex === -1) {
    return orderedSessionIDs;
  }

  const clampedIndex = Math.max(
    0,
    Math.min(targetIndex, orderedSessionIDs.length),
  );
  const nextOrder = [...orderedSessionIDs];
  nextOrder.splice(currentIndex, 1);
  const insertionIndex =
    currentIndex < clampedIndex ? clampedIndex - 1 : clampedIndex;
  nextOrder.splice(insertionIndex, 0, draggedSessionID);
  return nextOrder;
}

export function normalizeFactorySessionsError(
  error: unknown,
): FactorySessionsAPIError {
  if (error instanceof FactorySessionsAPIError) {
    return error;
  }
  return new FactorySessionsAPIError(
    "The dashboard could not complete the factory session request.",
    {
      code: "INTERNAL_ERROR",
      responseBody: error,
    },
  );
}

export function folderValidationStatusMessage(
  validation: FolderValidationState,
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string | null {
  switch (validation.status) {
    case "pending":
      return messages.openSessionValidationPendingLabel;
    case "init_ready":
      return null;
    case "ready":
      return validation.targets.length > 1
        ? messages.openSessionLaunchReadyMultipleTargets
        : messages.openSessionLaunchReadySingleTarget;
    case "error":
      if (validation.reason === "config_load_failed") {
        return null;
      }
      return folderValidationErrorMessage(validation.reason, messages);
    default:
      return null;
  }
}

export function classifyFactorySessionFolderValidationError(
  error: FactorySessionsAPIError,
): FolderValidationErrorReason {
  if (error.code === "FACTORY_SESSION_CONFIG_LOAD_FAILED") {
    return "config_load_failed";
  }

  const targetedReason = classifyFactorySessionFolderValidationTarget(
    error.targets,
  );
  if (targetedReason !== null) {
    return targetedReason;
  }

  const message = error.message.trim();

  if (
    message === "folderPath is required" ||
    message === "factory session folder is required"
  ) {
    return "required";
  }
  if (
    message.includes("stat factory session folder") &&
    message.includes("no such file or directory")
  ) {
    return "missing";
  }
  if (message.includes("must be a directory")) {
    return "not_directory";
  }
  if (
    (message.includes("read factory session folder") ||
      message.includes("stat factory session folder")) &&
    message.includes("permission denied")
  ) {
    return "unreadable";
  }
  if (
    message.startsWith('factory session target "') &&
    message.endsWith('" was not found')
  ) {
    return "target_not_found";
  }
  if (message.includes("does not expose any runnable factory targets")) {
    return "not_runnable";
  }

  return "unknown";
}

function classifyFactorySessionFolderValidationTarget(
  targets: FactorySessionsAPIErrorTarget[] | undefined,
): FolderValidationErrorReason | null {
  const validationTarget = targets?.find((target) =>
    target.code.startsWith("factory.session.field."),
  );
  if (!validationTarget?.code) {
    return null;
  }

  switch (validationTarget.code.replace("factory.session.field.", "")) {
    case "required":
    case "missing":
    case "not_directory":
    case "not_runnable":
    case "target_not_found":
    case "unreadable":
      return validationTarget.code.replace(
        "factory.session.field.",
        "",
      ) as FolderValidationErrorReason;
    default:
      return "unknown";
  }
}

export function factorySessionTargetOptionValue(
  target: Pick<FactorySessionTarget, "ref">,
): string {
  if (target.ref.kind === "default") {
    return "default";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `named:${target.ref.name ?? ""}`;
}

export function selectedFactorySessionTarget(
  targets: FactorySessionTarget[],
  selectedTargetValue: string,
): FactorySessionTarget | null {
  return (
    targets.find(
      (target) =>
        factorySessionTargetOptionValue(target) === selectedTargetValue,
    ) ?? null
  );
}

function folderValidationErrorMessage(
  reason: Exclude<FolderValidationErrorReason, "config_load_failed">,
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string {
  switch (reason) {
    case "required":
      return messages.openSessionFolderRequiredError;
    case "missing":
      return messages.openSessionFolderMissingError;
    case "not_directory":
      return messages.openSessionFolderNotDirectoryError;
    case "not_runnable":
      return messages.openSessionFolderNotRunnableError;
    case "open_no_target":
      return messages.openFactoryNoExistingTargetError;
    case "new_target_exists":
      return messages.newFactoryExistingTargetError;
    case "target_not_found":
      return messages.openSessionOverrideNotFoundError;
    case "unreadable":
      return messages.openSessionFolderUnreadableError;
    default:
      return messages.openSessionFolderUnknownError;
  }
}

function basename(path: string): string {
  const segments = path.split(/[\\/]/).filter((segment) => segment.length > 0);
  return segments[segments.length - 1] ?? "";
}

function normalizePathForCompare(path: string): string {
  return path.trim().replace(/\\/g, "/").replace(/\/+$/, "");
}

function normalizeSessionLabelPart(value: string | undefined): string {
  return value?.trim() ?? "";
}

function sessionDOMIDFragment(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]+/g, "-");
}

function replaceSessionLabelToken(
  template: string,
  session: FactorySessionSummary,
): string {
  return template.replace("{{sessionLabel}}", sessionTabLabel(session));
}
