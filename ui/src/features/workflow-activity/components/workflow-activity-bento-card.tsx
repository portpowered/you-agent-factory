import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { AgentBentoCard } from "../../../components/ui";
import type { DashboardSelection } from "../../current-selection";
import type { CurrentActivityImportController } from "../current-activity-import-controller";
import {
  type CurrentActivitySelection,
  ReactFlowCurrentActivityCardView,
} from "./react-flow-current-activity-card";
import { useCurrentActivityGraphEditor } from "../react-flow-current-activity-card-editor";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";

interface WorkflowActivityBentoCardProps {
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
}

const GRAPH_PANEL_SHELL_CLASS = "relative h-full min-h-0";

export function WorkflowActivityBentoCard({
  importController,
  locale,
  now,
  selection,
  snapshot,
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
