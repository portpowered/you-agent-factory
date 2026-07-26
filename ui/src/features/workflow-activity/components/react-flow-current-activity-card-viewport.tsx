// biome-ignore lint/style/noExcessiveLinesPerFile: composes React Flow canvas wiring with editor toolbar overlays.
import {
  type Connection,
  type Edge,
  type EdgeChange,
  type EdgeTypes,
  type FitViewOptions,
  type IsValidConnection,
  type Node,
  type NodeChange,
  type NodeTypes,
  type OnSelectionChangeFunc,
  ReactFlow,
  type ReactFlowInstance,
  type XYPosition,
} from "@xyflow/react";
import {
  type ComponentProps,
  type KeyboardEvent,
  type MutableRefObject,
  useCallback,
  useEffect,
  useRef,
} from "react";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
} from "../../../components/dashboard/dashboard-graph";
import { cn } from "../../../lib/cn";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  type FactoryGraphEditorMenuAction,
  FactoryGraphEditorToolbar,
  type FactoryGraphEditorToolbarSelectionState,
  FactoryGraphEditorVisibilityPanel,
  type FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { FactoryGraphEdgeWaypointControls } from "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-controls";
import { FactoryGraphEdgeWaypointLayer } from "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-layer";
import { FactoryGraphVisualGroupControls } from "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-controls";
import { FactoryGraphVisualGroupLayer } from "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-layer";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import type {
  FactoryLayoutPoint,
  FactoryLayoutViewport,
} from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import {
  isFactoryGraphEditorDeleteSelectionKeyboardEvent,
  isFactoryGraphEditorRedoKeyboardEvent,
  isFactoryGraphEditorUndoKeyboardEvent,
  shouldHandleFactoryGraphEditorKeyboardShortcut,
} from "../../factory-graph-editor/lib/layout/history/factory-graph-layout-keyboard-shortcuts";
import type { FactoryLayoutGroup } from "../../factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups";
import { FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS } from "../../factory-graph-editor/lib/selection/factory-graph-editor-react-flow-interaction";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { useFactoryGraphTouchPanePan } from "../../factory-graph-editor/hooks/selection/use-factory-graph-touch-pane-pan";
import { GraphViewportSurface } from "../../graphs/components/dashboard-graph-viewport-surface";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCanonicalLayoutViewportSync } from "../lib/layout/use-canonical-layout-viewport-sync";
import { handleCurrentActivityReactFlowError } from "../lib/react-flow-current-activity-card-errors";
import { useMeasuredCurrentActivityGraphViewport } from "../lib/use-measured-current-activity-graph-viewport";
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
  addControls: CurrentActivityGraphViewportAddControls;
  canDeleteGraphSelection?: boolean;
  deleteGraphSelection?: () => void;
  editorControls: CurrentActivityGraphViewportEditorControls;
  graphSelectionToolbarState?: FactoryGraphEditorToolbarSelectionState;
  hasPendingChanges: boolean;
  isSavingDraft?: boolean;
  layoutControls: CurrentActivityGraphViewportLayoutControls;
  locale?: string;
  onCreateVisualGroup?: () => void;
  saveControls: CurrentActivityGraphViewportSaveControls;
  saveDisabledReason?: string;
  visibilityControls: CurrentActivityGraphViewportVisibilityControls;
}) {
  const messages = getFactoryGraphEditorMessages(props.locale);
  const editorUnavailableReason =
    props.editorControls.unavailableClassifierWorkstationName === undefined
      ? undefined
      : messages.modeClassifierRoutesUnavailable(
          props.editorControls.unavailableClassifierWorkstationName,
        );

  return (
    <FactoryGraphEditorToolbar
      activeTool={props.editorControls.activeTool}
      addMenuActions={props.addControls.actions}
      canDeleteSelection={props.canDeleteGraphSelection}
      canDiscard={props.hasPendingChanges}
      canInteract={props.editorControls.canInteract}
      canRedoLayout={props.layoutControls.canRedo}
      canSave={props.saveControls.canSave}
      canUndoLayout={props.layoutControls.canUndo}
      editModeToggle={{
        disabled:
          !props.editorControls.isEditing &&
          editorUnavailableReason !== undefined,
        editorMode: props.editorControls.isEditing,
        hasChanges: props.hasPendingChanges,
        onToggle: props.editorControls.toggleMode,
        tooltipOverride: editorUnavailableReason,
      }}
      graphSelectionToolbarState={props.graphSelectionToolbarState}
      hiddenNodeClasses={props.visibilityControls.hiddenNodeClasses}
      hideShowMenuOpen={props.visibilityControls.isMenuOpen}
      hideShowVisible={true}
      isSaving={props.isSavingDraft}
      locale={props.locale}
      onAddAction={props.addControls.startAction}
      onAddMenuOpenChange={props.addControls.setMenuOpen}
      onClearPreferences={props.visibilityControls.resetPreferences}
      onCreateVisualGroup={props.onCreateVisualGroup}
      onDiscard={props.editorControls.discardPendingChanges}
      onDeleteSelection={props.deleteGraphSelection}
      onHideShowMenuOpenChange={props.visibilityControls.setMenuOpen}
      onRedoLayout={props.layoutControls.redo}
      onResetLayout={props.layoutControls.reset}
      onSave={props.saveControls.requestConfirmation}
      onSelectTool={props.editorControls.selectTool}
      onToggleHiddenNodeClass={props.visibilityControls.toggleHiddenNodeClass}
      onUndoLayout={props.layoutControls.undo}
      openAddMenu={props.addControls.isMenuOpen}
      preferencesDirty={props.visibilityControls.isDirty}
      saveDisabledReason={props.saveDisabledReason}
      visible={props.editorControls.isEditing}
    />
  );
}

function buildVisibilityPresetOptions(
  locale: string | undefined,
  visibilityPreset: FactoryGraphEditorVisibilityPreset,
) {
  const messages = getFactoryGraphEditorMessages(locale);

  return (["all", "workflow", "execution", "infrastructure"] as const).map(
    (key) => ({
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
    }),
  );
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
      activeTool === "delete" ||
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
  const factoryGraphEdgeId = (
    edge.data as { factoryGraphEdgeId?: string } | undefined
  )?.factoryGraphEdgeId;
  if (factoryGraphEdgeId) {
    return factoryGraphEdgeId;
  }

  const edgeKind = edge.id.split(":")[0];
  if (!edgeKind || !edge.source || !edge.target) {
    return edge.id;
  }

  return `${edgeKind}:${factoryGraphNodeIdForRenderedNode(
    nodes,
    edge.source,
  )}->${factoryGraphNodeIdForRenderedNode(nodes, edge.target)}`;
}

type CurrentActivityGraphViewportAddControls = {
  actions?: FactoryGraphEditorMenuAction[];
  isMenuOpen?: boolean;
  setMenuOpen?: (open: boolean) => void;
  startAction?: (actionID: string) => void;
  updatePlacementViewport?: (viewport: {
    height: number;
    viewport: { x: number; y: number; zoom: number };
    width: number;
  }) => void;
};

type CurrentActivityGraphViewportSaveControls = {
  canSave: boolean;
  requestConfirmation: () => void;
};

type CurrentActivityGraphViewportEditorControls = {
  activeTool: "add" | "connect" | "delete" | null;
  canInteract: boolean;
  discardPendingChanges: () => void;
  isEditing: boolean;
  selectTool: (tool: "add" | "connect" | "delete" | null) => void;
  toggleMode: () => void;
  unavailableClassifierWorkstationName?: string;
};

type CurrentActivityGraphViewportLayoutControls = {
  canMoveLayout: boolean;
  canRedo?: boolean;
  canUndo?: boolean;
  canonicalViewport?: FactoryLayoutViewport | null;
  initialFitViewKey: string;
  initialFitViewOptions: FitViewOptions;
  moveNode: (nodeId: string, position: XYPosition) => void;
  moveNodesByDelta: (
    nodeIds: readonly string[],
    delta: XYPosition,
    resolvedPositionsByNodeId: ReadonlyMap<string, XYPosition>,
  ) => void;
  redo?: () => void;
  reset?: () => void;
  undo?: () => void;
  updateViewport: (viewport: { x: number; y: number; zoom: number }) => void;
};

type CurrentActivityGraphViewportVisibilityControls = {
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  isDirty: boolean;
  isMenuOpen: boolean;
  preset: FactoryGraphEditorVisibilityPreset;
  resetPreferences: () => void;
  setMenuOpen: (open: boolean) => void;
  setPreset: (preset: FactoryGraphEditorVisibilityPreset) => void;
  toggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: composes React Flow canvas wiring with editor toolbar overlays.
export function CurrentActivityGraphViewport({
  addControls,
  canDeleteGraphSelection,
  clearGraphSelection,
  deleteGraphSelection,
  editorControls,
  graphSelectionToolbarState,
  edges,
  handleEdgesChange,
  handleGraphSelectionChange,
  handleGraphSelectionStart,
  handleNodesChange,
  hasPendingChanges,
  headingID,
  imports,
  isSavingDraft = false,
  layoutControls,
  edgeTypes,
  locale,
  nodeTypes,
  nodes,
  onConnect,
  onEditorEdgeClick,
  onEditorEdgeDoubleClick,
  onCreateVisualGroup,
  onEditorNodeClick,
  onMoveEdgeWaypoint,
  onMoveVisualGroup,
  onRemoveEdgeWaypoint,
  onResizeVisualGroup,
  onSelectVisualGroup,
  selectedEdgeWaypoints = [],
  selectedVisualGroupId = null,
  selectedWaypointEdgeId = null,
  visualGroupAriaLabel,
  visualGroupCanEdit = false,
  visualGroupControls = null,
  visualGroupResizeHandleAriaLabel,
  visualGroups = [],
  waypointAriaLabel,
  waypointControls = null,
  saveControls,
  saveDisabledReason,
  visibilityControls,
  flowContainerRef,
  flowInstanceRef,
}: {
  addControls: CurrentActivityGraphViewportAddControls;
  canDeleteGraphSelection?: boolean;
  clearGraphSelection?: () => void;
  deleteGraphSelection?: () => void;
  editorControls: CurrentActivityGraphViewportEditorControls;
  graphSelectionToolbarState?: FactoryGraphEditorToolbarSelectionState;
  edges: Edge[];
  handleEdgesChange?: (changes: EdgeChange[]) => void;
  handleGraphSelectionChange?: OnSelectionChangeFunc;
  handleGraphSelectionStart?: (event: { shiftKey: boolean }) => void;
  handleNodesChange: (changes: NodeChange[]) => void;
  hasPendingChanges: boolean;
  headingID: string;
  imports: CurrentActivityImportController;
  isSavingDraft?: boolean;
  layoutControls: CurrentActivityGraphViewportLayoutControls;
  edgeTypes?: EdgeTypes;
  locale?: string;
  nodeTypes: NodeTypes;
  nodes: Node[];
  onConnect?: (connection: Connection) => void;
  onEditorEdgeClick?: (edgeId: string) => void;
  onEditorEdgeDoubleClick?: (
    edgeId: string,
    position: FactoryLayoutPoint,
  ) => void;
  onCreateVisualGroup?: () => void;
  onEditorNodeClick?: (nodeId: string) => void;
  onMoveVisualGroup?: (
    groupId: string,
    delta: FactoryLayoutPoint,
    startMemberPositions: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  onResizeVisualGroup?: (
    groupId: string,
    bounds: FactoryLayoutGroup["bounds"],
  ) => void;
  onSelectVisualGroup?: (groupId: string) => void;
  onMoveEdgeWaypoint?: (
    edgeId: string,
    waypointIndex: number,
    position: FactoryLayoutPoint,
  ) => void;
  onRemoveEdgeWaypoint?: (edgeId: string, waypointIndex: number) => void;
  selectedEdgeWaypoints?: readonly FactoryLayoutPoint[];
  selectedVisualGroupId?: string | null;
  selectedWaypointEdgeId?: string | null;
  visualGroupAriaLabel?: (group: FactoryLayoutGroup) => string;
  visualGroupCanEdit?: boolean;
  visualGroupResizeHandleAriaLabel?: (
    corner: "ne" | "nw" | "se" | "sw",
  ) => string;
  visualGroupControls?: ComponentProps<
    typeof FactoryGraphVisualGroupControls
  > | null;
  visualGroups?: readonly FactoryLayoutGroup[];
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
  saveControls: CurrentActivityGraphViewportSaveControls;
  saveDisabledReason?: string;
  visibilityControls: CurrentActivityGraphViewportVisibilityControls;
  flowContainerRef?: MutableRefObject<HTMLElement | null>;
  flowInstanceRef?: MutableRefObject<ReactFlowInstance | null>;
}) {
  const editorMessages = getFactoryGraphEditorMessages(locale);
  const dragSessionRef = useRef<{
    draggedNodeId: string;
    factoryGraphNodeIds: readonly string[];
    startPositionsByNodeId: Map<string, XYPosition>;
  } | null>(null);
  const internalFlowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const activeFlowInstanceRef = flowInstanceRef ?? internalFlowInstanceRef;
  const touchPanePanProps = useFactoryGraphTouchPanePan(activeFlowInstanceRef);
  const isValidConnection = buildCurrentActivityIsValidConnection({
    activeTool: editorControls.activeTool,
    editorMode: editorControls.isEditing,
    nodes,
  });
  const graphViewport =
    useMeasuredCurrentActivityGraphViewport(flowContainerRef);
  const canonicalLayoutViewport = layoutControls.canonicalViewport ?? null;
  const shouldFitView = canonicalLayoutViewport == null;
  const skipNextViewportMoveEndRef = useRef(false);
  const canPersistLayoutChanges = layoutControls.canMoveLayout;
  const moveLayoutNode = canPersistLayoutChanges
    ? layoutControls.moveNode
    : undefined;
  const moveLayoutNodesByDelta = canPersistLayoutChanges
    ? layoutControls.moveNodesByDelta
    : undefined;
  const updateLayoutViewport = canPersistLayoutChanges
    ? layoutControls.updateViewport
    : undefined;
  const updatePlacementViewport = addControls.updatePlacementViewport;
  const reportPlacementViewport = useCallback(
    (viewport: { x: number; y: number; zoom: number }) => {
      if (!graphViewport.ready) {
        return;
      }

      updatePlacementViewport?.({
        height: graphViewport.height,
        viewport,
        width: graphViewport.width,
      });
    },
    [
      graphViewport.height,
      graphViewport.ready,
      graphViewport.width,
      updatePlacementViewport,
    ],
  );
  useCanonicalLayoutViewportSync({
    canonicalLayoutViewport,
    fitViewOptions: layoutControls.initialFitViewOptions,
    flowInstanceRef: activeFlowInstanceRef,
    skipNextViewportMoveEndRef,
    viewportResetKey: layoutControls.initialFitViewKey,
  });
  useEffect(() => {
    const flowInstance = activeFlowInstanceRef.current;
    if (!flowInstance) {
      if (canonicalLayoutViewport) {
        reportPlacementViewport(canonicalLayoutViewport);
      }
      return;
    }

    reportPlacementViewport(flowInstance.getViewport());
  }, [activeFlowInstanceRef, canonicalLayoutViewport, reportPlacementViewport]);
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
      if (event.key === "Escape") {
        clearGraphSelection?.();
        return;
      }

      if (
        !editorControls.isEditing ||
        !shouldHandleFactoryGraphEditorKeyboardShortcut(event.target)
      ) {
        return;
      }

      if (
        isFactoryGraphEditorDeleteSelectionKeyboardEvent(event) &&
        canDeleteGraphSelection &&
        deleteGraphSelection
      ) {
        event.preventDefault();
        deleteGraphSelection();
        return;
      }

      if (
        isFactoryGraphEditorUndoKeyboardEvent(event) &&
        layoutControls.canUndo
      ) {
        event.preventDefault();
        layoutControls.undo?.();
        return;
      }

      if (
        isFactoryGraphEditorRedoKeyboardEvent(event) &&
        layoutControls.canRedo
      ) {
        event.preventDefault();
        layoutControls.redo?.();
      }
    },
    [
      canDeleteGraphSelection,
      clearGraphSelection,
      deleteGraphSelection,
      editorControls.isEditing,
      layoutControls,
    ],
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
          "h-full min-h-96",
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
        data-factory-graph-editor-canvas={
          editorControls.isEditing ? "true" : undefined
        }
        onKeyDown={handleEditorCanvasKeyDown}
        ref={flowContainerRef}
        tabIndex={0}
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
              {...FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS}
              {...touchPanePanProps}
              className="shadow-none"
              connectionLineStyle={{
                stroke: "var(--color-primary)",
                strokeWidth: 2.4,
              }}
              edges={edges}
              edgeTypes={edgeTypes}
              defaultViewport={canonicalLayoutViewport ?? undefined}
              fitView={shouldFitView}
              fitViewOptions={layoutControls.initialFitViewOptions}
              key={layoutControls.initialFitViewKey}
              isValidConnection={isValidConnection}
              maxZoom={2}
              minZoom={0.25}
              nodeTypes={nodeTypes}
              nodes={nodes}
              edgesFocusable={editorControls.isEditing}
              nodesConnectable={
                editorControls.isEditing &&
                editorControls.activeTool !== "delete"
              }
              onConnect={handleConnect}
              onInit={(instance) => {
                activeFlowInstanceRef.current = instance;
                reportPlacementViewport(instance.getViewport());
              }}
              onError={handleCurrentActivityReactFlowError}
              onEdgeClick={(_event, edge) => {
                if (
                  editorControls.isEditing &&
                  editorControls.activeTool === "delete"
                ) {
                  onEditorEdgeClick?.(
                    factoryGraphEdgeIdForRenderedEdge(nodes, edge),
                  );
                  return;
                }

                if (editorControls.isEditing) {
                  onEditorEdgeClick?.(
                    factoryGraphEdgeIdForRenderedEdge(nodes, edge),
                  );
                }
              }}
              onEdgeDoubleClick={(event, edge) => {
                if (!editorControls.isEditing || !onEditorEdgeDoubleClick) {
                  return;
                }

                const flowInstance = activeFlowInstanceRef.current;
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
                if (
                  editorControls.isEditing &&
                  editorControls.activeTool === "delete"
                ) {
                  onEditorNodeClick?.(
                    factoryGraphNodeIdForRenderedNode(nodes, node.id),
                  );
                }
              }}
              onMoveEnd={(_, viewport) => {
                reportPlacementViewport(viewport);
                if (skipNextViewportMoveEndRef.current) {
                  skipNextViewportMoveEndRef.current = false;
                  return;
                }

                updateLayoutViewport?.(viewport);
              }}
              onNodeDragStart={(_, node) => {
                if (!editorControls.isEditing) {
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
                if (moveLayoutNode && factoryGraphNodeId) {
                  const dragSession = dragSessionRef.current;
                  dragSessionRef.current = null;
                  if (
                    dragSession &&
                    dragSession.factoryGraphNodeIds.length > 1 &&
                    moveLayoutNodesByDelta
                  ) {
                    const startPosition =
                      dragSession.startPositionsByNodeId.get(
                        factoryGraphNodeId,
                      );
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
              }}
              onEdgesChange={handleEdgesChange}
              onNodesChange={handleNodesChange}
              onPaneClick={() => {
                if (editorControls.activeTool !== "delete") {
                  clearGraphSelection?.();
                }
              }}
              onSelectionChange={handleGraphSelectionChange}
              onSelectionStart={(event) => {
                handleGraphSelectionStart?.({
                  shiftKey: event.shiftKey,
                });
              }}
              proOptions={{ hideAttribution: true }}
            >
              <DashboardGraphBackground key="factory-graph-background" />
              {editorControls.isEditing &&
              visualGroups.length > 0 &&
              onSelectVisualGroup &&
              visualGroupAriaLabel ? (
                <FactoryGraphVisualGroupLayer
                  key="factory-graph-visual-groups"
                  canEdit={visualGroupCanEdit}
                  groupAriaLabel={visualGroupAriaLabel}
                  groups={visualGroups}
                  onMoveGroup={onMoveVisualGroup}
                  onResizeGroup={onResizeVisualGroup}
                  onSelectGroup={onSelectVisualGroup}
                  resizeHandleAriaLabel={
                    visualGroupResizeHandleAriaLabel ??
                    ((corner) => `Resize group from ${corner} corner`)
                  }
                  selectedGroupId={selectedVisualGroupId}
                />
              ) : null}
              <DashboardGraphControls
                key="factory-graph-controls"
                fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
              />
              {editorControls.isEditing &&
              selectedWaypointEdgeId &&
              onMoveEdgeWaypoint &&
              waypointAriaLabel ? (
                <FactoryGraphEdgeWaypointLayer
                  key="factory-graph-edge-waypoints"
                  ariaLabel={waypointAriaLabel}
                  edgeId={selectedWaypointEdgeId}
                  onMoveWaypoint={onMoveEdgeWaypoint}
                  onRemoveWaypoint={onRemoveEdgeWaypoint}
                  waypoints={selectedEdgeWaypoints}
                />
              ) : null}
            </ReactFlow>
            {editorControls.isEditing && waypointControls ? (
              <FactoryGraphEdgeWaypointControls {...waypointControls} />
            ) : null}
            {editorControls.isEditing && visualGroupControls ? (
              <FactoryGraphVisualGroupControls {...visualGroupControls} />
            ) : null}
          </div>
        ) : null}
        <FactoryGraphEditorVisibilityPanel
          locale={locale}
          onSelectPreset={visibilityControls.setPreset}
          options={buildVisibilityPresetOptions(
            locale,
            visibilityControls.preset,
          )}
          visible={false}
        />
        <CurrentActivityGraphEditorChrome
          addControls={addControls}
          canDeleteGraphSelection={canDeleteGraphSelection}
          deleteGraphSelection={deleteGraphSelection}
          editorControls={editorControls}
          graphSelectionToolbarState={graphSelectionToolbarState}
          hasPendingChanges={hasPendingChanges}
          isSavingDraft={isSavingDraft}
          layoutControls={layoutControls}
          locale={locale}
          onCreateVisualGroup={onCreateVisualGroup}
          saveControls={saveControls}
          saveDisabledReason={saveDisabledReason}
          visibilityControls={visibilityControls}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </GraphViewportSurface>
    </div>
  );
}
