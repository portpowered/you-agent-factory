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
  type XYPosition,
} from "@xyflow/react";
import { useCallback } from "react";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
} from "../../../components/dashboard/dashboard-graph";
import { cn } from "../../../lib/cn";
import {
  type FactoryGraphEditorMenuAction,
  FactoryGraphEditorToolbar,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { handleCurrentActivityReactFlowError } from "../lib/react-flow-current-activity-card-errors";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import {
  GraphDropOverlay,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";

const CURRENT_ACTIVITY_LEGEND_CLASS = "absolute left-4 right-4 top-4 z-10";

function CurrentActivityEditorToolbar(props: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  editorMode: boolean;
  handleDiscardPendingChanges: () => void;
  handleSaveDraft: () => void;
  hasPendingChanges: boolean;
  isSavingDraft?: boolean;
  locale?: string;
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  openAddMenu?: boolean;
  saveDisabledReason?: string;
}) {
  return (
    <FactoryGraphEditorToolbar
      activeTool={props.activeTool}
      addMenuActions={props.addMenuActions}
      canDiscard={props.hasPendingChanges}
      canInteract={props.canInteractWithEditor}
      canSave={props.canSaveDraft}
      hasPendingChanges={props.hasPendingChanges}
      isSaving={props.isSavingDraft}
      locale={props.locale}
      onAddAction={props.onAddAction}
      onAddMenuOpenChange={props.onAddMenuOpenChange}
      onDiscard={props.handleDiscardPendingChanges}
      onSave={props.handleSaveDraft}
      onSelectTool={props.onSelectTool}
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

export function CurrentActivityGraphViewport({
  activeTool,
  addMenuActions,
  canInteractWithEditor,
  canSaveDraft,
  handleDiscardPendingChanges,
  handleSaveDraft,
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
  onConnect,
  onEditorEdgeClick,
  onEditorNodeClick,
  onSelectTool,
  openAddMenu,
  saveDisabledReason,
  setStoredNodePosition,
}: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  handleDiscardPendingChanges: () => void;
  handleSaveDraft: () => void;
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
}) {
  const editorMessages = getFactoryGraphEditorMessages(locale);
  const isValidConnection = buildCurrentActivityIsValidConnection({
    activeTool,
    editorMode,
    nodes,
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

  return (
    <div className="relative min-h-96 flex-1">
      <DashboardFlowAxisLegend
        className={CURRENT_ACTIVITY_LEGEND_CLASS}
        defaultExpanded={false}
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems(locale)}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems(locale)}
        locale={locale}
      />
      <section
        aria-describedby={headingID}
        aria-label={editorMessages.viewportLabel}
        className={cn(
          "relative h-full min-h-96 overflow-hidden rounded-3xl border transition-colors",
          (imports.dropState.status === "drag-active" ||
            imports.dropState.status === "reading") &&
            "border-af-accent-border bg-af-accent-surface",
          imports.dropState.status === "error" &&
            "border-af-danger-border bg-af-danger-surface",
          imports.dropState.status === "idle" && "border-transparent",
        )}
        data-current-activity-drop-state={graphDropStateAttribute(
          imports.dropState,
        )}
        data-current-activity-flow
        onDragEnter={imports.onDragEnter}
        onDragLeave={imports.onDragLeave}
        onDragOver={imports.onDragOver}
        onDrop={imports.onDrop}
      >
        <ReactFlow
          connectionLineStyle={{
            stroke: "var(--color-af-accent)",
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
        <CurrentActivityEditorToolbar
          activeTool={activeTool}
          addMenuActions={addMenuActions}
          canInteractWithEditor={canInteractWithEditor}
          canSaveDraft={canSaveDraft}
          editorMode={editorMode}
          handleDiscardPendingChanges={handleDiscardPendingChanges}
          handleSaveDraft={handleSaveDraft}
          hasPendingChanges={hasPendingChanges}
          isSavingDraft={isSavingDraft}
          locale={locale}
          onAddAction={onAddAction}
          onAddMenuOpenChange={onAddMenuOpenChange}
          onSelectTool={onSelectTool}
          openAddMenu={openAddMenu}
          saveDisabledReason={saveDisabledReason}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </section>
    </div>
  );
}
