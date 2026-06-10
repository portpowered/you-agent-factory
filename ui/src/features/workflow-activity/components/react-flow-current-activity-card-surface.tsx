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
  const editorControls = model.editorControls;
  const editorValidationProjection = useMemo(() => {
    if (!editorControls.isEditing) {
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
    editorControls.isEditing,
    model.validationControls.projection,
    model.validationControls.targets,
    saveError,
  ]);
  const validationSelectionMessages = editorControls.isEditing
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
  const saveFailureMessages = editorControls.isEditing
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
    activeTool: editorControls.activeTool,
    addEdgeWaypoint: layoutControls.addEdgeWaypoint,
    canInteractWithEditor: editorControls.canInteract,
    editorMode: editorControls.isEditing,
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

  if (!snapshotHasObserverGraph(snapshot) && !editorControls.isEditing) {
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
      {editorControls.connectionNotice ? (
        <FactoryGraphEditorNotice
          title={messages.noticeConnectionBlockedTitle}
          tone="warning"
        >
          {editorControls.connectionNotice}
        </FactoryGraphEditorNotice>
      ) : null}
      {model.status.hasActiveWork && model.status.hasTopologyChanges ? (
        <FactoryGraphEditorNotice
          title={messages.noticeTopologyBlockedTitle}
          tone="danger"
        >
          {messages.noticeTopologyBlockedDescription}
        </FactoryGraphEditorNotice>
      ) : null}
      {model.status.isStaleDraft ? (
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
          layoutControls.canMoveLayout ? layoutControls.moveNode : undefined
        }
        nodes={model.nodes}
      />
      <CurrentActivityGraphViewport
        addControls={model.addControls}
        editorControls={{
          ...editorControls,
          discardPendingChanges:
            discardPendingChanges ?? editorControls.discardPendingChanges,
        }}
        edgeTypes={FACTORY_GRAPH_EDGE_TYPES}
        edges={viewportEdges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        handleNodesChange={model.handleNodesChange}
        hasPendingChanges={model.status.hasSharedGraphChanges}
        headingID={headingID}
        imports={imports}
        canonicalLayoutViewport={model.canonicalLayoutViewport}
        initialFitViewKey={model.initialFitViewKey}
        initialFitViewOptions={model.initialFitViewOptions}
        isSavingDraft={model.status.isSaving}
        layoutControls={layoutControls}
        locale={locale}
        nodeTypes={NODE_TYPES}
        nodes={model.nodes}
        visibilityControls={model.visibilityControls}
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
        saveControls={model.saveControls}
        saveDisabledReason={model.status.saveBlockedReason}
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
