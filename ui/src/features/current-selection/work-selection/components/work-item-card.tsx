import {
  formatList,
  formatWorkItemLabel,
} from "../../../../components/ui/formatters";
import { WidgetSubtitle } from "../../../../components/ui/widget-frame";
import {
  getWorkContentInspectMessages,
  WorkContentReadOnlyList,
} from "../../../work-content/public";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import { SelectedWorkDispatchHistorySection } from "../../dispatch-selection/public";
import type { WorkItemDetailCardProps } from "../lib/detail-card-types";
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
      <WidgetSubtitle>{formatWorkItemLabel(selection.workItem)}</WidgetSubtitle>
      <dl>
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
      </dl>
      <WorkContentReadOnlyList
        content={selection.workItem.content ?? []}
        messages={workContentMessages}
        payloadStatus={payloadStatus}
        reason={payloadReason}
      />
      <WorkRelationshipsSection
        activeTraceID={activeTraceID}
        locale={locale}
        messages={messages}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        relationshipGraph={relationshipGraph}
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
        workstationKind={selectedNode?.workstation_kind}
      />
    </SelectionDetailLayout>
  );
}
