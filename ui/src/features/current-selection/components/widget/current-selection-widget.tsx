// biome-ignore lint/style/noExcessiveLinesPerFile: widget wires all current-selection detail cards and save hooks in one mount surface.
import type { ReactNode } from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../../../api/dashboard/types";
import type { FactoryGraphBulkSelectionSummary } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";
import { useFactoryGraphEditorSelectionBridge } from "../../../workflow-activity/state/factory-graph-editor-selection-bridge";
import { CurrentSelectionHeaderActionProvider } from "../../base/components/layout/current-selection-detail-layout";
import { CurrentSelectionLocaleProvider } from "../../base/components/presentation/current-selection-locale";
import { useEditableDocConfigurationState } from "../../doc-selection/hooks/use-editable-doc-configuration-state";
import { GraphBulkSelectionDetailCard } from "../../graph-selection/components/graph-bulk-selection-detail-card";
import { resolveActiveGraphBulkSelectionSummary } from "../../graph-selection/lib/resolve-active-graph-bulk-selection-summary";
import type { CurrentSelectionState } from "../../hooks/core/useCurrentSelection";
import { useEditableResourceConfigurationState } from "../../resource-selection/hooks/use-editable-resource-configuration-state";
import { useSelectedProviderSessionState } from "../../work-selection/hooks/useSelectedProviderSessionState";
import { WorkItemDetailCard } from "../../work-selection/components/work-item/work-item-card";
import type { SelectedWorkItemExecutionDetails } from "../../work-selection/state/executionDetails";
import type { SelectedWorkRelationshipGraph } from "../../work-selection/lib/selected-work-relationship-graph";
import { useEditableWorkStateConfigurationState } from "../../work-state-selection/hooks/use-editable-work-state-configuration-state";
import { useEditableWorkTypeConfigurationState } from "../../work-type-selection/hooks/use-editable-work-type-configuration-state";
import { useEditableWorkerConfigurationState } from "../../worker-selection/hooks/use-editable-worker-configuration-state";
import { useEditableWorkstationConfigurationState } from "../../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { WorkstationDetailCard } from "../../workstation-selection/components/detail-card/workstation-detail-card";
import {
  DocDetailCard,
  NoSelectionDetailCard,
  ResourceDetailCard,
  StateNodeDetailCard,
  WorkerDetailCard,
  WorkstationRequestDetailCard,
  WorkTypeDetailCard,
} from "./current-selection-cards";
import { CurrentSelectionWidgetSaveNotifications } from "./current-selection-widget-save-notifications";
import {
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

function renderGraphBulkSelectionDetailCard({
  bulkSelectionSummary,
  locale,
  widgetId,
}: {
  bulkSelectionSummary: FactoryGraphBulkSelectionSummary | null;
  locale?: string;
  widgetId: string;
}) {
  if (!bulkSelectionSummary) {
    return null;
  }

  return (
    <GraphBulkSelectionDetailCard
      locale={locale}
      summary={bulkSelectionSummary}
      widgetId={widgetId}
    />
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: selection routing keeps each detail card branch explicit for review.
function renderCurrentSelectionDetailCard({
  activeTraceID,
  currentSelection,
  docHeaderAction,
  docSaveState,
  editableConfigurationState,
  editableDocConfigurationState,
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
  docHeaderAction: ReactNode;
  docSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["docSaveState"];
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  editableDocConfigurationState: ReturnType<
    typeof useEditableDocConfigurationState
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
    selectedDocBundledFile,
    selectedDocTargetPath,
    selectedResource,
    selectedResourceName,
    selectedResourceTokenCount,
    selectedResourceWorkerNames,
    selectedResourceWorkstationNames,
    selectedWorker,
    selectedWorkerName,
    selectedWorkerWorkstationNames,
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

  if (selection?.kind === "doc" && selectedDocTargetPath) {
    return (
      <DocDetailCard
        editableConfigurationState={editableDocConfigurationState}
        headerAction={docHeaderAction}
        locale={locale}
        saveState={docSaveState}
        savedBundledDoc={selectedDocBundledFile}
        targetPath={selectedDocTargetPath}
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
        worker={selectedWorker}
        workerName={selectedWorkerName}
        workstationNames={selectedWorkerWorkstationNames}
      />
    );
  }

  if (selection?.kind === "resource" && selectedResourceName) {
    return (
      <ResourceDetailCard
        editableConfigurationState={editableResourceConfigurationState}
        headerAction={resourceHeaderAction}
        locale={locale}
        resource={selectedResource}
        resourceName={selectedResourceName}
        saveState={resourceSaveState}
        tokenCount={selectedResourceTokenCount}
        widgetId={widgetId}
        workerNames={selectedResourceWorkerNames}
        workstationNames={selectedResourceWorkstationNames}
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
  const graphSelectionBridge = useFactoryGraphEditorSelectionBridge(
    (state) => state.selection,
  );
  const graphBulkSelectionSummary = resolveActiveGraphBulkSelectionSummary(
    currentSelection.selection,
    graphSelectionBridge,
  );
  const {
    currentFactoryDefinition,
    selectedNode,
    selectedNodeProviderSessions,
    selectedWorkDispatchAttempts,
    selectedWorkRequestHistory,
    selectedDocTargetPath,
    selectedResourceName,
    selectedWorkerName,
    selectedWorkTypeName,
    selection,
  } = currentSelection;
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
    locale,
    currentFactoryDefinition,
  );
  const workStatePlaceId =
    selection?.kind === "state-node" ? selection.placeId : null;
  const editableWorkStateConfigurationState =
    useEditableWorkStateConfigurationState(
      selection,
      workStatePlaceId,
      locale,
      currentFactoryDefinition,
    );
  const editableWorkerConfigurationState = useEditableWorkerConfigurationState(
    selection,
    selectedWorkerName,
    locale,
    currentFactoryDefinition,
  );
  const editableDocConfigurationState = useEditableDocConfigurationState(
    selection,
    selectedDocTargetPath,
    locale,
    currentFactoryDefinition,
  );
  const editableResourceConfigurationState =
    useEditableResourceConfigurationState(
      selection,
      selectedResourceName,
      locale,
      currentFactoryDefinition,
    );
  const editableWorkTypeConfigurationState =
    useEditableWorkTypeConfigurationState(
      selection,
      selectedWorkTypeName,
      locale,
      currentFactoryDefinition,
    );
  const {
    docHeaderAction,
    docSave,
    docSaveState,
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
    editableDocConfigurationState,
    editableResourceConfigurationState,
    editableWorkStateConfigurationState,
    editableWorkerConfigurationState,
    editableWorkTypeConfigurationState,
    locale,
    selectedDocTargetPath,
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
  const graphBulkSelectionDetailCard = renderGraphBulkSelectionDetailCard({
    bulkSelectionSummary: graphBulkSelectionSummary,
    locale: locale ?? undefined,
    widgetId,
  });
  const detailCard =
    graphBulkSelectionDetailCard ??
    renderCurrentSelectionDetailCard({
      activeTraceID,
      currentSelection,
      docHeaderAction,
      docSaveState,
      editableConfigurationState,
      editableDocConfigurationState,
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

  return (
    <CurrentSelectionLocaleProvider locale={resolvedLocale}>
      <CurrentSelectionHeaderActionProvider headerAction={headerAction ?? null}>
        {detailCard}
      </CurrentSelectionHeaderActionProvider>
      <CurrentSelectionWidgetSaveNotifications
        editableConfigurationState={editableConfigurationState}
        editableDocConfigurationState={editableDocConfigurationState}
        editableResourceConfigurationState={editableResourceConfigurationState}
        editableWorkStateConfigurationState={
          editableWorkStateConfigurationState
        }
        editableWorkerConfigurationState={editableWorkerConfigurationState}
        editableWorkTypeConfigurationState={editableWorkTypeConfigurationState}
        locale={resolvedLocale}
        docSave={docSave}
        docSaveState={docSaveState}
        resourceSave={resourceSave}
        resourceSaveState={resourceSaveState}
        selectedDocTargetPath={selectedDocTargetPath}
        selectedNode={selectedNode}
        selectedResourceName={selectedResourceName}
        selectedWorkerName={selectedWorkerName}
        selectedWorkTypeName={selectedWorkTypeName}
        selection={selection}
        workStatePlaceId={workStatePlaceId}
        workStateSave={workStateSave}
        workerSave={workerSave}
        workerSaveState={workerSaveState}
        workstationSave={workstationSave}
        workstationSaveState={workstationSaveState}
        workTypeSave={workTypeSave}
      />
      <CurrentSelectionWorkTypeSaveDialog
        locale={locale}
        workTypeSave={workTypeSave}
      />
    </CurrentSelectionLocaleProvider>
  );
}
