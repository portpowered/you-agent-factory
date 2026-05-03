import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../api/dashboard/types";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import {
  NoSelectionDetailCard,
  StateNodeDetailCard,
  WorkItemDetailCard,
  WorkstationDetailCard,
  WorkstationRequestDetailCard,
} from "./current-selection-cards";
import type { CurrentSelectionState } from "./useCurrentSelection";

export interface CurrentSelectionWidgetProps {
  activeTraceID?: string | null;
  currentSelection: CurrentSelectionState;
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  selectedTrace?: DashboardTrace;
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  terminalWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  widgetId?: string;
}

export function CurrentSelectionWidget({
  activeTraceID,
  currentSelection,
  failedWorkDetailsByWorkID,
  now,
  onSelectTraceID,
  selectedTrace,
  selectedWorkExecutionDetails,
  widgetId = "current-selection",
}: CurrentSelectionWidgetProps) {
  const {
    selectedNode,
    selectedNodeActiveExecutions,
    selectedNodeProviderSessions,
    selectedNodeWorkstationRequests,
    selectedStateCurrentWorkItems,
    selectedStatePlace,
    selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount,
    selectedWorkDispatchAttempts,
    selectedWorkID,
    selectedWorkstationRequest,
    selection,
    selectWorkByID,
    selectStateWorkItem,
    selectWorkstationRequest,
  } = currentSelection;

  if (selection?.kind === "work-item" && selectedWorkExecutionDetails) {
    return (
      <WorkItemDetailCard
        activeTraceID={activeTraceID}
        executionDetails={selectedWorkExecutionDetails}
        failureMessage={
          currentSelection.terminalWorkDetail?.traceWorkID === selection.workItem.work_id
            ? currentSelection.terminalWorkDetail.failureMessage
            : failedWorkDetailsByWorkID?.[selection.workItem.work_id]?.failure_message
        }
        failureReason={
          currentSelection.terminalWorkDetail?.traceWorkID === selection.workItem.work_id
            ? currentSelection.terminalWorkDetail.failureReason
            : failedWorkDetailsByWorkID?.[selection.workItem.work_id]?.failure_reason
        }
        now={now}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={selectWorkByID}
        dispatchAttempts={selectedWorkDispatchAttempts}
        selectedNode={selectedNode}
        selection={selection}
        selectedTrace={selectedTrace}
        workstationRequests={currentSelection.selectedWorkRequestHistory}
        widgetId={widgetId}
      />
    );
  }

  if (selectedWorkstationRequest) {
    return (
      <WorkstationRequestDetailCard
        request={selectedWorkstationRequest}
        widgetId={widgetId}
      />
    );
  }

  if (selectedStatePlace) {
    return (
      <StateNodeDetailCard
        currentWorkItems={selectedStateCurrentWorkItems}
        failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
        onSelectWorkItem={(workItem) => selectStateWorkItem(selectedStatePlace, workItem)}
        place={selectedStatePlace}
        terminalHistoryWorkItems={selectedStateTerminalHistoryWorkItems}
        tokenCount={selectedStateTokenCount}
        widgetId={widgetId}
      />
    );
  }

  if (selectedNode) {
    return (
      <WorkstationDetailCard
        activeExecutions={selectedNodeActiveExecutions}
        now={now}
        onSelectWorkID={selectWorkByID}
        onSelectWorkstationRequest={selectWorkstationRequest}
        providerSessions={selectedNodeProviderSessions}
        selectedNode={selectedNode}
        selectedRequest={selectedWorkstationRequest}
        selectedWorkID={selectedWorkID}
        workstationRequests={selectedNodeWorkstationRequests}
        widgetId={widgetId}
      />
    );
  }

  return <NoSelectionDetailCard widgetId={widgetId} />;
}

