import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  FactoryGraphEditorNotice,
  FactoryGraphEditorVisibilityPanel,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { CURRENT_ACTIVITY_NODE_TYPES } from "../../flowchart/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import type { useFactoryGraphEditorViewModel } from "../hooks/react-flow-current-activity-card-editor-graph";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

export function CurrentActivityGraphSurface({
  editor,
  editorGraph,
  graph,
  headingID,
  imports,
  locale,
  snapshot,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  editorGraph: ReturnType<typeof useFactoryGraphEditorViewModel>;
  graph: ReturnType<typeof useCurrentActivityGraphViewModel>;
  headingID: string;
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

  const activeGraph = editor.editorMode ? editorGraph : graph;

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
      <FactoryGraphEditorVisibilityPanel
        locale={locale}
        onSelectPreset={editorGraph.selectVisibilityPreset}
        options={editorGraph.visibilityPresetOptions}
        visible={editor.editorMode}
      />
      <CurrentActivityGraphViewport
        activeTool={editor.activeTool}
        addMenuActions={editor.addMenuActions}
        canInteractWithEditor={editor.canInteractWithEditor}
        editorMode={editor.editorMode}
        edges={activeGraph.edges}
        edgeTypes={editor.editorMode ? editorGraph.edgeTypes : undefined}
        graphKey={activeGraph.graphKey}
        handleNodesChange={activeGraph.handleNodesChange}
        hasPendingChanges={editor.draftState.hasChanges}
        headingID={headingID}
        imports={imports}
        initialFitViewKey={activeGraph.initialFitViewKey}
        initialFitViewOptions={activeGraph.initialFitViewOptions}
        locale={locale}
        nodeTypes={
          editor.editorMode
            ? editorGraph.nodeTypes
            : CURRENT_ACTIVITY_NODE_TYPES
        }
        nodes={activeGraph.nodes}
        onAddAction={editor.handleAddEntityAction}
        onAddMenuOpenChange={editor.setAddMenuOpen}
        onConnect={editor.handleEditorConnect}
        onEditorEdgeClick={editor.handleEditorEdgeDelete}
        onEditorNodeClick={editor.handleEditorNodeDelete}
        onSelectTool={editor.setActiveTool}
        openAddMenu={editor.addMenuOpen}
        setStoredNodePosition={activeGraph.setStoredNodePosition}
      />
    </div>
  );
}

function EmptyCurrentActivityState({ locale }: { locale?: string }) {
  const messages = getFactoryGraphEditorMessages(locale);
  return (
    <div className="grid min-h-60 items-start gap-1 rounded-2xl border border-dashed border-af-border-strong bg-af-surface-subtle p-5 [&_h3]:m-0">
      <h3>{messages.noticeEmptyTitle}</h3>
      <p>{messages.noticeEmptyMessage}</p>
    </div>
  );
}
