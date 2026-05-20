import { useEffect, useMemo, useState, type ReactNode } from "react";

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
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
  type LoadableProviderSessionRef,
} from "./provider-session-details";
import { requestInferenceAttempts } from "./selected-work-dispatch-history-helpers";
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

function useSelectedProviderSessionState({
  selectedNode,
  selectedNodeProviderSessions,
  selectedWorkDispatchAttempts,
  selectedWorkRequestHistory,
  selectionKind,
}: Pick<
  CurrentSelectionState,
  | "selectedNode"
  | "selectedNodeProviderSessions"
  | "selectedWorkDispatchAttempts"
  | "selectedWorkRequestHistory"
> & {
  selectionKind: CurrentSelectionState["selection"] extends { kind: infer T }
    ? T | null | undefined
    : string | null | undefined;
}) {
  const [selectedProviderSession, setSelectedProviderSession] =
    useState<LoadableProviderSessionRef | null>(null);
  const visibleProviderSessionKeys = useMemo(
    () =>
      new Set(
        (selectionKind === "work-item"
          ? [
              ...selectedWorkDispatchAttempts,
              ...selectedWorkRequestHistory.flatMap((request) =>
                requestInferenceAttempts(request),
              ),
            ]
          : selectedNode
            ? selectedNodeProviderSessions
            : []
        )
          .map((attempt) => getLoadableProviderSessionRef(attempt))
          .filter(
            (session): session is LoadableProviderSessionRef =>
              session !== null,
          )
          .map((session) => providerSessionSelectionKey(session)),
      ),
    [
      selectedNode,
      selectedNodeProviderSessions,
      selectedWorkDispatchAttempts,
      selectedWorkRequestHistory,
      selectionKind,
    ],
  );
  const selectedProviderSessionKey = selectedProviderSession
    ? providerSessionSelectionKey(selectedProviderSession)
    : null;

  useEffect(() => {
    if (!selectedProviderSession) {
      return;
    }

    if (
      !visibleProviderSessionKeys.has(
        providerSessionSelectionKey(selectedProviderSession),
      )
    ) {
      setSelectedProviderSession(null);
    }
  }, [selectedProviderSession, visibleProviderSessionKeys]);

  return {
    selectedProviderSession,
    selectedProviderSessionKey,
    setSelectedProviderSession,
  };
}

function renderCurrentSelectionDetailCard({
  activeTraceID,
  currentSelection,
  editableConfigurationState,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  now,
  onSelectTraceID,
  saveState,
  selectedProviderSession,
  selectedProviderSessionKey,
  selectedTrace,
  selectedWorkExecutionDetails,
  setSelectedProviderSession,
  widgetId,
}: {
  activeTraceID?: string | null;
  currentSelection: CurrentSelectionState;
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  headerAction: ReactNode;
  locale?: string;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  saveState: ReturnType<
    typeof useSaveEditableWorkstationConfiguration
  >["saveState"];
  selectedProviderSession: LoadableProviderSessionRef | null;
  selectedProviderSessionKey: string | null;
  selectedTrace?: DashboardTrace;
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
        selectedNode={selectedNode}
        selectedProviderSession={selectedProviderSession}
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
        selectedProviderSession={selectedProviderSession}
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
  locale,
  now,
  onSelectTraceID,
  selectedTrace,
  selectedWorkExecutionDetails,
  widgetId = "current-selection",
}: CurrentSelectionWidgetProps) {
  const {
    selectedNode,
    selectedNodeProviderSessions,
    selectedWorkDispatchAttempts,
    selectedWorkRequestHistory,
    selection,
  } = currentSelection;
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
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
  const {
    selectedProviderSession,
    selectedProviderSessionKey,
    setSelectedProviderSession,
  } = useSelectedProviderSessionState({
    selectedNode,
    selectedNodeProviderSessions,
    selectedWorkDispatchAttempts,
    selectedWorkRequestHistory,
    selectionKind: selection?.kind,
  });
  const headerAction = (
    <EditableWorkstationSaveHeaderAction
      canSave={workstationSave.canSave}
      locale={locale ?? undefined}
      onClick={workstationSave.beginSaveConfirmation}
      saveState={workstationSave.saveState}
    />
  );
  const detailCard = renderCurrentSelectionDetailCard({
    activeTraceID,
    currentSelection,
    editableConfigurationState,
    failedWorkDetailsByWorkID,
    headerAction,
    locale: locale ?? undefined,
    now,
    onSelectTraceID,
    saveState: workstationSave.saveState,
    selectedProviderSession,
    selectedProviderSessionKey,
    selectedTrace,
    selectedWorkExecutionDetails,
    setSelectedProviderSession,
    widgetId,
  });

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
