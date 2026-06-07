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
import { type MutableRefObject, useCallback } from "react";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
} from "../../../components/dashboard/dashboard-graph";
import { cn } from "../../../lib/cn";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  type FactoryGraphEditorMenuAction,
  FactoryGraphEditorToolbar,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { GraphViewportSurface } from "../../graphs/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
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
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
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
  onHideShowMenuOpenChange: (open: boolean) => void;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  openAddMenu?: boolean;
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
      canSave={props.canSaveDraft}
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
      onDiscard={props.handleDiscardPendingChanges}
      onHideShowMenuOpenChange={props.onHideShowMenuOpenChange}
      onSave={props.handleSaveDraft}
      onSelectTool={props.onSelectTool}
      onToggleHiddenNodeClass={props.onToggleHiddenNodeClass}
      openAddMenu={props.openAddMenu}
      saveDisabledReason={props.saveDisabledReason}
      visible={props.editorMode}
    />
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
  canSaveDraft,
  editorUnavailableClassifierWorkstationName,
  handleDiscardPendingChanges,
  handleEditorModeToggle,
  handleSaveDraft,
  hiddenNodeClasses,
  hideShowMenuOpen,
  editorMode,
  edges,
  graphKey,
  handleNodesChange,
  hasPendingChanges,
  headingID,
  imports,
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
  onToggleHiddenNodeClass,
  onConnect,
  onEditorEdgeClick,
  onEditorNodeClick,
  onSelectTool,
  openAddMenu,
  saveDisabledReason,
  setStoredNodePosition,
  flowContainerRef,
  flowInstanceRef,
}: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  handleDiscardPendingChanges: () => void;
  handleEditorModeToggle: () => void;
  handleSaveDraft: () => void;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen: boolean;
  editorMode: boolean;
  edges: Edge[];
  graphKey: string;
  handleNodesChange: (changes: NodeChange[]) => void;
  hasPendingChanges: boolean;
  headingID: string;
  imports: CurrentActivityImportController;
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
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  onConnect?: (connection: Connection) => void;
  onEditorEdgeClick?: (edgeId: string) => void;
  onEditorNodeClick?: (nodeId: string) => void;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  openAddMenu?: boolean;
  saveDisabledReason?: string;
  setStoredNodePosition: (
    graphKey: string,
    nodeId: string,
    position: XYPosition,
  ) => void;
  flowContainerRef?: MutableRefObject<HTMLElement | null>;
  flowInstanceRef?: MutableRefObject<ReactFlowInstance | null>;
}) {
  const editorMessages = getFactoryGraphEditorMessages(locale);
  const isValidConnection = buildCurrentActivityIsValidConnection({
    activeTool,
    editorMode,
    nodes,
  });
  const graphViewport =
    useMeasuredCurrentActivityGraphViewport(flowContainerRef);
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
        ref={flowContainerRef}
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
              fitView
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
              nodesDraggable={true}
              onNodeClick={(_, node) => {
                if (editorMode) {
                  onEditorNodeClick?.(
                    factoryGraphNodeIdForRenderedNode(nodes, node.id),
                  );
                }
              }}
              onNodeDragStop={(_, node) => {
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
            </ReactFlow>
          </div>
        ) : null}
        <CurrentActivityGraphEditorChrome
          activeTool={activeTool}
          addMenuActions={addMenuActions}
          canInteractWithEditor={canInteractWithEditor}
          canSaveDraft={canSaveDraft}
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
          onHideShowMenuOpenChange={onHideShowMenuOpenChange}
          onSelectTool={onSelectTool}
          onToggleHiddenNodeClass={onToggleHiddenNodeClass}
          openAddMenu={openAddMenu}
          saveDisabledReason={saveDisabledReason}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </GraphViewportSurface>
    </div>
  );
}
