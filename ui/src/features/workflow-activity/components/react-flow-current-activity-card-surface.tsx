import type { ReactFlowInstance } from "@xyflow/react";
import { useMemo, useRef } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { FactoryGraphEditorNotice } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { CURRENT_ACTIVITY_NODE_TYPES } from "../../flowchart/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import type { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
import {
  mergeFactoryValidationTargets,
  saveErrorNoticeMessages,
  validationMessagesForGraphSelection,
} from "../lib/react-flow-current-activity-card-validation";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import { GraphEditorPlacementRegistrar } from "./graph-editor-placement-context";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph surface keeps editor notices, validation, and viewport wiring together.
export function CurrentActivityGraphSurface({
  editor,
  graph,
  headingID,
  imports,
  locale,
  selection,
  snapshot,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  graph: ReturnType<typeof useCurrentActivityGraphViewModel>;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const flowContainerRef = useRef<HTMLElement | null>(null);
  const flowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graph.graphKey] ?? {},
  );
  const saveError = editor.saveEditableDefinition.error;
  const editorValidationProjection = useMemo(() => {
    if (!editor.editorMode) {
      return editor.structuralValidation.projection;
    }

    const saveErrorTargets =
      saveError instanceof CurrentFactoryDefinitionError
        ? (saveError.targets ?? [])
        : [];
    return mergeFactoryValidationTargets(
      editor.structuralValidation.targets,
      saveErrorTargets,
    );
  }, [
    editor.editorMode,
    editor.structuralValidation.projection,
    editor.structuralValidation.targets,
    saveError,
  ]);
  const validationSelectionMessages = editor.editorMode
    ? validationMessagesForGraphSelection({
        factoryDefinition:
          editor.draftState.pendingFactoryDefinition ??
          editor.currentFactoryDefinition ??
          (editor.editorMode ? undefined : snapshot.factory),
        projection: editorValidationProjection,
        selectionNodeId:
          selection?.kind === "node" ? selection.nodeId : undefined,
        selectionPlaceId:
          selection?.kind === "state-node" ? selection.placeId : undefined,
      })
    : [];
  const saveFailureMessages = editor.editorMode
    ? saveErrorNoticeMessages(saveError)
    : [];
  if (!snapshotHasObserverGraph(snapshot) && !editor.editorMode) {
    return <EmptyCurrentActivityState locale={locale} />;
  }

  return (
    <div className="grid min-h-0 flex-1 gap-3">
      {saveFailureMessages.length > 0 ? (
        <FactoryGraphEditorNotice
          title={messages.noticeSaveFailedTitle}
          tone="danger"
        >
          <ul className="m-0 grid list-disc gap-1 pl-5">
            {saveFailureMessages.map((message) => (
              <li key={message}>{message}</li>
            ))}
          </ul>
        </FactoryGraphEditorNotice>
      ) : null}
      {validationSelectionMessages.length > 0 ? (
        <FactoryGraphEditorNotice
          title={messages.noticeValidationFailureTitle}
          tone="danger"
        >
          <ul className="m-0 grid list-disc gap-1 pl-5">
            {validationSelectionMessages.map((message) => (
              <li key={message}>{message}</li>
            ))}
          </ul>
        </FactoryGraphEditorNotice>
      ) : null}
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
      <GraphEditorPlacementRegistrar
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        graphKey={graph.graphKey}
        nodes={graph.nodes}
        setStoredNodePosition={graph.setStoredNodePosition}
        storedNodePositions={storedNodePositions}
      />
      <CurrentActivityGraphViewport
        activeTool={editor.activeTool}
        addMenuActions={editor.addMenuActions}
        canInteractWithEditor={editor.canInteractWithEditor}
        canSaveDraft={editor.canSaveDraft}
        editorMode={editor.editorMode}
        edges={graph.edges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
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
        isSavingDraft={editor.saveEditableDefinition.isPending}
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
