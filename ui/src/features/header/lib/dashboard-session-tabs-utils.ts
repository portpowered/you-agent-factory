import {
  type FactorySessionTarget,
  type FactorySessionSummary,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import type { getHeaderControlsMessages } from "../messages/header-controls";

export type FolderValidationState =
  | { status: "idle" }
  | { status: "pending" }
  | { status: "error"; reason: FolderValidationErrorReason }
  | { status: "ready"; targets: FactorySessionTarget[] };

export type FolderValidationErrorReason =
  | "required"
  | "missing"
  | "not_directory"
  | "not_runnable"
  | "unreadable"
  | "unknown";

export function sessionTabLabel(session: FactorySessionSummary): string {
  const namedTarget = session.target.kind === "named" ? session.target.name : "";
  return (
    normalizeSessionLabelPart(namedTarget) ||
    normalizeSessionLabelPart(basename(session.factoryDir)) ||
    normalizeSessionLabelPart(basename(session.folderPath)) ||
    normalizeSessionLabelPart(session.project) ||
    "factory"
  );
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

export function sessionPanelID(sessionTabsID: string, sessionID: string): string {
  return `${sessionTabsID}-panel-${sessionDOMIDFragment(sessionID)}`;
}

export function normalizeFactorySessionsError(error: unknown): FactorySessionsAPIError {
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
  const message = error.message.trim();

  if (message === "folderPath is required" || message === "factory session folder is required") {
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
  if (message.includes("does not expose any runnable factory targets")) {
    return "not_runnable";
  }

  return "unknown";
}

export function factorySessionTargetOptionValue(
  target: Pick<FactorySessionTarget, "ref">,
): string {
  return target.ref.kind === "default"
    ? "default"
    : `named:${target.ref.name ?? ""}`;
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
