import type { DashboardWorkItemRef } from "../../../../../api/dashboard/types";
import {
  DashboardText,
  surfacePanelVariants,
} from "../../../../../components/ui";
import { formatWorkItemLabel } from "../../../../../components/ui/formatters";
import { WorkContentReadOnlyList } from "../../../../work-content/public";
import { useCurrentSelectionDetailMessages } from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionSelectableButton } from "../../../base/components/presentation/current-selection-selectable-button";
import { CurrentSelectionLabel } from "../../../base/public";

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
type WorkItemPayloadListVariant = "panel" | "plain";

export function WorkItemPayloadList({
  messages,
  onSelectWorkID,
  selectedWorkID,
  variant = "panel",
  workItems,
}: {
  messages?: WorkItemPayloadMessages;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID?: string | null;
  variant?: WorkItemPayloadListVariant;
  workItems: PayloadAwareWorkItem[];
}) {
  const fallbackMessages = useCurrentSelectionDetailMessages();
  const resolvedMessages = messages ?? fallbackMessages;

  if (workItems.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2">
      <CurrentSelectionLabel>
        {resolvedMessages.consumedWorkItemsLabel}
      </CurrentSelectionLabel>
      <div className="grid gap-3">
        {workItems.map((workItem) => {
          const workLabel = formatWorkItemLabel(workItem);
          const isSelected = selectedWorkID === workItem.work_id;
          const hasPayloadDetails = workItemHasPayloadDetails(workItem);

          return (
            <article
              className={
                variant === "panel"
                  ? surfacePanelVariants({
                      className: "grid gap-2",
                      padding: "default",
                      radius: "lg",
                    })
                  : "grid gap-2"
              }
              key={workItem.work_id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <CurrentSelectionSelectableButton
                  aria-label={resolvedMessages.selectWorkItemLabel(workLabel)}
                  onClick={() => onSelectWorkID?.(workItem.work_id)}
                  selected={isSelected}
                  selectedStyle="outline"
                >
                  {workLabel}
                </CurrentSelectionSelectableButton>
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

function _resolveWorkTypeID(workItem: PayloadAwareWorkItem) {
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
