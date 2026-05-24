import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  FactoryGraphEditorNotice,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { CURRENT_ACTIVITY_NODE_TYPES } from "../../flowchart/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

export function CurrentActivityGraphSurface({
  editor,
  graph,
  imports,
  locale,
  snapshot,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  graph: ReturnType<typeof useCurrentActivityGraphViewModel>;
  imports: CurrentActivityImportController;
  locale?: string;
  snapshot: DashboardSnapshot;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  if (
    snapshot.topology.workstation_node_ids.length === 0 &&
    !editor.editorMode
  ) {
    return <EmptyCurrentActivityState locale={locale} />;
  }

  return (
    <div className="grid min-h-0 flex-1 gap-3">
      {editor.blockedRemovalReason ? (
        <FactoryGraphEditorNotice
          title={messages.noticeRemovalBlockedTitle}
          tone="warning"
        >
          {editor.blockedRemovalReason}
        </FactoryGraphEditorNotice>
      ) : null}
      {editor.connectionNotice ? (
        <FactoryGraphEditorNotice
          title={messages.noticeConnectionBlockedTitle}
          tone="warning"
        >
          {editor.connectionNotice}
        </FactoryGraphEditorNotice>
      ) : null}
      {editor.hasActiveWork && editor.draftState.hasChanges ? (
        <FactoryGraphEditorNotice
          title={messages.noticeTopologyBlockedTitle}
          tone="danger"
        >
          {messages.noticeTopologyBlockedDescription}
        </FactoryGraphEditorNotice>
      ) : null}
      {editor.isStaleDraft ? (
        <FactoryGraphEditorNotice
          title={messages.noticeStaleTitle}
          tone="warning"
        >
          {messages.noticeStaleDescription}
        </FactoryGraphEditorNotice>
      ) : null}
      {editor.saveEditableDefinition.error ? (
        <FactoryGraphEditorNotice
          title={messages.noticeSaveFailedTitle}
          tone="danger"
        >
          {editor.saveEditableDefinition.error.message}
        </FactoryGraphEditorNotice>
      ) : null}
      {editor.saveEditableDefinition.status === "success" &&
      !editor.draftState.hasChanges ? (
        <FactoryGraphEditorNotice
          title={messages.noticeSaveSuccessTitle}
          tone="neutral"
        >
          {messages.noticeSaveSuccessDescription}
        </FactoryGraphEditorNotice>
      ) : null}
      <CurrentActivityGraphViewport
        activeTool={editor.activeTool}
        addMenuActions={editor.addMenuActions}
        canInteractWithEditor={editor.canInteractWithEditor}
        editorMode={editor.editorMode}
        edges={graph.edges}
        graphKey={graph.graphKey}
        handleNodesChange={graph.handleNodesChange}
        hasPendingChanges={editor.draftState.hasChanges}
        imports={imports}
        initialFitViewKey={graph.initialFitViewKey}
        initialFitViewOptions={graph.initialFitViewOptions}
        locale={locale}
        nodeTypes={CURRENT_ACTIVITY_NODE_TYPES}
        nodes={graph.nodes}
        onAddAction={editor.handleAddEntityAction}
        onAddMenuOpenChange={editor.setAddMenuOpen}
        onConnect={editor.handleEditorConnect}
        onEditorEdgeClick={editor.handleEditorEdgeDelete}
        onEditorNodeClick={editor.handleEditorNodeDelete}
        onSelectTool={editor.setActiveTool}
        openAddMenu={editor.addMenuOpen}
        setStoredNodePosition={graph.setStoredNodePosition}
      />
    </div>
  );
}

function EmptyCurrentActivityState({ locale }: { locale?: string }) {
  const messages = getFactoryGraphEditorMessages(locale);
  return (
    <div className="grid min-h-60 items-start gap-1 rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0">
      <h3>{messages.noticeEmptyTitle}</h3>
      <p>{messages.noticeEmptyMessage}</p>
    </div>
  );
}
