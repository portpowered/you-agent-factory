import { type ReactNode, useEffect } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { AgentBentoCard } from "../../bento/public";
import type { DashboardSelection } from "../../current-selection/public";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { useFactoryGraphTopologyEditorBridge } from "../state/factory-graph-topology-editor-bridge";
import {
  type CurrentActivitySelection,
  ReactFlowCurrentActivityCardView,
} from "./react-flow-current-activity-card";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";

interface WorkflowActivityBentoCardProps {
  headerAction?: ReactNode;
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
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
  onNodeRemovedFromDraft,
  selection,
  snapshot,
  widgetInstanceID,
  onSelectWorkID,
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
  const editor = useCurrentActivityGraphEditor(
    snapshot,
    locale,
    sessionID,
    onNodeRemovedFromDraft,
  );

  useEffect(() => {
    if (!editor.editorMode) {
      setTopologyEditorBridgeHandlers(null);
      return;
    }

    setTopologyEditorBridgeHandlers({
      blockedRemovalReason: editor.blockedRemovalReason,
      canInteractWithEditor: editor.canInteractWithEditor,
      editorMode: editor.editorMode,
      requestNodeRemoval: editor.handleSelectionNodeDelete,
    });

    return () => {
      setTopologyEditorBridgeHandlers(null);
    };
  }, [
    editor.blockedRemovalReason,
    editor.canInteractWithEditor,
    editor.editorMode,
    editor.handleSelectionNodeDelete,
    setTopologyEditorBridgeHandlers,
  ]);

  return (
    <AgentBentoCard
      chromeDensity="compact"
      headerAction={
        <CurrentActivityGraphHeaderActions
          key={`graph-editor-header-${editor.editorMode}-${editor.draftState.hasChanges}`}
          compact
          editorMode={editor.editorMode}
          editorUnavailableClassifierWorkstationName={
            editor.editorUnavailableClassifierWorkstationName
          }
          hasChanges={editor.draftState.hasChanges}
          headerActions={headerAction}
          isDefinitionLoading={
            editor.editableDefinitionQuery.status === "pending"
          }
          loadErrorMessage={editor.editableDefinitionQuery.error?.message}
          locale={locale}
          onToggle={editor.handleEditorModeToggle}
        />
      }
      title={messages.widgetTitle}
    >
      <section className="relative h-full min-h-0 min-w-0">
        <ReactFlowCurrentActivityCardView
          editor={editor}
          importController={importController}
          locale={locale}
          now={now}
          selection={toCurrentActivitySelection(selection)}
          showHeaderActions={false}
          snapshot={snapshot}
          widgetInstanceID={widgetInstanceID}
          onSelectWorkID={onSelectWorkID}
          onSelectResource={onSelectResource}
          onSelectStateNode={onSelectStateNode}
          onSelectWorker={onSelectWorker}
          onSelectWorkType={onSelectWorkType}
          onSelectWorkstation={onSelectWorkstation}
        />
      </section>
    </AgentBentoCard>
  );
}

function toCurrentActivitySelection(
  selection: DashboardSelection | null,
): CurrentActivitySelection | null {
  if (selection?.kind === "workstation-request") {
    return { kind: "node", nodeId: selection.nodeId };
  }

  if (
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
