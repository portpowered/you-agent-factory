import {
  DashboardActionButton,
  DashboardActionRow,
  DashboardText,
} from "../../../../components/ui";
import {
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../../../components/ui/formatters";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import { CurrentSelectionExpandableSection } from "../../base/components/current-selection-expandable-section";
import { CurrentSelectionExecutionPill } from "../../base/components/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../base/public";
import type { WorkstationActiveWorkListProps } from "../lib/detail-card-types";

export function WorkstationActiveWorkList({
  executions,
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  selectedNodeID,
  selectedRequest,
  selectedWorkID,
  workstationRequestsByDispatchID,
}: WorkstationActiveWorkListProps) {
  const sectionId = `active-work-${selectedNodeID}`;

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      headingId={sectionId}
      title={messages.activeWorkHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {executions.length > 0 ? (
        <ul className="m-0 grid list-none gap-2.5 p-0">
          {executions.flatMap((execution) => {
            const workItems =
              execution.work_items && execution.work_items.length > 0
                ? execution.work_items
                : [undefined];

            return workItems.map((workItem) => {
              const request =
                workstationRequestsByDispatchID?.[execution.dispatch_id];
              const traceID = workItem?.trace_id ?? execution.trace_ids?.[0];
              const workIdentifier =
                workItem?.work_id ?? traceID ?? messages.unavailableValue;
              const workLabel = workItem
                ? formatWorkItemLabel(workItem)
                : messages.unknownActiveWorkLabel;
              const requestSelected =
                selectedRequest?.dispatch_id === execution.dispatch_id;
              const elapsed = formatDurationFromISO(execution.started_at, now);
              const workAction =
                workItem && onSelectWorkID ? (
                  <DashboardActionButton
                    aria-label={messages.selectWorkItemLabel(workLabel)}
                    aria-pressed={selectedWorkID === workItem.work_id}
                    onClick={() => onSelectWorkID(workItem.work_id)}
                    type="button"
                  >
                    {selectedWorkID === workItem.work_id
                      ? messages.workSelectedAction
                      : messages.openWorkItemAction}
                  </DashboardActionButton>
                ) : null;
              const requestAction = onSelectWorkstationRequest ? (
                request ? (
                  <DashboardActionButton
                    aria-label={messages.selectWorkstationRequestLabel(
                      request.dispatch_id,
                    )}
                    aria-pressed={requestSelected}
                    onClick={() => onSelectWorkstationRequest(request)}
                    type="button"
                  >
                    {requestSelected
                      ? messages.requestSelectedAction
                      : messages.openRequestDetailsAction}
                  </DashboardActionButton>
                ) : null
              ) : null;
              const headerActions =
                workAction || requestAction ? (
                  <>
                    {workAction}
                    {requestAction}
                  </>
                ) : undefined;

              return (
                <DashboardText
                  as="li"
                  className="grid min-w-0 gap-2 rounded-lg px-3 py-2"
                  key={`${execution.dispatch_id}-${workIdentifier}`}
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <strong className="min-w-0 flex-1 [overflow-wrap:anywhere]">
                      {workLabel}
                    </strong>
                    <DashboardActionRow
                      statuses={
                        <CurrentSelectionExecutionPill>
                          {messages.elapsedLabel}: {elapsed}
                        </CurrentSelectionExecutionPill>
                      }
                      statusesClassName="justify-end"
                      actions={headerActions}
                      actionsClassName="justify-end"
                      className="justify-end"
                    />
                  </div>
                  {workItem ? null : (
                    <CurrentSelectionSupportingText tone="status">
                      {messages.workDetailsUnavailable(execution.dispatch_id)}
                    </CurrentSelectionSupportingText>
                  )}
                  {requestSelected ? (
                    <CurrentSelectionSupportingText tone="status">
                      {messages.selectedRequestLabel(execution.dispatch_id)}
                    </CurrentSelectionSupportingText>
                  ) : null}
                  {onSelectWorkstationRequest ? (
                    request ? null : (
                      <CurrentSelectionSupportingText tone="status">
                        {messages.requestDetailsUnavailable(
                          execution.dispatch_id,
                        )}
                      </CurrentSelectionSupportingText>
                    )
                  ) : null}
                </DashboardText>
              );
            });
          })}
        </ul>
      ) : (
        <DetailCopy>{messages.activeWorkEmpty}</DetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}
