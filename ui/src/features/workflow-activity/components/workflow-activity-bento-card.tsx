import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { AgentBentoCard } from "../../../components/ui";
import type { DashboardSelection } from "../../current-selection/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
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
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  widgetInstanceID?: string;
}

const GRAPH_PANEL_SHELL_CLASS = "relative h-full min-h-0";

export function WorkflowActivityBentoCard({
  headerAction,
  importController,
  locale,
  now,
  selection,
  snapshot,
  widgetInstanceID,
  onSelectWorkID,
  onSelectStateNode,
  onSelectWorkstation,
}: WorkflowActivityBentoCardProps) {
  const messages = getWorkflowActivityShellMessages(locale);
  const editor = useCurrentActivityGraphEditor(snapshot);

  return (
    <AgentBentoCard
      chromeDensity="compact"
      headerAction={
        <CurrentActivityGraphHeaderActions
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
      <section className={GRAPH_PANEL_SHELL_CLASS}>
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
          onSelectStateNode={onSelectStateNode}
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
