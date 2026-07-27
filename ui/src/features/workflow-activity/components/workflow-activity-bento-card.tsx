import { type ReactNode, useEffect } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { DashboardSelection } from "../../current-selection/base/state/selection-types";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { useCurrentActivityGraphState } from "../hooks/use-current-activity-graph-state";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { useFactoryGraphTopologyEditorBridge } from "../state/factory-graph-topology-editor-bridge";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { ReactFlowCurrentActivityCardView } from "./react-flow-current-activity-card-view";
import { WorkflowActivityBentoShell } from "./bento-card/workflow-activity-bento-shell";

interface WorkflowActivityBentoCardProps {
  headerAction?: ReactNode;
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onDocAdded?: (targetPath: string) => void;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
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

export function WorkflowActivityBentoCard({
  headerAction,
  importController,
  locale,
  now,
  onDocAdded,
  onNodeRemovedFromDraft,
  selection,
  snapshot,
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
