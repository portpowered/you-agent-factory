import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { DashboardActionButton } from "../../../../../components/ui";
import {
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../../../../components/ui/formatters";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionExecutionPill } from "../../../base/components/presentation/current-selection-pill";
import {
  CurrentSelectionSupportingText,
  CurrentSelectionWorkRow,
} from "../../../base/public";
import type { WorkstationActiveWorkListProps } from "../../lib/keys/detail-card-types";

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
                <CurrentSelectionWorkRow
                  actions={headerActions}
                  key={`${execution.dispatch_id}-${workIdentifier}`}
                  status={
                    <CurrentSelectionExecutionPill>
                      {messages.elapsedLabel}: {elapsed}
                    </CurrentSelectionExecutionPill>
                  }
                  supportingContent={
                    <>
                      {workItem ? null : (
                        <CurrentSelectionSupportingText tone="status">
                          {messages.workDetailsUnavailable(
                            execution.dispatch_id,
                          )}
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
                    </>
                  }
                  title={workLabel}
                />
              );
            });
          })}
        </ul>
      ) : (
        <WidgetDetailCopy>{messages.activeWorkEmpty}</WidgetDetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}
