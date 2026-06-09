import type { ReactFlowInstance } from "@xyflow/react";
import { useCallback, useMemo, useRef, useState } from "react";
import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { AlertPanel } from "../../../components/ui";
import { FactoryGraphEditorNotice } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { useFactoryGraphEdgeWaypointEditor } from "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook";
import { decorateProjectedEdgesWithWaypoints } from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-edge-waypoint-projection";
import type { FactoryGraphReactFlowEdge } from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { NODE_TYPES } from "../../flowchart/public";
import { FACTORY_GRAPH_EDGE_TYPES } from "../../graphs/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { shouldShowGraphSaveFailureNotice } from "../lib/graph-save-failure-notice-visibility";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import {
  mergeFactoryValidationTargets,
  saveErrorNoticeMessages,
  validationMessagesForGraphSelection,
} from "../lib/react-flow-current-activity-card-validation";
import { GraphEditorPlacementRegistrar } from "./graph-editor-placement-context";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

type CurrentActivityGraphSurfaceModel = CurrentActivityGraphCardViewModel;

export function CurrentActivityGraphSurface({
  discardPendingChanges,
  viewModel,
  headingID,
  imports,
  locale,
  selection,
  snapshot,
}: {
  discardPendingChanges?: () => void;
  viewModel: CurrentActivityGraphCardViewModel;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}) {
  return (
    <CurrentActivityGraphSurfaceContent
      discardPendingChanges={discardPendingChanges}
      headingID={headingID}
      imports={imports}
      locale={locale}
      model={viewModel}
      selection={selection}
      snapshot={snapshot}
    />
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph surface keeps editor notices, validation, and viewport wiring together.
function CurrentActivityGraphSurfaceContent({
  discardPendingChanges,
  headingID,
  imports,
  locale,
  model,
  selection,
  snapshot,
}: {
  discardPendingChanges?: () => void;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  model: CurrentActivityGraphSurfaceModel;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const flowContainerRef = useRef<HTMLElement | null>(null);
  const flowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const saveError = model.status.saveError;
  const editorValidationProjection = useMemo(() => {
    if (!model.editorMode) {
      return model.validationControls.projection;
    }

    const saveErrorTargets =
      saveError instanceof CurrentFactoryDefinitionError
        ? (saveError.targets ?? [])
        : [];
    return mergeFactoryValidationTargets(
      model.validationControls.targets,
      saveErrorTargets,
    );
  }, [
    model.editorMode,
    model.validationControls.projection,
    model.validationControls.targets,
    saveError,
  ]);
  const validationSelectionMessages = model.editorMode
    ? validationMessagesForGraphSelection({
        factoryDefinition:
          model.validationControls.factoryDefinition ?? undefined,
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
  const layoutControls = model.layoutControls;
  const removalControls = model.removalControls;
  const currentLayout = layoutControls.currentLayout;
  const waypointEditor = useFactoryGraphEdgeWaypointEditor({
    activeTool: model.activeTool,
    addEdgeWaypoint: layoutControls.addEdgeWaypoint,
    canInteractWithEditor: model.canInteractWithEditor,
    editorMode: model.editorMode,
    handleEditorEdgeDelete: removalControls.deleteEdge,
    layout: currentLayout,
    locale,
    moveEdgeWaypoint: layoutControls.moveEdgeWaypoint,
    removeEdgeWaypoint: layoutControls.removeEdgeWaypoint,
    nodes: model.nodes,
  });
  const viewportEdges = useMemo(
    () =>
      decorateProjectedEdgesWithWaypoints({
        edges: model.edges as FactoryGraphReactFlowEdge[],
        layout: currentLayout,
        selectedWaypointEdgeId: waypointEditor.selectedWaypointEdgeId,
      }),
    [currentLayout, model.edges, waypointEditor.selectedWaypointEdgeId],
  );
  const canPersistLayoutChanges = layoutControls.canMoveLayout;

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
      {removalControls.blockedReason ? (
        <FactoryGraphEditorNotice
          title={messages.noticeRemovalBlockedTitle}
          tone="warning"
        >
          {removalControls.blockedReason}
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
      {model.hasActiveWork && model.status.hasTopologyChanges ? (
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
        moveLayoutNode={
          canPersistLayoutChanges ? layoutControls.moveNode : undefined
        }
        nodes={model.nodes}
      />
      <CurrentActivityGraphViewport
        activeTool={model.activeTool}
        addMenuActions={model.addControls.actions}
        canInteractWithEditor={model.canInteractWithEditor}
        canRedoLayout={layoutControls.canRedo}
        canSaveDraft={model.saveControls.canSave}
        canUndoLayout={layoutControls.canUndo}
        editorUnavailableClassifierWorkstationName={
          model.editorUnavailableClassifierWorkstationName
        }
        editorMode={model.editorMode}
        edgeTypes={FACTORY_GRAPH_EDGE_TYPES}
        edges={viewportEdges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        handleDiscardPendingChanges={
          discardPendingChanges ?? model.handleDiscardPendingChanges
        }
        handleEditorModeToggle={model.handleEditorModeToggle}
        handleNodesChange={model.handleNodesChange}
        handleSaveDraft={model.saveControls.requestConfirmation}
        hasPendingChanges={model.status.hasSharedGraphChanges}
        headingID={headingID}
        imports={imports}
        canonicalLayoutViewport={model.canonicalLayoutViewport}
        initialFitViewKey={model.initialFitViewKey}
        initialFitViewOptions={model.initialFitViewOptions}
        isSavingDraft={model.status.isSaving}
        locale={locale}
        nodeTypes={NODE_TYPES}
        nodes={model.nodes}
        onAddAction={model.addControls.startAction}
        onAddMenuOpenChange={model.addControls.setMenuOpen}
        hiddenNodeClasses={model.hiddenNodeClasses}
        hideShowMenuOpen={model.hideShowMenuOpen}
        onClearPreferences={model.resetPreferences}
        onHideShowMenuOpenChange={model.setHideShowMenuOpen}
        onSelectVisibilityPreset={model.setVisibilityPreset}
        onToggleHiddenNodeClass={model.toggleHiddenNodeClass}
        preferencesDirty={model.status.preferencesDirty}
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
        onEditorNodeClick={removalControls.deleteNode}
        onSelectTool={model.setActiveTool}
        openAddMenu={model.addControls.isMenuOpen}
        saveDisabledReason={model.saveBlockedReason}
        moveLayoutNode={
          canPersistLayoutChanges ? layoutControls.moveNode : undefined
        }
        moveLayoutNodesByDelta={
          canPersistLayoutChanges ? layoutControls.moveNodesByDelta : undefined
        }
        onRedoLayout={layoutControls.redo}
        onResetLayout={layoutControls.reset}
        onUndoLayout={layoutControls.undo}
        updateLayoutViewport={
          canPersistLayoutChanges ? layoutControls.updateViewport : undefined
        }
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
