import {
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

import type {
  DashboardFailedWorkDetail,
  DashboardTrace,
} from "../../api/dashboard/types";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import { resolveEditableWorkstationValues } from "../current-factory-definition/workstation-editable-values";
import {
  NoSelectionDetailCard,
  StateNodeDetailCard,
  WorkItemDetailCard,
  WorkstationDetailCard,
  WorkstationRequestDetailCard,
} from "./current-selection-cards";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import type { WorkstationDetailCardProps } from "./detail-card-types";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import type { DashboardSelection } from "./types";
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
  const editableConfigurationState = useEditableWorkstationConfigurationState(
    selection,
    selectedNode,
  );

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
        locale={locale ?? undefined}
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
    <CurrentSelectionLocaleProvider locale={locale ?? undefined}>
      {detailCard}
    </CurrentSelectionLocaleProvider>
  );
}

function useEditableWorkstationConfigurationState(
  selection: DashboardSelection | null,
  selectedNode: CurrentSelectionState["selectedNode"],
): WorkstationDetailCardProps["editableConfigurationState"] {
  const [editableDefinitionEnabled, setEditableDefinitionEnabled] =
    useState(false);
  const previousSelectedNodeID = useRef<string | null>(null);
  const hasMounted = useRef(false);

  useEffect(() => {
    const selectedNodeID =
      selection?.kind === "node" && selectedNode ? selectedNode.node_id : null;

    if (!hasMounted.current) {
      hasMounted.current = true;
      previousSelectedNodeID.current = selectedNodeID;
      return;
    }

    if (selectedNodeID && selectedNodeID !== previousSelectedNodeID.current) {
      setEditableDefinitionEnabled(true);
    }

    if (!selectedNodeID) {
      setEditableDefinitionEnabled(false);
    }

    previousSelectedNodeID.current = selectedNodeID;
  }, [selectedNode, selection]);

  const editableDefinition = useCurrentEditableFactoryDefinition(
    editableDefinitionEnabled &&
      selection?.kind === "node" &&
      selectedNode != null,
  );

  if (selection?.kind !== "node" || !selectedNode) {
    return undefined;
  }

  if (editableDefinition.isPending) {
    return { status: "loading" };
  }

  if (editableDefinition.isError) {
    return {
      errorMessage: editableDefinition.error.message,
      status: "error",
    };
  }

  if (!editableDefinition.data) {
    return {
      message:
        "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.",
      status: "empty",
    };
  }

  const values = resolveEditableWorkstationValues(
    editableDefinition.data,
    selectedNode,
  );

  return values
    ? { status: "ready", values }
    : {
        message:
          "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.",
        status: "empty",
      };
}
