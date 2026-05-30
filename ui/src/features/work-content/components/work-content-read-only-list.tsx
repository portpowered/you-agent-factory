import type { ReactNode } from "react";
import type { components } from "../../../api/generated/openapi";
import {
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  getWorkContentInspectMessages,
  type WorkContentInspectMessages,
} from "../messages/work-content";
import { WorkContentPartList } from "./work-content-part-list";

export type WorkContent = components["schemas"]["WorkContent"];

export interface WorkContentReadOnlyListProps {
  ariaLabel?: string;
  content?: WorkContent;
  messages?: Partial<WorkContentInspectMessages>;
  payloadStatus?: string;
  reason?: string | null;
  showHeading?: boolean;
}

export function WorkContentReadOnlyList({
  ariaLabel,
  content = [],
  messages: messageOverrides,
  payloadStatus,
  reason,
  showHeading = true,
}: WorkContentReadOnlyListProps) {
  const messages = {
    ...getWorkContentInspectMessages(),
    ...messageOverrides,
  };
  const resolvedAriaLabel = ariaLabel ?? messages.heading;
  const body = resolveWorkContentBody({
    content,
    messages,
    payloadStatus,
    reason,
  });

  return (
    <section aria-label={resolvedAriaLabel} className="grid gap-2">
      {showHeading ? (
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{messages.heading}</span>
      ) : null}
      {body}
    </section>
  );
}

function resolveWorkContentBody({
  content,
  messages,
  payloadStatus,
  reason,
}: {
  content: WorkContent;
  messages: WorkContentInspectMessages;
  payloadStatus?: string;
  reason?: string | null;
}): ReactNode {
  switch (payloadStatus) {
    case "ERROR":
      return (
        <StatusMessage tone="warning">
          {messages.error}
          {reason ? ` ${reason}` : ""}
        </StatusMessage>
      );
    case "LOADING":
      return <StatusMessage tone="muted">{messages.loading}</StatusMessage>;
    case "UNAVAILABLE":
      return (
        <StatusMessage tone="warning">
          {messages.unavailable}
          {reason ? ` ${reason}` : ""}
        </StatusMessage>
      );
    default:
      if (content.length === 0) {
        return <StatusMessage tone="muted">{messages.empty}</StatusMessage>;
      }

      return <WorkContentPartList content={content} />;
  }
}

function StatusMessage({
  children,
  tone,
}: {
  children: ReactNode;
  tone: "muted" | "warning";
}) {
  return (
    <p
      className={cn(
        "m-0",
        tone === "warning" ? "text-af-warning-text" : "text-af-text-muted",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
    >
      {children}
    </p>
  );
}
