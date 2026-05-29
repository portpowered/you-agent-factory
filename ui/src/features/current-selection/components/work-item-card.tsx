import { WIDGET_SUBTITLE_CLASS } from "../../../components/ui/widget-frame";
import {
  formatList,
  formatWorkItemLabel,
} from "../../../components/ui/formatters";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import type { WorkItemDetailCardProps } from "./detail-card-types";
import { WorkRelationshipsSection } from "./work-item-relationship-graph";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

export function WorkItemDetailCard({
  activeTraceID,
  dispatchAttempts,
  executionDetails,
  locale,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  relationshipGraph,
  selectedNode,
  selectedProviderSessionKey,
  selection,
  workstationRequests,
  traceTargetId = "trace",
  widgetId = "current-selection",
}: WorkItemDetailCardProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>
        {formatWorkItemLabel(selection.workItem)}
      </p>
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
          <dd>{dispatchAttempts.length}</dd>
        </div>
      </dl>
      <WorkRelationshipsSection
        activeTraceID={activeTraceID}
        messages={messages}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        relationshipGraph={relationshipGraph}
        selectedWorkLabel={
          relationshipGraph?.status !== "loading" &&
          relationshipGraph?.selectedWork.label
            ? relationshipGraph.selectedWork.label
            : formatWorkItemLabel(selection.workItem)
        }
        traceTargetId={traceTargetId}
      />
      <SelectedWorkDispatchHistorySection
        activeTraceID={activeTraceID}
        currentDispatchID={selection.dispatchId}
        fallbackProviderSessions={dispatchAttempts}
        locale={locale}
        onSelectProviderSession={onSelectProviderSession}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        requests={workstationRequests}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedWorkID={selection.workItem.work_id}
        traceTargetId={traceTargetId}
        workstationKind={selectedNode?.workstation_kind}
      />
    </SelectionDetailLayout>
  );
}
