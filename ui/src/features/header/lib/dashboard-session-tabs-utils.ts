import {
  type FactorySessionSummary,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import type { getHeaderControlsMessages } from "../messages/header-controls";

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
