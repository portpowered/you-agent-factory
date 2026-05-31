import type { ReactNode } from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../../api/dashboard/types";
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
import { useEditableWorkTypeConfigurationState } from "../work-type-selection/hooks/use-editable-work-type-configuration-state";
import { useSaveEditableWorkTypeConfiguration } from "../work-type-selection/hooks/use-save-editable-work-type-configuration";
import {
  EditableWorkTypeSaveDialog,
  EditableWorkTypeSaveHeaderAction,
} from "../work-type-selection/components/work-type-save-controls";
import { useEditableWorkerConfigurationState } from "../worker-selection/hooks/use-editable-worker-configuration-state";
import { useEditableWorkstationConfigurationState } from "../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { WorkstationDetailCard } from "../workstation-selection/public";
import { useEditableWorkStateConfigurationState } from "../work-state-selection/hooks/use-editable-work-state-configuration-state";
import { useSaveEditableWorkStateConfiguration } from "../work-state-selection/hooks/use-save-editable-work-state-configuration";
import { EditableWorkStateSaveHeaderAction } from "../work-state-selection/public";
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: selection routing keeps each detail card branch explicit for review.
function renderCurrentSelectionDetailCard({
  activeTraceID,
  currentSelection,
  editableConfigurationState,
  editableResourceConfigurationState,
  editableWorkStateConfigurationState,
  editableWorkerConfigurationState,
  editableWorkTypeConfigurationState,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  now,
  onSelectTraceID,
  resourceHeaderAction,
  resourceSaveState,
  saveState,
  workStateHeaderAction,
  workStateSaveState,
  workerHeaderAction,
  workTypeHeaderAction,
  workTypeSaveState,
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
  editableWorkStateConfigurationState: ReturnType<
    typeof useEditableWorkStateConfigurationState
  >;
  editableWorkerConfigurationState: ReturnType<
    typeof useEditableWorkerConfigurationState
  >;
  editableWorkTypeConfigurationState: ReturnType<
    typeof useEditableWorkTypeConfigurationState
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
  workStateHeaderAction: ReactNode;
  workStateSaveState: ReturnType<
    typeof useSaveEditableWorkStateConfiguration
  >["saveState"];
  workerHeaderAction: ReactNode;
  workTypeHeaderAction: ReactNode;
  workTypeSaveState: ReturnType<
    typeof useSaveEditableWorkTypeConfiguration
  >["saveState"];
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
    selectedWorkTypeName,
    selectedWorkstationRequest,
    selection,
    selectWorkByID,
    selectStateWorkItem,
    selectWorkstation,
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
        editableConfigurationState={editableWorkStateConfigurationState}
        failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
        headerAction={workStateHeaderAction}
        locale={locale}
        onSelectWorkItem={(workItem) =>
          selectStateWorkItem(selectedStatePlace, workItem)
        }
        place={selectedStatePlace}
        saveState={workStateSaveState}
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

  if (selection?.kind === "work-type" && selectedWorkTypeName) {
    return (
      <WorkTypeDetailCard
        editableConfigurationState={editableWorkTypeConfigurationState}
        headerAction={workTypeHeaderAction}
        locale={locale}
        onSelectWorkStateGraphNode={selectWorkstation}
        saveState={workTypeSaveState}
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: widget wires editable workstation, worker, resource, work-state, and work-type save hooks into one selection detail surface.
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
    selectedWorkTypeName,
    selection,
  } = currentSelection;
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
    locale,
  );
  const workStatePlaceId =
    selection?.kind === "state-node" ? selection.placeId : null;
  const editableWorkStateConfigurationState =
    useEditableWorkStateConfigurationState(selection, workStatePlaceId, locale);
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
  const editableWorkTypeConfigurationState =
    useEditableWorkTypeConfigurationState(
      selection,
      selectedWorkTypeName,
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
  const workStateSave = useSaveEditableWorkStateConfiguration({
    editableConfigurationState: editableWorkStateConfigurationState,
    locale,
    onWorkStateRenamed: currentSelection.selectStateNode,
    scopeKey: workStatePlaceId,
  });
  const workTypeSaveScopeKey =
    selection?.kind === "work-type" && selectedWorkTypeName
      ? selectedWorkTypeName
      : null;
  const workTypeSave = useSaveEditableWorkTypeConfiguration({
    editableConfigurationState: editableWorkTypeConfigurationState,
    locale,
    onWorkTypeRenamed: currentSelection.selectWorkType,
    scopeKey: workTypeSaveScopeKey,
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
  const workStateHeaderAction = (
    <EditableWorkStateSaveHeaderAction
      canSave={workStateSave.canSave}
      locale={locale ?? undefined}
      onClick={() => void workStateSave.save()}
      saveState={workStateSave.saveState}
    />
  );
  const workTypeHeaderAction = (
    <EditableWorkTypeSaveHeaderAction
      canSave={workTypeSave.canSave}
      locale={locale ?? undefined}
      onClick={workTypeSave.beginSaveConfirmation}
      saveState={workTypeSave.saveState}
    />
  );
  const detailCard = renderCurrentSelectionDetailCard({
    activeTraceID,
    currentSelection,
    editableConfigurationState,
    editableResourceConfigurationState,
    editableWorkStateConfigurationState,
    editableWorkerConfigurationState,
    editableWorkTypeConfigurationState,
    failedWorkDetailsByWorkID,
    headerAction: workstationHeaderAction,
    locale: locale ?? undefined,
    now,
    onSelectTraceID,
    resourceHeaderAction,
    resourceSaveState,
    workStateHeaderAction,
    workStateSaveState: workStateSave.saveState,
    workerHeaderAction,
    workTypeHeaderAction,
    saveState: workstationSaveState,
    selectedProviderSessionKey,
    workTypeSaveState: workTypeSave.saveState,
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
      <EditableWorkTypeSaveDialog
        locale={locale ?? undefined}
        onCancel={workTypeSave.cancelSaveConfirmation}
        onConfirm={() => void workTypeSave.confirmSave()}
        saveState={workTypeSave.saveState}
      />
    </CurrentSelectionLocaleProvider>
  );
}
