import type { ReactNode } from "react";
import { Label, Text } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import type { WorkContent } from "../lib/work-content-types";
import {
  getWorkContentInspectMessages,
  type WorkContentInspectMessages,
} from "../messages/work-content";
import { WorkContentPartList } from "./work-content-part-list";

export interface WorkContentReadOnlyListProps {
  ariaLabel?: string;
  content?: WorkContent;
  landmark?: boolean;
  messages?: Partial<WorkContentInspectMessages>;
  payloadStatus?: string;
  reason?: string | null;
  showHeading?: boolean;
}

export function WorkContentReadOnlyList({
  ariaLabel,
  content = [],
  landmark = true,
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

  const contentBody = (
    <>
      {showHeading ? <Label>{messages.heading}</Label> : null}
      {body}
    </>
  );

  if (!landmark) {
    return <div className="grid gap-2">{contentBody}</div>;
  }

  return (
    <section aria-label={resolvedAriaLabel} className="grid gap-2">
      {contentBody}
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
    <Text
      className={cn(
        "m-0",
        tone === "warning"
          ? "text-on-warning-container"
          : "text-on-surface-variant",
      )}
      variant="supporting"
    >
      {children}
    </Text>
  );
}
