import type { ReactNode } from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../../api/dashboard/types";
import { parseFactoryGraphWorkTypeNodeId } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/lib/provider-session-ref";
import {
  CurrentSelectionHeaderActionProvider,
  CurrentSelectionLocaleProvider,
} from "../base/public";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { useEditableResourceConfigurationState } from "../resource-selection/hooks/use-editable-resource-configuration-state";
import { useSelectedProviderSessionState } from "../work-selection/hooks/useSelectedProviderSessionState";
import type {
  SelectedWorkItemExecutionDetails,
  SelectedWorkRelationshipGraph,
} from "../work-selection/public";
import { WorkItemDetailCard } from "../work-selection/public";
import { useEditableWorkerConfigurationState } from "../worker-selection/hooks/use-editable-worker-configuration-state";
import { useEditableWorkstationConfigurationState } from "../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { WorkstationDetailCard } from "../workstation-selection/public";
import {
  CurrentSelectionWorkstationSaveDialog,
  useCurrentSelectionDetailSave,
} from "./use-current-selection-detail-save";
import {
  NoSelectionDetailCard,
  ResourceDetailCard,
  StateNodeDetailCard,
  WorkerDetailCard,
  WorkstationRequestDetailCard,
  WorkTypeDetailCard,
} from "./current-selection-cards";

export interface CurrentSelectionWidgetProps {
  activeTraceID?: string | null;
  currentSelection: CurrentSelectionState;
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  headerAction?: ReactNode;
  locale?: string | null;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedTrace?: DashboardTrace;
  selectedProviderSessionKey?: string | null;
  selectedWorkRelationshipGraph?: SelectedWorkRelationshipGraph;
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  widgetId?: string;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: selection detail routing keeps one switch over dashboard selection kinds.
function renderCurrentSelectionDetailCard({
  activeTraceID,
  currentSelection,
  editableConfigurationState,
  editableResourceConfigurationState,
  editableWorkerConfigurationState,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  now,
  onSelectTraceID,
  resourceHeaderAction,
  resourceSaveState,
  saveState,
  workerHeaderAction,
  workerSaveState,
  selectedProviderSessionKey,
  selectedTrace,
  selectedWorkRelationshipGraph,
  selectedWorkExecutionDetails,
  setSelectedProviderSession,
  widgetId,
}: {
  activeTraceID?: string | null;
  currentSelection: CurrentSelectionState;
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  editableResourceConfigurationState: ReturnType<
    typeof useEditableResourceConfigurationState
  >;
  editableWorkerConfigurationState: ReturnType<
    typeof useEditableWorkerConfigurationState
  >;
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  headerAction: ReactNode;
  locale?: string;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  resourceHeaderAction: ReactNode;
  resourceSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["resourceSaveState"];
  saveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workstationSaveState"];
  workerHeaderAction: ReactNode;
  workerSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workerSaveState"];
  selectedProviderSessionKey: string | null;
  selectedTrace?: DashboardTrace;
  selectedWorkRelationshipGraph?: SelectedWorkRelationshipGraph;
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  setSelectedProviderSession: (session: LoadableProviderSessionRef) => void;
  widgetId: string;
}) {
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
    selectedWorkRequestHistory,
    selectedResourceName,
    selectedResourceTokenCount,
    selectedWorkerName,
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
        dispatchAttempts={selectedWorkDispatchAttempts}
        executionDetails={selectedWorkExecutionDetails}
        locale={locale}
        onSelectProviderSession={setSelectedProviderSession}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={selectWorkByID}
        relationshipGraph={selectedWorkRelationshipGraph}
        selectedNode={selectedNode}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedTrace={selectedTrace}
        selection={selection}
        traceTargetId="trace"
        widgetId={widgetId}
        workstationRequests={selectedWorkRequestHistory}
      />
    );
  }

  if (selectedWorkstationRequest) {
    return (
      <WorkstationRequestDetailCard
        onSelectWorkID={selectWorkByID}
        request={selectedWorkstationRequest}
        selectedWorkID={selectedWorkID}
        widgetId={widgetId}
      />
    );
  }

  if (selectedStatePlace) {
    return (
      <StateNodeDetailCard
        currentWorkItems={selectedStateCurrentWorkItems}
        failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
        locale={locale}
        onSelectWorkItem={(workItem) =>
          selectStateWorkItem(selectedStatePlace, workItem)
        }
        place={selectedStatePlace}
        terminalHistoryWorkItems={selectedStateTerminalHistoryWorkItems}
        tokenCount={selectedStateTokenCount}
        widgetId={widgetId}
      />
    );
  }

  if (selection?.kind === "worker" && selectedWorkerName) {
    return (
      <WorkerDetailCard
        editableConfigurationState={editableWorkerConfigurationState}
        headerAction={workerHeaderAction}
        locale={locale}
        saveState={workerSaveState}
        widgetId={widgetId}
        workerName={selectedWorkerName}
      />
    );
  }

  if (selection?.kind === "resource" && selectedResourceName) {
    return (
      <ResourceDetailCard
        editableConfigurationState={editableResourceConfigurationState}
        headerAction={resourceHeaderAction}
        locale={locale}
        resourceName={selectedResourceName}
        saveState={resourceSaveState}
        tokenCount={selectedResourceTokenCount}
        widgetId={widgetId}
      />
    );
  }

  const selectedWorkTypeName =
    selection?.kind === "node"
      ? parseFactoryGraphWorkTypeNodeId(selection.nodeId)
      : null;
  if (selectedWorkTypeName) {
    return (
      <WorkTypeDetailCard
        locale={locale}
        widgetId={widgetId}
        workTypeName={selectedWorkTypeName}
      />
    );
  }

  if (selectedNode) {
    return (
      <WorkstationDetailCard
        activeExecutions={selectedNodeActiveExecutions}
        editableConfigurationState={editableConfigurationState}
        headerAction={headerAction}
        locale={locale}
        now={now}
        onSelectProviderSession={setSelectedProviderSession}
        onSelectWorkID={selectWorkByID}
        onSelectWorkstationRequest={selectWorkstationRequest}
        providerSessions={selectedNodeProviderSessions}
        saveState={saveState}
        selectedNode={selectedNode}
        selectedProviderSessionKey={selectedProviderSessionKey}
        selectedRequest={selectedWorkstationRequest}
        selectedWorkID={selectedWorkID}
        workstationRequests={selectedNodeWorkstationRequests}
        widgetId={widgetId}
      />
    );
  }

  return <NoSelectionDetailCard widgetId={widgetId} />;
}

export function CurrentSelectionWidget({
  activeTraceID,
  currentSelection,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  now,
  onSelectTraceID,
  onSelectProviderSession,
  selectedTrace,
  selectedProviderSessionKey: controlledSelectedProviderSessionKey,
  selectedWorkRelationshipGraph,
  selectedWorkExecutionDetails,
  widgetId = "current-selection",
}: CurrentSelectionWidgetProps) {
  const {
    selectedNode,
    selectedNodeProviderSessions,
    selectedWorkDispatchAttempts,
    selectedWorkRequestHistory,
    selectedResourceName,
    selectedWorkerName,
    selection,
  } = currentSelection;
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
    locale,
  );
  const editableWorkerConfigurationState = useEditableWorkerConfigurationState(
    selection,
    selectedWorkerName,
    locale,
  );
  const editableResourceConfigurationState = useEditableResourceConfigurationState(
    selection,
    selectedResourceName,
    locale,
  );
  const {
    resourceHeaderAction,
    resourceSaveState,
    workstationHeaderAction,
    workstationSave,
    workstationSaveState,
    workerHeaderAction,
    workerSaveState,
  } = useCurrentSelectionDetailSave({
    currentSelection,
    editableConfigurationState,
    editableResourceConfigurationState,
    editableWorkerConfigurationState,
    locale,
    selectedNode,
    selectedResourceName,
    selectedWorkerName,
    selection,
  });
  const providerSessionState = useSelectedProviderSessionState({
    selectedNode,
    selectedNodeProviderSessions,
    selectedWorkDispatchAttempts,
    selectedWorkRequestHistory,
    selection,
  });
  const selectedProviderSessionKey =
    controlledSelectedProviderSessionKey ??
    providerSessionState.selectedProviderSessionKey;
  const handleSelectProviderSession =
    onSelectProviderSession ?? providerSessionState.setSelectedProviderSession;
  const detailCard = renderCurrentSelectionDetailCard({
    activeTraceID,
    currentSelection,
    editableConfigurationState,
    editableResourceConfigurationState,
    editableWorkerConfigurationState,
    failedWorkDetailsByWorkID,
    headerAction: workstationHeaderAction,
    locale: locale ?? undefined,
    now,
    onSelectTraceID,
    resourceHeaderAction,
    resourceSaveState,
    workerHeaderAction,
    saveState: workstationSaveState,
    selectedProviderSessionKey,
    workerSaveState,
    selectedTrace,
    selectedWorkRelationshipGraph,
    selectedWorkExecutionDetails,
    setSelectedProviderSession: handleSelectProviderSession,
    widgetId,
  });

  return (
    <CurrentSelectionLocaleProvider locale={locale ?? undefined}>
      <CurrentSelectionHeaderActionProvider headerAction={headerAction ?? null}>
        {detailCard}
      </CurrentSelectionHeaderActionProvider>
      <CurrentSelectionWorkstationSaveDialog
        editableConfigurationState={editableConfigurationState}
        locale={locale}
        workstationSave={workstationSave}
      />
    </CurrentSelectionLocaleProvider>
  );
}
