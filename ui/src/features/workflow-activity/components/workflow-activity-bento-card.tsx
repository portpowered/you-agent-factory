import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { AgentBentoCard } from "../../bento/public";
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
  onSelectWorker: (workerName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  widgetInstanceID?: string;
}

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
  onSelectWorker,
  onSelectWorkstation,
}: WorkflowActivityBentoCardProps) {
  const messages = getWorkflowActivityShellMessages(locale);
  const { sessionID } = useDashboardSession();
  const editor = useCurrentActivityGraphEditor(snapshot, locale, sessionID);

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
          onSelectStateNode={onSelectStateNode}
          onSelectWorker={onSelectWorker}
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

  if (selection?.kind === "worker") {
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
