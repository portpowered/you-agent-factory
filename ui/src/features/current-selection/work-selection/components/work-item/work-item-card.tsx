import {
  formatList,
  formatWorkItemLabel,
} from "../../../../../components/ui/formatters";
import { WorkContentReadOnlyList } from "../../../../work-content/components/work-content-read-only-list";
import { getWorkContentInspectMessages } from "../../../../work-content/messages/work-content";
import { SelectedWorkDispatchHistorySection } from "../../../dispatch-selection/components/dispatch-history/selected-work-dispatch-history";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionBodyLayout } from "../../../base/components/layout/current-selection-body-layout";
import { SelectionDetailLayout } from "../../../base/components/layout/current-selection-detail-layout";
import { useCurrentSelectionDispatchHistoryMessages } from "../../../base/components/presentation/current-selection-locale";
import type { WorkItemDetailCardProps } from "../../lib/detail-card-types";
import { WorkRelationshipsSection } from "./work-item-relationship-graph";

export function WorkItemDetailCard({
  activeTraceID,
  dispatchAttempts,
  executionDetails,
  locale,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  operationCount,
  operationHistory,
  relationshipGraph,
  selectedNode,
  selectedProviderSessionKey,
  selection,
  workstationRequests,
  widgetId = "current-selection",
}: WorkItemDetailCardProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const workContentMessages = getWorkContentInspectMessages(locale);
  const displayedOperationCount = operationCount ?? dispatchAttempts.length;
  const payloadStatus =
    selection.workItem.payloadStatus ?? selection.workItem.payload_status;
  const payloadReason =
    selection.workItem.payloadUnavailableReason ??
    selection.workItem.payload_unavailable_reason;

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <CurrentSelectionBodyLayout
        title={formatWorkItemLabel(selection.workItem)}
      >
        <CurrentSelectionExpandableSection
          contentId={`${widgetId}-work-item-summary-content`}
          defaultExpanded
          headingId={`${widgetId}-work-item-summary-heading`}
          title={messages.summaryHeading}
          toggleLabel={(expanded) =>
            expanded ? messages.collapseAction : messages.expandAction
          }
        >
          <CurrentSelectionDescriptionList>
            <div>
              <dt>{messages.workIdLabel}</dt>
              <dd>{selection.workItem.work_id}</dd>
            </div>
            <div>
              <dt>{messages.workTypeLabel}</dt>
              <dd>
                {selection.workItem.work_type_id ||
                  messages.currentSelectionUnavailableValue}
              </dd>
            </div>
            <div>
              <dt>{messages.workstationLabel}</dt>
              <dd>
                {selectedNode?.workstation_name ??
                  executionDetails.workstationName ??
                  messages.workstationUnavailableValue}
              </dd>
            </div>
            <div>
              <dt>{messages.runtimeLabelsLabel}</dt>
              <dd>
                {formatList(
                  selection.execution?.work_type_ids ??
                    [selection.workItem.work_type_id ?? ""].filter(Boolean),
                )}
              </dd>
            </div>
            <div>
              <dt>{messages.workstationDispatchesLabel}</dt>
              <dd>{displayedOperationCount}</dd>
            </div>
          </CurrentSelectionDescriptionList>
        </CurrentSelectionExpandableSection>
        <CurrentSelectionExpandableSection
          contentId={`${widgetId}-work-item-content-content`}
          defaultExpanded
          headingId={`${widgetId}-work-item-content-heading`}
          title={workContentMessages.heading}
          toggleLabel={(expanded) =>
            expanded ? messages.collapseAction : messages.expandAction
          }
        >
          <WorkContentReadOnlyList
            content={selection.workItem.content ?? []}
            landmark={false}
            messages={workContentMessages}
            payloadStatus={payloadStatus}
            reason={payloadReason}
            showHeading={false}
          />
        </CurrentSelectionExpandableSection>
        <WorkRelationshipsSection
          activeTraceID={activeTraceID}
          locale={locale}
          messages={messages}
          onSelectTraceID={onSelectTraceID}
          onSelectWorkID={onSelectWorkID}
          relationshipGraph={relationshipGraph}
          widgetId={widgetId}
        />
        <SelectedWorkDispatchHistorySection
          activeTraceID={activeTraceID}
          currentDispatchID={selection.dispatchId}
          fallbackProviderSessions={dispatchAttempts}
          locale={locale}
          onSelectProviderSession={onSelectProviderSession}
          onSelectTraceID={onSelectTraceID}
          onSelectWorkID={onSelectWorkID}
          operationHistory={operationHistory}
          requests={workstationRequests}
          selectedProviderSessionKey={selectedProviderSessionKey}
          selectedWorkID={selection.workItem.work_id}
          widgetId={widgetId}
          workstationKind={selectedNode?.workstation_kind}
        />
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
