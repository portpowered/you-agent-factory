import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FactoryGraphEditorNotice } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { CURRENT_ACTIVITY_NODE_TYPES } from "../../flowchart/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import type { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

export function CurrentActivityGraphSurface({
  editor,
  graph,
  headingID,
  imports,
  locale,
  snapshot,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  graph: ReturnType<typeof useCurrentActivityGraphViewModel>;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  snapshot: DashboardSnapshot;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  if (!snapshotHasObserverGraph(snapshot) && !editor.editorMode) {
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
        canSaveDraft={editor.canSaveDraft}
        editorMode={editor.editorMode}
        edges={graph.edges}
        graphKey={graph.graphKey}
        handleDiscardPendingChanges={editor.handleDiscardPendingChanges}
        handleNodesChange={graph.handleNodesChange}
        handleSaveDraft={() => {
          editor.setIsConfirmingSave(true);
        }}
        hasPendingChanges={editor.draftState.hasChanges}
        headingID={headingID}
        imports={imports}
        initialFitViewKey={graph.initialFitViewKey}
        initialFitViewOptions={graph.initialFitViewOptions}
        isSavingDraft={editor.saveEditableDefinition.status === "pending"}
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
        saveDisabledReason={editor.saveBlockedReason}
        setStoredNodePosition={graph.setStoredNodePosition}
      />
    </div>
  );
}

function snapshotHasObserverGraph(snapshot: DashboardSnapshot): boolean {
  return (
    snapshot.topology.workstation_node_ids.length > 0 ||
    (snapshot.factory?.workstations?.length ?? 0) > 0
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
