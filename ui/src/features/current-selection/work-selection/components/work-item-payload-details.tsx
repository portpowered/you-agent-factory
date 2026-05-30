import type { DashboardWorkItemRef } from "../../../../api/dashboard/types";
import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { formatWorkItemLabel } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import { WorkContentReadOnlyList } from "../../../work-content/public";
import { useCurrentSelectionDetailMessages } from "../../base/components/current-selection-locale";
import { WORK_SELECTION_BUTTON_CLASS } from "../../base/components/detail-card-shared";

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
                    {resolvedMessages.workTypeLabel}:{" "}
                    {resolveWorkTypeID(workItem)}
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

  return (
    <WorkContentReadOnlyList
      ariaLabel={messages.consumedPayloadHeading}
      content={workItem.content ?? []}
      messages={{
        empty: messages.consumedPayloadEmpty,
        error: messages.consumedPayloadError,
        heading: messages.consumedPayloadHeading,
        loading: messages.consumedPayloadLoading,
        unavailable: messages.consumedPayloadUnavailable,
      }}
      payloadStatus={payloadStatus}
      reason={payloadReason}
    />
  );
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
