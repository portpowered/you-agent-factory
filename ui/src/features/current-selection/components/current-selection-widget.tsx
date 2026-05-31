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
import { useSelectedProviderSessionState } from "../work-selection/hooks/useSelectedProviderSessionState";
import type {
  SelectedWorkItemExecutionDetails,
  SelectedWorkRelationshipGraph,
} from "../work-selection/public";
import { WorkItemDetailCard } from "../work-selection/public";
import { useEditableWorkStateConfigurationState } from "../work-state-selection/hooks/use-editable-work-state-configuration-state";
import { useSaveEditableWorkStateConfiguration } from "../work-state-selection/hooks/use-save-editable-work-state-configuration";
import { EditableWorkStateSaveHeaderAction } from "../work-state-selection/public";
import {
  EditableWorkTypeSaveDialog,
  EditableWorkTypeSaveHeaderAction,
} from "../work-type-selection/components/work-type-save-controls";
import { useEditableWorkTypeConfigurationState } from "../work-type-selection/hooks/use-editable-work-type-configuration-state";
import { useSaveEditableWorkTypeConfiguration } from "../work-type-selection/hooks/use-save-editable-work-type-configuration";
import { useEditableWorkerConfigurationState } from "../worker-selection/hooks/use-editable-worker-configuration-state";
import { useSaveEditableWorkerConfiguration } from "../worker-selection/hooks/use-save-editable-worker-configuration";
import { EditableWorkerSaveHeaderAction } from "../worker-selection/public";
import { useEditableWorkstationConfigurationState } from "../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { useSaveEditableWorkstationConfiguration } from "../workstation-selection/hooks/use-save-editable-workstation-configuration";
import {
  EditableWorkstationSaveDialog,
  EditableWorkstationSaveHeaderAction,
  WorkstationDetailCard,
} from "../workstation-selection/public";
import {
  NoSelectionDetailCard,
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
  editableWorkStateConfigurationState,
  editableWorkerConfigurationState,
  editableWorkTypeConfigurationState,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  now,
  onSelectTraceID,
  saveState,
  workStateHeaderAction,
  workStateSaveState,
  onSaveWorkerConfiguration,
  onSaveWorkstationConfiguration,
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
  workStateHeaderAction: ReactNode;
  workStateSaveState: ReturnType<
    typeof useSaveEditableWorkStateConfiguration
  >["saveState"];
  onSaveWorkerConfiguration: () => void;
  onSaveWorkstationConfiguration: () => void;
  workerHeaderAction: ReactNode;
  workTypeHeaderAction: ReactNode;
  saveState: ReturnType<
    typeof useSaveEditableWorkstationConfiguration
  >["saveState"];
  workTypeSaveState: ReturnType<
    typeof useSaveEditableWorkTypeConfiguration
  >["saveState"];
  workerSaveState: ReturnType<
    typeof useSaveEditableWorkerConfiguration
  >["saveState"];
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
        onSaveConfiguration={onSaveWorkerConfiguration}
        saveState={workerSaveState}
        widgetId={widgetId}
        workerName={selectedWorkerName}
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
        onSaveConfiguration={onSaveWorkstationConfiguration}
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: widget wires editable workstation, worker, work-state, and work-type save hooks into one selection detail surface.
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
  const editableWorkTypeConfigurationState =
    useEditableWorkTypeConfigurationState(
      selection,
      selectedWorkTypeName,
      locale,
    );
  const workstationSaveScopeKey =
    selection?.kind === "node" && selectedNode
      ? `${selectedNode.node_id}:${selectedNode.transition_id}:${selectedNode.workstation_name}`
      : null;
  const workstationSave = useSaveEditableWorkstationConfiguration({
    editableConfigurationState,
    locale,
    scopeKey: workstationSaveScopeKey,
  });
  const workerSaveScopeKey =
    selection?.kind === "worker" && selectedWorkerName
      ? selectedWorkerName
      : null;
  const workStateSave = useSaveEditableWorkStateConfiguration({
    editableConfigurationState: editableWorkStateConfigurationState,
    locale,
    onWorkStateRenamed: currentSelection.selectStateNode,
    scopeKey: workStatePlaceId,
  });
  const workerSave = useSaveEditableWorkerConfiguration({
    editableConfigurationState: editableWorkerConfigurationState,
    locale,
    onWorkerRenamed: currentSelection.selectWorker,
    scopeKey: workerSaveScopeKey,
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
  const workstationHeaderAction = (
    <EditableWorkstationSaveHeaderAction
      canSave={workstationSave.canSave}
      locale={locale ?? undefined}
      onClick={workstationSave.beginSaveConfirmation}
      saveState={workstationSave.saveState}
    />
  );
  const workStateHeaderAction = (
    <EditableWorkStateSaveHeaderAction
      canSave={workStateSave.canSave}
      locale={locale ?? undefined}
      onClick={() => void workStateSave.save()}
      saveState={workStateSave.saveState}
    />
  );
  const workerHeaderAction = (
    <EditableWorkerSaveHeaderAction
      canSave={workerSave.canSave}
      locale={locale ?? undefined}
      onClick={() => void workerSave.save()}
      saveState={workerSave.saveState}
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
    editableWorkStateConfigurationState,
    editableWorkerConfigurationState,
    editableWorkTypeConfigurationState,
    failedWorkDetailsByWorkID,
    headerAction: workstationHeaderAction,
    locale: locale ?? undefined,
    now,
    onSelectTraceID,
    workStateHeaderAction,
    workStateSaveState: workStateSave.saveState,
    onSaveWorkerConfiguration: () => void workerSave.save(),
    onSaveWorkstationConfiguration: workstationSave.beginSaveConfirmation,
    workerHeaderAction,
    workTypeHeaderAction,
    saveState: workstationSave.saveState,
    selectedProviderSessionKey,
    workTypeSaveState: workTypeSave.saveState,
    workerSaveState: workerSave.saveState,
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
      <EditableWorkTypeSaveDialog
        locale={locale ?? undefined}
        onCancel={workTypeSave.cancelSaveConfirmation}
        onConfirm={() => void workTypeSave.confirmSave()}
        saveState={workTypeSave.saveState}
      />
    </CurrentSelectionLocaleProvider>
  );
}
