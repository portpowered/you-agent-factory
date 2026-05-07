import type { ReactNode } from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../api/dashboard/types";
import {
  NoSelectionDetailCard,
  StateNodeDetailCard,
  WorkItemDetailCard,
  WorkstationDetailCard,
  WorkstationRequestDetailCard,
} from "./current-selection-cards";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import type { CurrentSelectionState } from "./useCurrentSelection";

export interface CurrentSelectionWidgetProps {
  activeTraceID?: string | null;
  currentSelection: CurrentSelectionState;
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  locale?: string | null;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  selectedTrace?: DashboardTrace;
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  widgetId?: string;
}

export function CurrentSelectionWidget({
  activeTraceID,
  currentSelection,
  failedWorkDetailsByWorkID,
  locale,
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

  let detailCard: ReactNode;

  if (selection?.kind === "work-item" && selectedWorkExecutionDetails) {
    detailCard = (
      <WorkItemDetailCard
        activeTraceID={activeTraceID}
        executionDetails={selectedWorkExecutionDetails}
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
  } else if (selectedWorkstationRequest) {
    detailCard = (
      <WorkstationRequestDetailCard
        request={selectedWorkstationRequest}
        widgetId={widgetId}
      />
    );
  } else if (selectedStatePlace) {
    detailCard = (
      <StateNodeDetailCard
        currentWorkItems={selectedStateCurrentWorkItems}
        failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
        onSelectWorkItem={(workItem) =>
          selectStateWorkItem(selectedStatePlace, workItem)
        }
        place={selectedStatePlace}
        terminalHistoryWorkItems={selectedStateTerminalHistoryWorkItems}
        tokenCount={selectedStateTokenCount}
        widgetId={widgetId}
      />
    );
  } else if (selectedNode) {
    detailCard = (
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
  } else {
    detailCard = <NoSelectionDetailCard widgetId={widgetId} />;
  }

  return (
    <CurrentSelectionLocaleProvider locale={locale}>
      {detailCard}
    </CurrentSelectionLocaleProvider>
  );
}
