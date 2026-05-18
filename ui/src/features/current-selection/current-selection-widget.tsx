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
import { useEditableWorkstationConfigurationState } from "./use-editable-workstation-configuration-state";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";
import type { CurrentSelectionState } from "./useCurrentSelection";
import {
  EditableWorkstationSaveDialog,
  EditableWorkstationSaveHeaderAction,
} from "./workstation-save-controls";

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
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
  );
  const workstationSaveScopeKey =
    selection?.kind === "node" && selectedNode
      ? `${selectedNode.node_id}:${selectedNode.transition_id}:${selectedNode.workstation_name}`
      : null;
  const workstationSave = useSaveEditableWorkstationConfiguration({
    editableConfigurationState,
    scopeKey: workstationSaveScopeKey,
  });

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
        onSelectWorkID={selectWorkByID}
        request={selectedWorkstationRequest}
        selectedWorkID={selectedWorkID}
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
        editableConfigurationState={editableConfigurationState}
        headerAction={
          <EditableWorkstationSaveHeaderAction
            canSave={workstationSave.canSave}
            locale={locale ?? undefined}
            onClick={workstationSave.beginSaveConfirmation}
            saveState={workstationSave.saveState}
          />
        }
        locale={locale ?? undefined}
        now={now}
        onSelectWorkID={selectWorkByID}
        onSelectWorkstationRequest={selectWorkstationRequest}
        providerSessions={selectedNodeProviderSessions}
        saveState={workstationSave.saveState}
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
    <CurrentSelectionLocaleProvider locale={locale ?? undefined}>
      {detailCard}
      <EditableWorkstationSaveDialog
        locale={locale ?? undefined}
        onCancel={workstationSave.cancelSaveConfirmation}
        onConfirm={() => void workstationSave.confirmSave()}
        overwriteFieldNames={
          editableConfigurationState?.status === "ready"
            ? editableConfigurationState.overwriteFieldNames
            : []
        }
        saveState={workstationSave.saveState}
      />
    </CurrentSelectionLocaleProvider>
  );
}
