import type { ReactFlowInstance } from "@xyflow/react";
import { useCallback, useMemo, useRef, useState } from "react";
import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { AlertPanel } from "../../../components/ui";
import { FactoryGraphEditorNotice } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { NODE_TYPES } from "../../flowchart/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import type { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { shouldShowGraphSaveFailureNotice } from "../lib/graph-save-failure-notice-visibility";
import {
  mergeFactoryValidationTargets,
  saveErrorNoticeMessages,
  validationMessagesForGraphSelection,
} from "../lib/react-flow-current-activity-card-validation";
import { createDefaultFactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { FACTORY_GRAPH_EDGE_TYPES } from "../../graphs/public";
import { useFactoryGraphEdgeWaypointEditor } from "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook";
import { GraphEditorPlacementRegistrar } from "./graph-editor-placement-context";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph surface keeps editor notices, validation, and viewport wiring together.
export function CurrentActivityGraphSurface({
  discardPendingChanges,
  editor,
  graph,
  viewModel,
  headingID,
  imports,
  locale,
  selection,
  snapshot,
}: {
  discardPendingChanges?: () => void;
  editor?: ReturnType<typeof useCurrentActivityGraphEditor>;
  graph?: ReturnType<typeof useCurrentActivityGraphViewModel>;
  viewModel?: CurrentActivityGraphCardViewModel;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}) {
  const model =
    viewModel ??
    (editor && graph
      ? {
          ...editor,
          ...graph,
        }
      : null);
  if (!model) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);
  const flowContainerRef = useRef<HTMLElement | null>(null);
  const flowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const storedNodePositions = model.storedNodePositions;
  const saveError = model.saveEditableDefinition.error;
  const editorValidationProjection = useMemo(() => {
    if (!model.editorMode) {
      return model.structuralValidation.projection;
    }

    const saveErrorTargets =
      saveError instanceof CurrentFactoryDefinitionError
        ? (saveError.targets ?? [])
        : [];
    return mergeFactoryValidationTargets(
      model.structuralValidation.targets,
      saveErrorTargets,
    );
  }, [
    model.editorMode,
    model.structuralValidation.projection,
    model.structuralValidation.targets,
    saveError,
  ]);
  const validationSelectionMessages = model.editorMode
    ? validationMessagesForGraphSelection({
        factoryDefinition:
          model.viewState?.pendingFactoryDefinition ??
          model.draftState.pendingFactoryDefinition ??
          model.viewState?.currentFactoryDefinition ??
          model.currentFactoryDefinition ??
          (model.editorMode
            ? undefined
            : (model.viewState?.persistedFactoryDefinition ??
              snapshot.factory)),
        projection: editorValidationProjection,
        selectionNodeId:
          selection?.kind === "node" ? selection.nodeId : undefined,
        selectionPlaceId:
          selection?.kind === "state-node" ? selection.placeId : undefined,
      })
    : [];
  const saveFailureMessages = model.editorMode
    ? saveErrorNoticeMessages(saveError)
    : [];
  const [dismissedSaveFailureRevision, setDismissedSaveFailureRevision] =
    useState<number | null>(null);
  const dismissSaveFailureNotice = useCallback(() => {
    setDismissedSaveFailureRevision(model.saveAttemptRevision);
  }, [model.saveAttemptRevision]);
  const showSaveFailureNotice = shouldShowGraphSaveFailureNotice({
    dismissedSaveFailureRevision,
    hasFailureMessages: saveFailureMessages.length > 0,
    saveAttemptRevision: model.saveAttemptRevision,
  });
  const waypointEditor = useFactoryGraphEdgeWaypointEditor({
    activeTool: model.activeTool,
    addEdgeWaypoint: model.addEdgeWaypoint,
    canInteractWithEditor: model.canInteractWithEditor,
    editorMode: model.editorMode,
    handleEditorEdgeDelete: model.handleEditorEdgeDelete,
    layout:
      model.layoutDraftState?.layout ?? createDefaultFactoryLayout(),
    locale,
    moveEdgeWaypoint: model.moveEdgeWaypoint,
    removeEdgeWaypoint: model.removeEdgeWaypoint,
    nodes: model.nodes,
  });
  const viewportEdges = waypointEditor.decorateEditorEdges(model.edges);
  const clearStoredViewport = useCurrentActivityGraphStore(
    (state) => state.clearViewport,
  );
  const setStoredViewport = useCurrentActivityGraphStore(
    (state) => state.setViewport,
  );

  if (!snapshotHasObserverGraph(snapshot) && !model.editorMode) {
    return <EmptyCurrentActivityState locale={locale} />;
  }

  return (
    <div
      className="grid max-h-full min-h-0 flex-1 gap-3 overflow-hidden"
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
    >
      {showSaveFailureNotice ? (
        <FactoryGraphEditorNotice
          dismissLabel={messages.noticeDismissLabel}
          onDismiss={dismissSaveFailureNotice}
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
      {model.blockedRemovalReason ? (
        <FactoryGraphEditorNotice
          title={messages.noticeRemovalBlockedTitle}
          tone="warning"
        >
          {model.blockedRemovalReason}
        </FactoryGraphEditorNotice>
      ) : null}
      {model.connectionNotice ? (
        <FactoryGraphEditorNotice
          title={messages.noticeConnectionBlockedTitle}
          tone="warning"
        >
          {model.connectionNotice}
        </FactoryGraphEditorNotice>
      ) : null}
      {model.hasActiveWork && model.draftState.hasChanges ? (
        <FactoryGraphEditorNotice
          title={messages.noticeTopologyBlockedTitle}
          tone="danger"
        >
          {messages.noticeTopologyBlockedDescription}
        </FactoryGraphEditorNotice>
      ) : null}
      {model.isStaleDraft ? (
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
        graphKey={model.graphKey}
        moveLayoutNode={
          model.editorMode ? model.moveLayoutNode : undefined
        }
        nodes={model.nodes}
        setStoredNodePosition={model.setStoredNodePosition}
        storedNodePositions={storedNodePositions}
      />
      <CurrentActivityGraphViewport
        activeTool={model.activeTool}
        addMenuActions={model.addMenuActions}
        canInteractWithEditor={model.canInteractWithEditor}
        canRedoLayout={model.layoutDraftState?.canRedoLayout ?? false}
        canSaveDraft={model.canSaveDraft}
        canUndoLayout={model.layoutDraftState?.canUndoLayout ?? false}
        editorUnavailableClassifierWorkstationName={
          model.editorUnavailableClassifierWorkstationName
        }
        editorMode={model.editorMode}
        edgeTypes={FACTORY_GRAPH_EDGE_TYPES}
        edges={viewportEdges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        graphKey={model.graphKey}
        handleDiscardPendingChanges={
          discardPendingChanges ?? model.handleDiscardPendingChanges
        }
        handleEditorModeToggle={model.handleEditorModeToggle}
        handleNodesChange={model.handleNodesChange}
        handleSaveDraft={() => {
          model.setIsConfirmingSave(true);
        }}
        hasPendingChanges={
          model.draftState.hasChanges ||
          (model.layoutDraftState?.layoutDirty ?? false)
        }
        headingID={headingID}
        imports={imports}
        canonicalLayoutViewport={model.canonicalLayoutViewport}
        initialFitViewKey={model.initialFitViewKey}
        initialFitViewOptions={model.initialFitViewOptions}
        isSavingDraft={model.saveEditableDefinition.isPending}
        locale={locale}
        nodeTypes={NODE_TYPES}
        nodes={model.nodes}
        onAddAction={model.handleAddEntityAction}
        onAddMenuOpenChange={model.setAddMenuOpen}
        hiddenNodeClasses={model.hiddenNodeClasses}
        hideShowMenuOpen={model.hideShowMenuOpen}
        onClearPreferences={model.resetPreferences}
        onHideShowMenuOpenChange={model.setHideShowMenuOpen}
        onSelectVisibilityPreset={model.setVisibilityPreset}
        onToggleHiddenNodeClass={model.toggleHiddenNodeClass}
        preferencesDirty={model.dirtyStateSummary.preferencesDirty}
        visibilityPreset={model.visibilityPreset}
        onConnect={model.handleEditorConnect}
        onEditorEdgeClick={waypointEditor.handleEditorEdgeClick}
        onEditorEdgeDoubleClick={waypointEditor.handleEditorEdgeDoubleClick}
        onMoveEdgeWaypoint={waypointEditor.handleMoveSelectedEdgeWaypoint}
        onRemoveEdgeWaypoint={waypointEditor.handleRemoveSelectedEdgeWaypoint}
        selectedEdgeWaypoints={waypointEditor.selectedEdgeWaypoints}
        selectedWaypointEdgeId={waypointEditor.selectedWaypointEdgeId}
        waypointAriaLabel={waypointEditor.waypointAriaLabel}
        waypointControls={waypointEditor.waypointControls}
        onEditorNodeClick={model.handleEditorNodeDelete}
        onSelectTool={model.setActiveTool}
        openAddMenu={model.addMenuOpen}
        saveDisabledReason={model.saveBlockedReason}
        moveLayoutNode={model.moveLayoutNode}
        moveLayoutNodesByDelta={model.moveLayoutNodesByDelta}
        onRedoLayout={model.redoLayout}
        onResetLayout={() => {
          clearStoredViewport(model.graphKey);
          model.resetLayout();
        }}
        onUndoLayout={model.undoLayout}
        updateLayoutViewport={model.updateLayoutViewport}
        setStoredNodePosition={model.setStoredNodePosition}
        setStoredViewport={setStoredViewport}
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
    <AlertPanel tone="neutral" variant="empty">
      <h3>{messages.noticeEmptyTitle}</h3>
      <p>{messages.noticeEmptyMessage}</p>
    </AlertPanel>
  );
}
