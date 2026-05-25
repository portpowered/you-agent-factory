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

import { cn } from "../../../lib/cn";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
} from "../../../components/dashboard/dashboard-graph";
import {
  FactoryGraphEditorToolbar,
  type FactoryGraphEditorMenuAction,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  GraphDropOverlay,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";

const CURRENT_ACTIVITY_LEGEND_CLASS =
  "absolute left-7 top-7 z-10 max-md:left-4 max-md:right-4 max-md:top-4";

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

    return isValidFactoryGraphConnection({
      sourceAnchorId: connection.sourceHandle,
      sourceNodeKind,
      targetAnchorId: connection.targetHandle,
      targetNodeKind,
    });
  };
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

  return (
    <div className="relative min-h-0 flex-1">
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
          "relative h-full min-h-0 overflow-hidden rounded-3xl border transition-colors",
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
          onConnect={onConnect}
          onEdgeClick={(_, edge) => {
            if (editorMode) {
              onEditorEdgeClick?.(edge.id);
            }
          }}
          nodesDraggable={true}
          onNodeClick={(_, node) => {
            if (editorMode) {
              onEditorNodeClick?.(node.id);
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
