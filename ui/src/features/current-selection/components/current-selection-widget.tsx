import type { ReactNode } from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../../api/dashboard/types";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/lib/provider-session-ref";
import { buildCurrentSelectionSaveToastMessages } from "../base/lib/build-current-selection-save-toast-messages";
import {
  CurrentSelectionHeaderActionProvider,
  CurrentSelectionLocaleProvider,
  CurrentSelectionSaveNotifications,
} from "../base/public";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { useEditableResourceConfigurationState } from "../resource-selection/hooks/use-editable-resource-configuration-state";
import { useSelectedProviderSessionState } from "../work-selection/hooks/useSelectedProviderSessionState";
import type {
  SelectedWorkItemExecutionDetails,
  SelectedWorkRelationshipGraph,
} from "../work-selection/public";
import { WorkItemDetailCard } from "../work-selection/public";
import { useEditableWorkStateConfigurationState } from "../work-state-selection/hooks/use-editable-work-state-configuration-state";
import { useEditableWorkTypeConfigurationState } from "../work-type-selection/hooks/use-editable-work-type-configuration-state";
import { useEditableWorkerConfigurationState } from "../worker-selection/hooks/use-editable-worker-configuration-state";
import { useEditableWorkstationConfigurationState } from "../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { WorkstationDetailCard } from "../workstation-selection/public";
import {
  NoSelectionDetailCard,
  ResourceDetailCard,
  StateNodeDetailCard,
  WorkerDetailCard,
  WorkstationRequestDetailCard,
  WorkTypeDetailCard,
} from "./current-selection-cards";
import {
  CurrentSelectionWorkstationSaveDialog,
  CurrentSelectionWorkTypeSaveDialog,
  useCurrentSelectionDetailSave,
} from "./use-current-selection-detail-save";

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
  onSaveWorkerConfiguration,
  onSaveWorkstationConfiguration,
  onSaveWorkStateConfiguration,
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
    typeof useCurrentSelectionDetailSave
  >["workStateSaveState"];
  onSaveWorkerConfiguration: () => void;
  onSaveWorkstationConfiguration: () => void;
  onSaveWorkStateConfiguration: () => void;
  workerHeaderAction: ReactNode;
  workTypeHeaderAction: ReactNode;
  workTypeSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workTypeSaveState"];
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
    selectedWorkOperationHistory,
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
        operationCount={
          selectedWorkOperationHistory?.length ??
          selectedWorkRequestHistory.length
        }
        operationHistory={selectedWorkOperationHistory}
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
        onSaveConfiguration={onSaveWorkStateConfiguration}
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
  const editableResourceConfigurationState =
    useEditableResourceConfigurationState(
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
    resourceSave,
    resourceSaveState,
    saveWorkerConfiguration,
    saveWorkStateConfiguration,
    workstationHeaderAction,
    workstationSave,
    workstationSaveState,
    workerHeaderAction,
    workerSave,
    workerSaveState,
    workStateHeaderAction,
    workStateSave,
    workStateSaveState,
    workTypeHeaderAction,
    workTypeSave,
    workTypeSaveState,
  } = useCurrentSelectionDetailSave({
    currentSelection,
    editableConfigurationState,
    editableResourceConfigurationState,
    editableWorkStateConfigurationState,
    editableWorkerConfigurationState,
    editableWorkTypeConfigurationState,
    locale,
    selectedNode,
    selectedResourceName,
    selectedWorkerName,
    selectedWorkTypeName,
    selection,
    workStatePlaceId,
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
    workStateSaveState,
    onSaveWorkerConfiguration: saveWorkerConfiguration,
    onSaveWorkstationConfiguration: workstationSave.beginSaveConfirmation,
    onSaveWorkStateConfiguration: saveWorkStateConfiguration,
    workerHeaderAction,
    workTypeHeaderAction,
    saveState: workstationSaveState,
    selectedProviderSessionKey,
    workTypeSaveState,
    workerSaveState,
    selectedTrace,
    selectedWorkRelationshipGraph,
    selectedWorkExecutionDetails,
    setSelectedProviderSession: handleSelectProviderSession,
    widgetId,
  });

  const resolvedLocale = locale ?? undefined;
  const workstationHasDraftChanges =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty;
  const workerHasDraftChanges =
    editableWorkerConfigurationState?.status === "ready" &&
    editableWorkerConfigurationState.isDirty;
  const resourceHasDraftChanges =
    editableResourceConfigurationState?.status === "ready" &&
    editableResourceConfigurationState.isDirty;
  const workTypeHasDraftChanges =
    editableWorkTypeConfigurationState?.status === "ready" &&
    editableWorkTypeConfigurationState.isDirty;
  const workStateHasDraftChanges =
    editableWorkStateConfigurationState?.status === "ready" &&
    editableWorkStateConfigurationState.isDirty;
  const workStateDisplayName =
    editableWorkStateConfigurationState?.status === "ready"
      ? editableWorkStateConfigurationState.draft.name.trim() ||
        editableWorkStateConfigurationState.originalStateName
      : workStatePlaceId ?? "";
  const workerDisplayName =
    editableWorkerConfigurationState?.status === "ready"
      ? editableWorkerConfigurationState.draft.name.trim() ||
        (selectedWorkerName ?? "")
      : (selectedWorkerName ?? "");
  const resourceDisplayName =
    editableResourceConfigurationState?.status === "ready"
      ? editableResourceConfigurationState.draft.name.trim() ||
        (selectedResourceName ?? "")
      : (selectedResourceName ?? "");
  const workTypeDisplayName =
    editableWorkTypeConfigurationState?.status === "ready"
      ? editableWorkTypeConfigurationState.draft.name.trim() ||
        (selectedWorkTypeName ?? "")
      : (selectedWorkTypeName ?? "");

  return (
    <CurrentSelectionLocaleProvider locale={resolvedLocale}>
      <CurrentSelectionHeaderActionProvider headerAction={headerAction ?? null}>
        {detailCard}
      </CurrentSelectionHeaderActionProvider>
      {selectedNode ? (
        <CurrentSelectionSaveNotifications
          documentSave={workstationSaveState}
          entityKind="workstation"
          hasDraftChanges={workstationHasDraftChanges}
          locale={resolvedLocale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: selectedNode.workstation_name,
            entityKind: "workstation",
            locale,
          })}
          saveAttemptRevision={workstationSave.saveAttemptRevision}
          saveMutationError={workstationSave.saveMutationError}
        />
      ) : null}
      {selection?.kind === "worker" && selectedWorkerName ? (
        <CurrentSelectionSaveNotifications
          documentSave={workerSaveState}
          entityKind="worker"
          hasDraftChanges={workerHasDraftChanges}
          locale={resolvedLocale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: workerDisplayName,
            entityKind: "worker",
            locale,
          })}
          saveAttemptRevision={workerSave.saveAttemptRevision}
          saveMutationError={workerSave.saveMutationError}
        />
      ) : null}
      {selection?.kind === "resource" && selectedResourceName ? (
        <CurrentSelectionSaveNotifications
          documentSave={resourceSaveState}
          entityKind="resource"
          hasDraftChanges={resourceHasDraftChanges}
          locale={resolvedLocale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: resourceDisplayName,
            entityKind: "resource",
            locale,
          })}
          saveAttemptRevision={resourceSave.saveAttemptRevision}
          saveMutationError={resourceSave.saveMutationError}
        />
      ) : null}
      {selection?.kind === "work-type" && selectedWorkTypeName ? (
        <CurrentSelectionSaveNotifications
          documentSave={workTypeSave.saveState}
          entityKind="work-type"
          hasDraftChanges={workTypeHasDraftChanges}
          locale={resolvedLocale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: workTypeDisplayName,
            entityKind: "work-type",
            locale,
          })}
          saveAttemptRevision={workTypeSave.saveAttemptRevision}
          saveMutationError={workTypeSave.saveMutationError}
        />
      ) : null}
      {selection?.kind === "state-node" && workStatePlaceId ? (
        <CurrentSelectionSaveNotifications
          documentSave={workStateSave.saveState}
          entityKind="work-state"
          hasDraftChanges={workStateHasDraftChanges}
          locale={resolvedLocale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: workStateDisplayName,
            entityKind: "work-state",
            locale,
          })}
          saveAttemptRevision={workStateSave.saveAttemptRevision}
          saveMutationError={workStateSave.saveMutationError}
        />
      ) : null}
      <CurrentSelectionWorkstationSaveDialog
        editableConfigurationState={editableConfigurationState}
        locale={locale}
        workstationSave={workstationSave}
      />
      <CurrentSelectionWorkTypeSaveDialog
        locale={locale}
        workTypeSave={workTypeSave}
      />
    </CurrentSelectionLocaleProvider>
  );
}
