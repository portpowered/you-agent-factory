import type { ReactNode } from "react";
import type { DashboardWorkItemRef } from "../../../api/dashboard/types";
import { formatWorkItemLabel } from "../../../components/ui/formatters";
import {
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  AuthoredBodyText,
  REQUEST_AUTHORED_TEXT_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import { useCurrentSelectionDetailMessages } from "./current-selection-locale";

interface WorkItemPayloadMessages {
  consumedPayloadEmpty: string;
  consumedPayloadError: string;
  consumedPayloadHeading: string;
  consumedPayloadLoading: string;
  consumedPayloadUnavailable: string;
  consumedWorkItemsLabel: string;
  selectWorkItemLabel: (workItemLabel: string) => string;
  stateLabel: string;
  workTypeLabel: string;
}

export type PayloadAwareWorkItem = DashboardWorkItemRef;

export function WorkItemPayloadList({
  messages,
  onSelectWorkID,
  selectedWorkID,
  workItems,
}: {
  messages?: WorkItemPayloadMessages;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID?: string | null;
  workItems: PayloadAwareWorkItem[];
}) {
  const fallbackMessages = useCurrentSelectionDetailMessages();
  const resolvedMessages = messages ?? fallbackMessages;

  if (workItems.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2">
      <span>{resolvedMessages.consumedWorkItemsLabel}</span>
      <div className="grid gap-3">
        {workItems.map((workItem) => {
          const workLabel = formatWorkItemLabel(workItem);
          const isSelected = selectedWorkID === workItem.work_id;
          const hasPayloadDetails = workItemHasPayloadDetails(workItem);

          return (
            <article
              className="grid gap-2 rounded-lg border border-af-border bg-af-surface-subtle p-3"
              key={workItem.work_id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <button
                  aria-label={resolvedMessages.selectWorkItemLabel(workLabel)}
                  aria-pressed={isSelected}
                  className={cn(
                    WORK_SELECTION_BUTTON_CLASS,
                    isSelected &&
                      "border-af-accent-border bg-af-accent-surface text-af-accent",
                  )}
                  onClick={() => onSelectWorkID?.(workItem.work_id)}
                  type="button"
                >
                  {workLabel}
                </button>
                {workItem.state ? (
                  <span
                    className={cn(
                      "text-af-text-muted",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {resolvedMessages.stateLabel}: {workItem.state}
                  </span>
                ) : null}
                {resolveWorkTypeID(workItem) ? (
                  <span
                    className={cn(
                      "text-af-text-muted",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {resolvedMessages.workTypeLabel}: {resolveWorkTypeID(workItem)}
                  </span>
                ) : null}
              </div>
              {hasPayloadDetails ? (
                <WorkItemPayloadDetails
                  messages={resolvedMessages}
                  workItem={workItem}
                />
              ) : null}
            </article>
          );
        })}
      </div>
    </div>
  );
}

function WorkItemPayloadDetails({
  messages,
  workItem,
}: {
  messages: WorkItemPayloadMessages;
  workItem: PayloadAwareWorkItem;
}) {
  const payloadStatus =
    workItem.payloadStatus ?? workItem.payload_status ?? undefined;
  const payloadReason =
    workItem.payloadUnavailableReason ?? workItem.payload_unavailable_reason;
  const content = workItem.content ?? [];

  let body: ReactNode = null;
  switch (payloadStatus) {
    case "ERROR":
      body = (
        <p
          className={cn(
            "m-0 text-af-warning-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.consumedPayloadError}
          {payloadReason ? ` ${payloadReason}` : ""}
        </p>
      );
      break;
    case "LOADING":
      body = (
        <p
          className={cn("m-0 text-af-text-muted", DASHBOARD_SUPPORTING_TEXT_CLASS)}
        >
          {messages.consumedPayloadLoading}
        </p>
      );
      break;
    case "UNAVAILABLE":
      body = (
        <p
          className={cn(
            "m-0 text-af-warning-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.consumedPayloadUnavailable}
          {payloadReason ? ` ${payloadReason}` : ""}
        </p>
      );
      break;
    default:
      if (content.length === 0) {
        body = (
          <p
            className={cn(
              "m-0 text-af-text-muted",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {messages.consumedPayloadEmpty}
          </p>
        );
      } else {
        body = (
          <div className="grid gap-2">
            {content.map((part, index) => renderContentPart(part, index))}
          </div>
        );
      }
      break;
  }

  return (
    <section
      aria-label={messages.consumedPayloadHeading}
      className="grid gap-2"
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
        {messages.consumedPayloadHeading}
      </span>
      {body}
    </section>
  );
}

function renderContentPart(
  part: NonNullable<PayloadAwareWorkItem["content"]>[number],
  index: number,
) {
  if (part.type === "text" || part.type === "TEXT") {
    return typeof part.text === "string" ? (
      <AuthoredBodyText key={`content-${index}`} value={part.text} />
    ) : null;
  }

  if (part.type === "JSON") {
    const value =
      typeof part.json === "string"
        ? part.json
        : JSON.stringify(part.json ?? null, null, 2);
    return (
      <pre className={REQUEST_AUTHORED_TEXT_CLASS} key={`content-${index}`}>
        <code>{value}</code>
      </pre>
    );
  }

  return (
      <div
        className={cn(
        "rounded-lg border border-af-border bg-af-surface-raised p-3 text-af-text-muted",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
      key={`content-${index}`}
    >
      {describeNonTextContentPart(part)}
    </div>
  );
}

function describeNonTextContentPart(
  part: NonNullable<PayloadAwareWorkItem["content"]>[number],
) {
  const file = "file" in part ? part.file : undefined;
  if (typeof file === "string" && file) {
    return `${String(part.type)}: ${file}`;
  }
  const label = "label" in part ? part.label : undefined;
  if (typeof label === "string" && label) {
    return `${String(part.type)}: ${label}`;
  }
  const contentType = "contentType" in part ? part.contentType : undefined;
  if (typeof contentType === "string" && contentType) {
    return `${String(part.type)} (${contentType})`;
  }
  return `${String(part.type)}`;
}

function resolveWorkTypeID(workItem: PayloadAwareWorkItem) {
  return workItem.work_type_id ?? workItem.workTypeId;
}

function workItemHasPayloadDetails(workItem: PayloadAwareWorkItem) {
  return Boolean(
    workItem.payloadStatus ||
      workItem.payload_status ||
      workItem.payloadUnavailableReason ||
      workItem.payload_unavailable_reason ||
      workItem.content?.length,
  );
}
