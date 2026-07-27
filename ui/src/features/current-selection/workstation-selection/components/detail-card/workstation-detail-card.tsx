import { CurrentSelectionBodyLayout } from "../../../base/components/layout/current-selection-body-layout";
import { SelectionDetailLayout } from "../../../base/components/layout/current-selection-detail-layout";
import type { WorkstationDetailCardProps } from "../../lib/keys/detail-card-types";
import { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import {
  EditableConfigurationSection,
  WorkstationSummary,
} from "../editable/workstation-editable-configuration-section";
import { WorkstationActiveWorkList } from "./workstation-active-work-list";
import { WorkstationHistorySection } from "./workstation-history-section";

export function WorkstationDetailCard({
  activeExecutions,
  editableConfigurationState,
  headerAction,
  locale,
  now,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  providerSessions,
  saveState,
  selectedProviderSessionKey,
  selectedNode,
  selectedRequest,
  selectedWorkID,
  workstationRequests = [],
  widgetId = "current-selection",
}: WorkstationDetailCardProps) {
  const messages = getWorkstationDetailMessages(locale);
  const hasProjectedRequestHistory = workstationRequests.length > 0;
  const workstationRequestsByDispatchID = Object.fromEntries(
    workstationRequests.map((request) => [request.dispatch_id, request]),
  );

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={selectedNode.workstation_name}>
        <WorkstationSummary
          activeRunCount={activeExecutions.length}
          editableConfigurationState={editableConfigurationState}
          historyCount={
            hasProjectedRequestHistory
              ? workstationRequests.length
              : providerSessions.length
          }
          historyLabel={
            hasProjectedRequestHistory
              ? messages.historicalRequestsLabel
              : messages.historicalRunsLabel
          }
          locale={locale}
          messages={messages}
          selectedNode={selectedNode}
        />
        <EditableConfigurationSection
          key={`editable-configuration:${selectedNode.node_id}`}
          messages={messages}
          saveState={saveState}
          state={editableConfigurationState}
        />
        <WorkstationActiveWorkList
          executions={activeExecutions}
          messages={messages}
          now={now}
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          selectedNodeID={selectedNode.node_id}
          selectedRequest={selectedRequest}
          selectedWorkID={selectedWorkID}
          workstationRequestsByDispatchID={workstationRequestsByDispatchID}
        />
        <WorkstationHistorySection
          collapseActionLabel={messages.collapseAction}
          expandActionLabel={messages.expandAction}
          messages={messages}
          now={now}
          onSelectProviderSession={onSelectProviderSession}
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          providerSessions={providerSessions}
          selectedNodeID={selectedNode.node_id}
          selectedProviderSessionKey={selectedProviderSessionKey}
          selectedRequest={selectedRequest}
          selectedWorkID={selectedWorkID}
          workstationKind={selectedNode.workstation_kind}
          workstationRequests={workstationRequests}
          workstationRequestsByDispatchID={workstationRequestsByDispatchID}
        />
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
