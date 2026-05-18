import {
  Background,
  type Connection,
  Controls,
  type Edge,
  type FitViewOptions,
  type IsValidConnection,
  type Node,
  type NodeChange,
  type NodeTypes,
  ReactFlow,
  type XYPosition,
} from "@xyflow/react";
import type { CSSProperties } from "react";

import { cx } from "../../lib/cx";
import type { FactoryGraphNodeKind } from "../factory-graph-editor/factory-graph-draft-types";
import { isValidFactoryGraphConnection } from "../factory-graph-editor/factory-graph-editor-connections";
import {
  FactoryGraphEditorToolbar,
  type FactoryGraphEditorMenuAction,
} from "../factory-graph-editor/factory-graph-editor-controls";
import type { CurrentActivityImportController } from "./current-activity-import-controller";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import {
  GraphDropOverlay,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";

const GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const GRAPH_BACKGROUND_GAP = 24;
const GRAPH_BACKGROUND_SIZE = 1;
const CURRENT_ACTIVITY_LEGEND_CLASS =
  "absolute left-7 top-7 z-10 max-[720px]:left-4 max-[720px]:right-4 max-[720px]:top-4";

type CSSPropertiesWithVariables = CSSProperties &
  Record<`--${string}`, string | number>;

const GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  "--xy-controls-button-color-props":
    "rgb(from var(--color-af-ink) r g b / 0.72)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

export function CurrentActivityGraphViewport({
  activeTool,
  addMenuActions,
  canInteractWithEditor,
  editorMode,
  edges,
  graphKey,
  handleNodesChange,
  hasPendingChanges,
  imports,
  initialFitViewKey,
  initialFitViewOptions,
  locale,
  nodeTypes,
  nodes,
  onAddAction,
  onAddMenuOpenChange,
  onConnect,
  onEditorNodeClick,
  onSelectTool,
  openAddMenu,
  setStoredNodePosition,
}: {
  activeTool: "add" | "connect" | "delete" | null;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteractWithEditor: boolean;
  editorMode: boolean;
  edges: Edge[];
  graphKey: string;
  handleNodesChange: (changes: NodeChange[]) => void;
  hasPendingChanges: boolean;
  imports: CurrentActivityImportController;
  initialFitViewKey: string;
  initialFitViewOptions: FitViewOptions;
  locale?: string;
  nodeTypes: NodeTypes;
  nodes: Node[];
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onConnect?: (connection: Connection) => void;
  onEditorNodeClick?: (nodeId: string) => void;
  onSelectTool: (tool: "add" | "connect" | "delete" | null) => void;
  openAddMenu?: boolean;
  setStoredNodePosition: (
    graphKey: string,
    nodeId: string,
    position: XYPosition,
  ) => void;
}) {
  const isValidConnection: IsValidConnection = (connection) => {
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
    const sourceNodeKind = (sourceNode.data as { kind?: FactoryGraphNodeKind }).kind;
    const targetNodeKind = (targetNode.data as { kind?: FactoryGraphNodeKind }).kind;
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
        aria-describedby="workflow-graph-heading"
        aria-label="Work graph viewport"
        className={cx(
          "relative h-full min-h-0 overflow-hidden rounded-[1.4rem] border transition-colors",
          (imports.dropState.status === "drag-active" ||
            imports.dropState.status === "reading") &&
            "border-af-accent/35 bg-af-accent/6",
          imports.dropState.status === "error" && "border-af-danger/18",
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
          edges={edges}
          fitView
          fitViewOptions={initialFitViewOptions}
          key={initialFitViewKey}
          isValidConnection={isValidConnection}
          maxZoom={2}
          minZoom={0.25}
          nodeTypes={nodeTypes}
          nodes={nodes}
          nodesConnectable={editorMode && activeTool === "connect"}
          onConnect={onConnect}
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
          <Background
            color={GRAPH_BACKGROUND_COLOR}
            gap={GRAPH_BACKGROUND_GAP}
            size={GRAPH_BACKGROUND_SIZE}
          />
          <Controls
            fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
            showInteractive={false}
            style={GRAPH_CONTROLS_STYLE}
          />
        </ReactFlow>
        <FactoryGraphEditorToolbar
          activeTool={activeTool}
          addMenuActions={addMenuActions}
          canInteract={canInteractWithEditor}
          hasPendingChanges={hasPendingChanges}
          onAddAction={onAddAction}
          onAddMenuOpenChange={onAddMenuOpenChange}
          onSelectTool={onSelectTool}
          openAddMenu={openAddMenu}
          visible={editorMode}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </section>
    </div>
  );
}
