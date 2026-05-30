import {
  type FactorySessionSummary,
  FactorySessionsAPIError,
  type FactorySessionsAPIErrorTarget,
  type FactorySessionTarget,
} from "../../../api/factory-sessions";
import type { getHeaderControlsMessages } from "../messages/header-controls";

export const SESSION_TAB_PATH_MAX_LENGTH = 48;

export type FolderValidationState =
  | { status: "idle" }
  | { status: "pending" }
  | { status: "error"; reason: FolderValidationErrorReason }
  | { status: "init_ready"; folderPath: string }
  | { status: "ready"; targets: FactorySessionTarget[] };

export type FolderValidationErrorReason =
  | "required"
  | "missing"
  | "not_directory"
  | "not_runnable"
  | "target_not_found"
  | "unreadable"
  | "unknown";

export function sessionTabLabel(session: FactorySessionSummary): string {
  const namedTarget =
    session.target.kind === "named" ? session.target.name : "";
  return (
    normalizeSessionLabelPart(namedTarget) ||
    normalizeSessionLabelPart(basename(session.factoryDir)) ||
    normalizeSessionLabelPart(basename(session.folderPath)) ||
    normalizeSessionLabelPart(session.project) ||
    "factory"
  );
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
      return folderValidationErrorMessage(validation.reason, messages);
    default:
      return null;
  }
}

export function classifyFactorySessionFolderValidationError(
  error: FactorySessionsAPIError,
): FolderValidationErrorReason {
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
      return validationTarget.code.replace("factory.session.field.", "") as FolderValidationErrorReason;
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
  reason: FolderValidationErrorReason,
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
