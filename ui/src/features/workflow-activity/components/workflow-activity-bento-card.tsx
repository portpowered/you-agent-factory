import { type ReactNode, useEffect, useRef } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { DashboardSelection } from "../../current-selection/base/state/selection-types";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { useCurrentActivityGraphState } from "../hooks/use-current-activity-graph-state";
import type { WorkflowActivityBentoCardState } from "../hooks/workflow-activity-card-state";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { useFactoryGraphTopologyEditorBridge } from "../state/factory-graph-topology-editor-bridge";
import { WorkflowActivityBentoShell } from "./bento-card/workflow-activity-bento-shell";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { ReactFlowCurrentActivityCardView } from "./react-flow-current-activity-card-view";

interface WorkflowActivityBentoCardProps {
  headerAction?: ReactNode;
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onCardStateChange?: (state: WorkflowActivityBentoCardState) => void;
  onDocAdded?: (targetPath: string) => void;
  onDirtyStateChange?: (isDirty: boolean) => void;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  restoredCardState?: WorkflowActivityBentoCardState;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectDoc: (targetPath: string) => void;
  onSelectResource: (resourceName: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  widgetInstanceID?: string;
}

function useWorkflowActivityCardStateReporting({
  cardStateSnapshot,
  hasSharedGraphChanges,
  onCardStateChange,
  onDirtyStateChange,
  preferencesDirty,
}: {
  cardStateSnapshot: WorkflowActivityBentoCardState;
  hasSharedGraphChanges: boolean;
  onCardStateChange?: (state: WorkflowActivityBentoCardState) => void;
  onDirtyStateChange?: (isDirty: boolean) => void;
  preferencesDirty: boolean;
}): void {
  const dirtyStateChangeRef = useRef(onDirtyStateChange);
  const cardStateChangeRef = useRef(onCardStateChange);

  useEffect(() => {
    dirtyStateChangeRef.current = onDirtyStateChange;
    cardStateChangeRef.current = onCardStateChange;
  }, [onCardStateChange, onDirtyStateChange]);

  useEffect(() => {
    dirtyStateChangeRef.current?.(hasSharedGraphChanges || preferencesDirty);

    return () => {
      dirtyStateChangeRef.current?.(false);
    };
  }, [hasSharedGraphChanges, preferencesDirty]);

  useEffect(() => {
    cardStateChangeRef.current?.(cardStateSnapshot);
  }, [cardStateSnapshot]);
}

export function WorkflowActivityBentoCard({
  headerAction,
  importController,
  locale,
  now,
  onCardStateChange,
  onDocAdded,
  onDirtyStateChange,
  onNodeRemovedFromDraft,
  selection,
  snapshot,
  restoredCardState,
  widgetInstanceID,
  onSelectWorkID,
  onSelectDoc,
  onSelectResource,
  onSelectStateNode,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
}: WorkflowActivityBentoCardProps) {
  const messages = getWorkflowActivityShellMessages(locale);
  const { sessionID } = useDashboardSession();
  const setTopologyEditorBridgeHandlers = useFactoryGraphTopologyEditorBridge(
    (state) => state.setHandlers,
  );
  const activityGraphState = useCurrentActivityGraphState(
    snapshot,
    locale,
    sessionID,
    onDocAdded,
    onNodeRemovedFromDraft,
    restoredCardState,
  );
  const currentActivitySelection = toCurrentActivitySelection(selection);
  const viewModel = useCurrentActivityGraphCardViewModel({
    graphController: activityGraphState,
    locale,
    now,
    onSelectDoc,
    onSelectResource,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkType,
    onSelectWorkstation,
    selection: currentActivitySelection,
    snapshot,
  });
  const editorControls = viewModel.editorControls;
  useWorkflowActivityCardStateReporting({
    cardStateSnapshot: activityGraphState.cardStateSnapshot,
    hasSharedGraphChanges: viewModel.status.hasSharedGraphChanges,
    onCardStateChange,
    onDirtyStateChange,
    preferencesDirty: viewModel.status.preferencesDirty,
  });

  useEffect(() => {
    if (!editorControls.isEditing) {
      setTopologyEditorBridgeHandlers(null);
      return;
    }

    setTopologyEditorBridgeHandlers({
      blockedRemovalReason: viewModel.removalControls.blockedReason,
      canInteractWithEditor: editorControls.canInteract,
      editorMode: editorControls.isEditing,
      requestNodeRemoval: viewModel.removalControls.requestSelectionNodeRemoval,
    });

    return () => {
      setTopologyEditorBridgeHandlers(null);
    };
  }, [
    viewModel.removalControls.blockedReason,
    viewModel.removalControls.requestSelectionNodeRemoval,
    editorControls.canInteract,
    editorControls.isEditing,
    setTopologyEditorBridgeHandlers,
  ]);

  return (
    <WorkflowActivityBentoShell
      headerAction={
        <CurrentActivityGraphHeaderActions
          key={`graph-editor-header-${editorControls.isEditing}-${viewModel.status.hasSharedGraphChanges}-${viewModel.status.preferencesDirty}`}
          compact
          dirtySummary={viewModel.status.dirtySummary}
          editorMode={editorControls.isEditing}
          editorUnavailableClassifierWorkstationName={
            editorControls.unavailableClassifierWorkstationName
          }
          hasChanges={viewModel.status.hasSharedGraphChanges}
          headerActions={headerAction}
          isDefinitionLoading={viewModel.status.isDefinitionLoading}
          loadErrorMessage={viewModel.status.loadErrorMessage}
          locale={locale}
          onToggle={editorControls.toggleMode}
          showModeToggle={false}
        />
      }
      title={messages.widgetTitle}
    >
      <section
        className="relative h-full max-h-full min-h-0 min-w-0 overflow-hidden"
        style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
      >
        <ReactFlowCurrentActivityCardView
          chromeless
          importController={importController}
          locale={locale}
          now={now}
          selection={currentActivitySelection}
          showHeaderActions={false}
          snapshot={snapshot}
          viewModel={viewModel}
          widgetInstanceID={widgetInstanceID}
          onSelectWorkID={onSelectWorkID}
          onSelectDoc={onSelectDoc}
          onSelectResource={onSelectResource}
          onSelectStateNode={onSelectStateNode}
          onSelectWorker={onSelectWorker}
          onSelectWorkType={onSelectWorkType}
          onSelectWorkstation={onSelectWorkstation}
        />
      </section>
    </WorkflowActivityBentoShell>
  );
}

function toCurrentActivitySelection(
  selection: DashboardSelection | null,
): CurrentActivitySelection | null {
  if (selection?.kind === "workstation-request") {
    return { kind: "node", nodeId: selection.nodeId };
  }

  if (
    selection?.kind === "doc" ||
    selection?.kind === "worker" ||
    selection?.kind === "resource" ||
    selection?.kind === "work-type"
  ) {
    return selection;
  }

  if (selection?.kind !== "work-item") {
    return selection;
  }

  return {
    kind: "work-item",
    dispatchId: selection.dispatchId ?? "",
    nodeId: selection.nodeId,
    workID: selection.workItem.work_id,
  };
}
