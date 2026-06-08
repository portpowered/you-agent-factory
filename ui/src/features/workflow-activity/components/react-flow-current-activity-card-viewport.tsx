// biome-ignore lint/nursery/noExcessiveLinesPerFile: composes React Flow canvas wiring with editor toolbar overlays.
import {
  type Connection,
  type Edge,
  type EdgeTypes,
  type FitViewOptions,
  type IsValidConnection,
  type Node,
  type NodeChange,
  type NodeTypes,
  ReactFlow,
  type ReactFlowInstance,
  type XYPosition,
} from "@xyflow/react";
import { type MutableRefObject, useCallback, useRef, type KeyboardEvent } from "react";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
} from "../../../components/dashboard/dashboard-graph";
import { cn } from "../../../lib/cn";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  type FactoryGraphEditorMenuAction,
  FactoryGraphEditorToolbar,
  FactoryGraphEditorVisibilityPanel,
  type FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  isFactoryGraphEditorRedoKeyboardEvent,
  isFactoryGraphEditorUndoKeyboardEvent,
  shouldHandleFactoryGraphEditorKeyboardShortcut,
} from "../../factory-graph-editor/lib/layout/history/factory-graph-layout-keyboard-shortcuts";
import { FactoryGraphEdgeWaypointControls } from "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-controls";
import { FactoryGraphEdgeWaypointLayer } from "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-layer";
import type { FactoryLayoutPoint } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { GraphViewportSurface } from "../../graphs/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { handleCurrentActivityReactFlowError } from "../lib/react-flow-current-activity-card-errors";
import { useCanonicalLayoutViewportSync } from "../lib/layout/use-canonical-layout-viewport-sync";
import { useMeasuredCurrentActivityGraphViewport } from "../lib/use-measured-current-activity-graph-viewport";
import type { FactoryLayoutViewport } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
  getDefaultDashboardFlowAxisLegendPhaseItems,
} from "./dashboard-flow-axis-legend";
import {
  GraphDropOverlay,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";

function CurrentActivityGraphEditorChrome(props: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canRedoLayout?: boolean;
  canSaveDraft: boolean;
  canUndoLayout?: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  editorMode: boolean;
  handleDiscardPendingChanges: () => void;
  handleEditorModeToggle: () => void;
  handleSaveDraft: () => void;
  hasPendingChanges: boolean;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen: boolean;
  isSavingDraft?: boolean;
  locale?: string;
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onClearPreferences: () => void;
  onHideShowMenuOpenChange: (open: boolean) => void;
  onRedoLayout?: () => void;
  onResetLayout?: () => void;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  onUndoLayout?: () => void;
  openAddMenu?: boolean;
  preferencesDirty: boolean;
  saveDisabledReason?: string;
}) {
  const messages = getFactoryGraphEditorMessages(props.locale);
  const editorUnavailableReason =
    props.editorUnavailableClassifierWorkstationName === undefined
      ? undefined
      : messages.modeClassifierRoutesUnavailable(
          props.editorUnavailableClassifierWorkstationName,
        );

  return (
    <FactoryGraphEditorToolbar
      activeTool={props.activeTool}
      addMenuActions={props.addMenuActions}
      canDiscard={props.hasPendingChanges}
      canInteract={props.canInteractWithEditor}
      canRedoLayout={props.canRedoLayout}
      canSave={props.canSaveDraft}
      canUndoLayout={props.canUndoLayout}
      editModeToggle={{
        disabled: !props.editorMode && editorUnavailableReason !== undefined,
        editorMode: props.editorMode,
        hasChanges: props.hasPendingChanges,
        onToggle: props.handleEditorModeToggle,
        tooltipOverride: editorUnavailableReason,
      }}
      hasPendingChanges={props.hasPendingChanges}
      hiddenNodeClasses={props.hiddenNodeClasses}
      hideShowMenuOpen={props.hideShowMenuOpen}
      hideShowVisible={true}
      isSaving={props.isSavingDraft}
      locale={props.locale}
      onAddAction={props.onAddAction}
      onAddMenuOpenChange={props.onAddMenuOpenChange}
      onClearPreferences={props.onClearPreferences}
      onDiscard={props.handleDiscardPendingChanges}
      onHideShowMenuOpenChange={props.onHideShowMenuOpenChange}
      onRedoLayout={props.onRedoLayout}
      onResetLayout={props.onResetLayout}
      onSave={props.handleSaveDraft}
      onSelectTool={props.onSelectTool}
      onToggleHiddenNodeClass={props.onToggleHiddenNodeClass}
      onUndoLayout={props.onUndoLayout}
      openAddMenu={props.openAddMenu}
      preferencesDirty={props.preferencesDirty}
      saveDisabledReason={props.saveDisabledReason}
      visible={props.editorMode}
    />
  );
}

function buildVisibilityPresetOptions(
  locale: string | undefined,
  visibilityPreset: FactoryGraphEditorVisibilityPreset,
) {
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    [
      "all",
      "workflow",
      "execution",
      "infrastructure",
    ] as const
  ).map((key) => ({
    key,
    label:
      key === "all"
        ? messages.visibilityPresetAllLabel
        : key === "workflow"
          ? messages.visibilityPresetWorkflowLabel
          : key === "execution"
            ? messages.visibilityPresetExecutionLabel
            : messages.visibilityPresetInfrastructureLabel,
    selected: visibilityPreset === key,
  }));
}

function buildCurrentActivityIsValidConnection({
  activeTool,
  editorMode,
  nodes,
}: {
  activeTool: "add" | "connect" | "delete" | null;
  editorMode: boolean;
  nodes: Node[];
}): IsValidConnection {
  return (connection) => {
    if (
      !editorMode ||
      activeTool !== "connect" ||
      !connection.source ||
      !connection.sourceHandle ||
      !connection.target ||
      !connection.targetHandle
    ) {
      return false;
    }

    const sourceNode = nodes.find((node) => node.id === connection.source);
    const targetNode = nodes.find((node) => node.id === connection.target);
    if (!sourceNode || !targetNode) {
      return false;
    }
    const sourceNodeKind = (sourceNode.data as { kind?: FactoryGraphNodeKind })
      .kind;
    const targetNodeKind = (targetNode.data as { kind?: FactoryGraphNodeKind })
      .kind;
    if (!sourceNodeKind || !targetNodeKind) {
      return false;
    }

    const sourceProgressOutcomeWorkstation = (
      sourceNode.data as {
        progressOutcomeRouteWorkstation?: WorkstationProgressOutcomeRouteContext;
      }
    ).progressOutcomeRouteWorkstation;

    return isValidFactoryGraphConnection({
      sourceAnchorId: connection.sourceHandle,
      sourceNodeKind,
      sourceWorkstation: sourceProgressOutcomeWorkstation,
      targetAnchorId: connection.targetHandle,
      targetNodeKind,
    });
  };
}

function factoryGraphNodeIdForRenderedNode(nodes: Node[], nodeId: string) {
  const node = nodes.find((entry) => entry.id === nodeId);
  return (
    (node?.data as { factoryGraphNodeId?: string } | undefined)
      ?.factoryGraphNodeId ?? nodeId
  );
}

function factoryGraphEdgeIdForRenderedEdge(nodes: Node[], edge: Edge) {
  const edgeKind = edge.id.split(":")[0];
  if (!edgeKind || !edge.source || !edge.target) {
    return edge.id;
  }

  return `${edgeKind}:${factoryGraphNodeIdForRenderedNode(
    nodes,
    edge.source,
  )}->${factoryGraphNodeIdForRenderedNode(nodes, edge.target)}`;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: composes React Flow canvas wiring with editor toolbar overlays.
export function CurrentActivityGraphViewport({
  activeTool,
  addMenuActions,
  canInteractWithEditor,
  canRedoLayout = false,
  canSaveDraft,
  canUndoLayout = false,
  editorUnavailableClassifierWorkstationName,
  handleDiscardPendingChanges,
  handleEditorModeToggle,
  handleSaveDraft,
  hiddenNodeClasses,
  hideShowMenuOpen,
  editorMode,
  onClearPreferences,
  onSelectVisibilityPreset,
  preferencesDirty,
  visibilityPreset,
  edges,
  graphKey,
  handleNodesChange,
  hasPendingChanges,
  headingID,
  imports,
  canonicalLayoutViewport,
  initialFitViewKey,
  initialFitViewOptions,
  isSavingDraft = false,
  edgeTypes,
  locale,
  nodeTypes,
  nodes,
  onAddAction,
  onAddMenuOpenChange,
  onHideShowMenuOpenChange,
  onRedoLayout,
  onResetLayout,
  onToggleHiddenNodeClass,
  onUndoLayout,
  onConnect,
  onEditorEdgeClick,
  onEditorEdgeDoubleClick,
  onEditorNodeClick,
  onMoveEdgeWaypoint,
  onRemoveEdgeWaypoint,
  onSelectTool,
  selectedEdgeWaypoints = [],
  selectedWaypointEdgeId = null,
  waypointAriaLabel,
  waypointControls = null,
  openAddMenu,
  saveDisabledReason,
  moveLayoutNode,
  moveLayoutNodesByDelta,
  updateLayoutViewport,
  setStoredNodePosition,
  flowContainerRef,
  flowInstanceRef,
}: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canRedoLayout?: boolean;
  canSaveDraft: boolean;
  canUndoLayout?: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  handleDiscardPendingChanges: () => void;
  handleEditorModeToggle: () => void;
  handleSaveDraft: () => void;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen: boolean;
  editorMode: boolean;
  onClearPreferences: () => void;
  onSelectVisibilityPreset: (preset: FactoryGraphEditorVisibilityPreset) => void;
  preferencesDirty: boolean;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
  edges: Edge[];
  graphKey: string;
  handleNodesChange: (changes: NodeChange[]) => void;
  hasPendingChanges: boolean;
  headingID: string;
  imports: CurrentActivityImportController;
  canonicalLayoutViewport?: FactoryLayoutViewport | null;
  initialFitViewKey: string;
  initialFitViewOptions: FitViewOptions;
  isSavingDraft?: boolean;
  edgeTypes?: EdgeTypes;
  locale?: string;
  nodeTypes: NodeTypes;
  nodes: Node[];
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onHideShowMenuOpenChange: (open: boolean) => void;
  onRedoLayout?: () => void;
  onResetLayout?: () => void;
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  onUndoLayout?: () => void;
  onConnect?: (connection: Connection) => void;
  onEditorEdgeClick?: (edgeId: string) => void;
  onEditorEdgeDoubleClick?: (
    edgeId: string,
    position: FactoryLayoutPoint,
  ) => void;
  onEditorNodeClick?: (nodeId: string) => void;
  onMoveEdgeWaypoint?: (
    edgeId: string,
    waypointIndex: number,
    position: FactoryLayoutPoint,
  ) => void;
  onRemoveEdgeWaypoint?: (edgeId: string, waypointIndex: number) => void;
  selectedEdgeWaypoints?: readonly FactoryLayoutPoint[];
  selectedWaypointEdgeId?: string | null;
  waypointAriaLabel?: (index: number) => string;
  waypointControls?: {
    addWaypointLabel: string;
    edgeKindLabel: string;
    edgeSourceLabel: string;
    edgeTargetLabel: string;
    fieldKindLabel: string;
    fieldSourceLabel: string;
    fieldTargetLabel: string;
    onAddWaypoint: () => void;
    onRemoveWaypoint: (waypointIndex: number) => void;
    removeWaypointLabel: (index: number) => string;
    selectedEdgeLabel: string;
    waypointCount: number;
  } | null;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  openAddMenu?: boolean;
  saveDisabledReason?: string;
  moveLayoutNode?: (nodeId: string, position: XYPosition) => void;
  moveLayoutNodesByDelta?: (
    nodeIds: readonly string[],
    delta: XYPosition,
    resolvedPositionsByNodeId: ReadonlyMap<string, XYPosition>,
  ) => void;
  updateLayoutViewport?: (viewport: { x: number; y: number; zoom: number }) => void;
  setStoredNodePosition: (
    graphKey: string,
    nodeId: string,
    position: XYPosition,
  ) => void;
  flowContainerRef?: MutableRefObject<HTMLElement | null>;
  flowInstanceRef?: MutableRefObject<ReactFlowInstance | null>;
}) {
  const editorMessages = getFactoryGraphEditorMessages(locale);
  const dragSessionRef = useRef<{
    draggedNodeId: string;
    factoryGraphNodeIds: readonly string[];
    startPositionsByNodeId: Map<string, XYPosition>;
  } | null>(null);
  const isValidConnection = buildCurrentActivityIsValidConnection({
    activeTool,
    editorMode,
    nodes,
  });
  const graphViewport =
    useMeasuredCurrentActivityGraphViewport(flowContainerRef);
  const shouldFitView = canonicalLayoutViewport == null;
  const skipNextViewportMoveEndRef = useRef(false);
  useCanonicalLayoutViewportSync({
    canonicalLayoutViewport,
    fitViewOptions: initialFitViewOptions,
    flowInstanceRef,
    skipNextViewportMoveEndRef,
    viewportResetKey: initialFitViewKey,
  });
  const handleConnect = useCallback(
    (connection: Connection) => {
      onConnect?.({
        ...connection,
        source: connection.source
          ? factoryGraphNodeIdForRenderedNode(nodes, connection.source)
          : connection.source,
        target: connection.target
          ? factoryGraphNodeIdForRenderedNode(nodes, connection.target)
          : connection.target,
      });
    },
    [nodes, onConnect],
  );
  const handleEditorCanvasKeyDown = useCallback(
    (event: KeyboardEvent<HTMLElement>) => {
      if (!editorMode || !shouldHandleFactoryGraphEditorKeyboardShortcut(event.target)) {
        return;
      }

      if (isFactoryGraphEditorUndoKeyboardEvent(event) && canUndoLayout) {
        event.preventDefault();
        onUndoLayout?.();
        return;
      }

      if (isFactoryGraphEditorRedoKeyboardEvent(event) && canRedoLayout) {
        event.preventDefault();
        onRedoLayout?.();
      }
    },
    [canRedoLayout, canUndoLayout, editorMode, onRedoLayout, onUndoLayout],
  );

  return (
    <div
      className="relative max-h-full min-h-96 flex-1 overflow-hidden"
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
    >
      <DashboardFlowAxisLegend
        className="absolute left-4 right-4 top-4 z-10"
        defaultExpanded={false}
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems(locale)}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems(locale)}
        phaseItems={getDefaultDashboardFlowAxisLegendPhaseItems(locale)}
        locale={locale}
      />
      <GraphViewportSurface
        aria-describedby={headingID}
        aria-label={editorMessages.viewportLabel}
        className={cn(
          "min-h-96",
          (imports.dropState.status === "drag-active" ||
            imports.dropState.status === "reading") &&
            "border-primary bg-primary-container",
          imports.dropState.status === "error" &&
            "border-af-danger-border bg-error-container",
          imports.dropState.status === "idle" && "border-transparent",
        )}
        data-current-activity-drop-state={graphDropStateAttribute(
          imports.dropState,
        )}
        data-current-activity-flow
        data-factory-graph-editor-canvas={editorMode ? "true" : undefined}
        onKeyDown={handleEditorCanvasKeyDown}
        ref={flowContainerRef}
        tabIndex={editorMode ? 0 : undefined}
        style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
        onDragEnter={imports.onDragEnter}
        onDragLeave={imports.onDragLeave}
        onDragOver={imports.onDragOver}
        onDrop={imports.onDrop}
      >
        {graphViewport.ready ? (
          <div
            className="absolute left-0 top-0"
            data-current-activity-flow-viewport
            style={{
              height: `${graphViewport.height}px`,
              width: `${graphViewport.width}px`,
            }}
          >
            <ReactFlow
              className="shadow-none"
              connectionLineStyle={{
                stroke: "var(--color-primary)",
                strokeWidth: 2.4,
              }}
              edges={edges}
              edgeTypes={edgeTypes}
              defaultViewport={canonicalLayoutViewport ?? undefined}
              fitView={shouldFitView}
              fitViewOptions={initialFitViewOptions}
              key={initialFitViewKey}
              isValidConnection={isValidConnection}
              maxZoom={2}
              minZoom={0.25}
              nodeTypes={nodeTypes}
              nodes={nodes}
              edgesFocusable={editorMode}
              nodesConnectable={editorMode && activeTool === "connect"}
              onConnect={handleConnect}
              onInit={(instance) => {
                if (flowInstanceRef) {
                  flowInstanceRef.current = instance;
                }
              }}
              onError={handleCurrentActivityReactFlowError}
              onEdgeClick={(_, edge) => {
                if (editorMode) {
                  onEditorEdgeClick?.(
                    factoryGraphEdgeIdForRenderedEdge(nodes, edge),
                  );
                }
              }}
              onEdgeDoubleClick={(event, edge) => {
                if (!editorMode || !onEditorEdgeDoubleClick) {
                  return;
                }

                const flowInstance = flowInstanceRef?.current;
                if (!flowInstance) {
                  return;
                }

                onEditorEdgeDoubleClick(
                  factoryGraphEdgeIdForRenderedEdge(nodes, edge),
                  flowInstance.screenToFlowPosition({
                    x: event.clientX,
                    y: event.clientY,
                  }),
                );
              }}
              nodesDraggable={true}
              onNodeClick={(_, node) => {
                if (editorMode) {
                  onEditorNodeClick?.(
                    factoryGraphNodeIdForRenderedNode(nodes, node.id),
                  );
                }
              }}
              onMoveEnd={(_, viewport) => {
                if (!editorMode || !updateLayoutViewport) {
                  return;
                }

                if (skipNextViewportMoveEndRef.current) {
                  skipNextViewportMoveEndRef.current = false;
                  return;
                }

                updateLayoutViewport(viewport);
              }}
              onNodeDragStart={(_, node) => {
                if (!editorMode) {
                  return;
                }

                const selectedNodes = nodes.filter((entry) => entry.selected);
                const draggedNodes =
                  selectedNodes.length > 1 ? selectedNodes : [node];
                const startPositionsByNodeId = new Map<string, XYPosition>();
                const factoryGraphNodeIds: string[] = [];

                for (const draggedNode of draggedNodes) {
                  const factoryGraphNodeId = factoryGraphNodeIdForRenderedNode(
                    nodes,
                    draggedNode.id,
                  );
                  factoryGraphNodeIds.push(factoryGraphNodeId);
                  startPositionsByNodeId.set(
                    factoryGraphNodeId,
                    draggedNode.position,
                  );
                }

                dragSessionRef.current = {
                  draggedNodeId: node.id,
                  factoryGraphNodeIds,
                  startPositionsByNodeId,
                };
              }}
              onNodeDragStop={(_, node) => {
                const factoryGraphNodeId = (
                  node.data as { factoryGraphNodeId?: string } | undefined
                )?.factoryGraphNodeId;
                if (editorMode && moveLayoutNode && factoryGraphNodeId) {
                  const dragSession = dragSessionRef.current;
                  dragSessionRef.current = null;
                  if (
                    dragSession &&
                    dragSession.factoryGraphNodeIds.length > 1 &&
                    moveLayoutNodesByDelta
                  ) {
                    const startPosition =
                      dragSession.startPositionsByNodeId.get(factoryGraphNodeId);
                    if (startPosition) {
                      moveLayoutNodesByDelta(
                        dragSession.factoryGraphNodeIds,
                        {
                          x: node.position.x - startPosition.x,
                          y: node.position.y - startPosition.y,
                        },
                        dragSession.startPositionsByNodeId,
                      );
                      return;
                    }
                  }

                  moveLayoutNode(factoryGraphNodeId, node.position);
                  return;
                }

                if (graphKey) {
                  setStoredNodePosition(graphKey, node.id, node.position);
                }
              }}
              onNodesChange={handleNodesChange}
              panOnDrag
              proOptions={{ hideAttribution: true }}
              zoomOnScroll
            >
              <DashboardGraphBackground />
              <DashboardGraphControls
                fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
              />
              {editorMode &&
              selectedWaypointEdgeId &&
              onMoveEdgeWaypoint &&
              waypointAriaLabel ? (
                <FactoryGraphEdgeWaypointLayer
                  ariaLabel={waypointAriaLabel}
                  edgeId={selectedWaypointEdgeId}
                  onMoveWaypoint={onMoveEdgeWaypoint}
                  onRemoveWaypoint={onRemoveEdgeWaypoint}
                  waypoints={selectedEdgeWaypoints}
                />
              ) : null}
            </ReactFlow>
            {editorMode && waypointControls ? (
              <FactoryGraphEdgeWaypointControls {...waypointControls} />
            ) : null}
          </div>
        ) : null}
        <FactoryGraphEditorVisibilityPanel
          locale={locale}
          onSelectPreset={onSelectVisibilityPreset}
          options={buildVisibilityPresetOptions(locale, visibilityPreset)}
          visible={false}
        />
        <CurrentActivityGraphEditorChrome
          activeTool={activeTool}
          addMenuActions={addMenuActions}
          canInteractWithEditor={canInteractWithEditor}
          canRedoLayout={canRedoLayout}
          canSaveDraft={canSaveDraft}
          canUndoLayout={canUndoLayout}
          editorUnavailableClassifierWorkstationName={
            editorUnavailableClassifierWorkstationName
          }
          editorMode={editorMode}
          handleDiscardPendingChanges={handleDiscardPendingChanges}
          handleEditorModeToggle={handleEditorModeToggle}
          handleSaveDraft={handleSaveDraft}
          hasPendingChanges={hasPendingChanges}
          hiddenNodeClasses={hiddenNodeClasses}
          hideShowMenuOpen={hideShowMenuOpen}
          isSavingDraft={isSavingDraft}
          locale={locale}
          onAddAction={onAddAction}
          onAddMenuOpenChange={onAddMenuOpenChange}
          onClearPreferences={onClearPreferences}
          onHideShowMenuOpenChange={onHideShowMenuOpenChange}
          onRedoLayout={onRedoLayout}
          onResetLayout={onResetLayout}
          onSelectTool={onSelectTool}
          onToggleHiddenNodeClass={onToggleHiddenNodeClass}
          onUndoLayout={onUndoLayout}
          openAddMenu={openAddMenu}
          preferencesDirty={preferencesDirty}
          saveDisabledReason={saveDisabledReason}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </GraphViewportSurface>
    </div>
  );
}
