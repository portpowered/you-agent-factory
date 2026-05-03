import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  Background,
  Controls,
  ReactFlow,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import type { CSSProperties } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { DashboardActiveExecution, DashboardSnapshot, DashboardWorkItemRef } from "../../api/dashboard/types";
import type { FactoryValue } from "../../api/named-factory";
import { cx } from "../../lib/cx";
import { useCurrentActivityGraphStore } from "../../state/currentActivityGraphStore";
import {
  type FactoryPngImportValue,
  type ReadFactoryImportFile,
  useFactoryImportActivation,
  useFactoryImportPreview,
  useFactoryPngDrop,
} from "../import";
import { CURRENT_ACTIVITY_NODE_TYPES, type CurrentActivityNode } from "../flowchart/current-activity-nodes";
import { buildGraphLayout, type GraphLayout } from "../flowchart/layout";
import {
  DashboardFlowAxisLegend,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS,
} from "./dashboard-flow-axis-legend";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildGraphEdges,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_GRAPH_LAYOUT,
  EMPTY_NODE_POSITIONS,
  initialFocusNodes,
} from "./react-flow-current-activity-card-graph";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";
import {
  FactoryImportPreviewDialog,
  GraphDropOverlay,
  graphDropStateAttribute,
  GraphImportErrorPanel,
} from "./react-flow-current-activity-card-import";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";

const GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const GRAPH_BACKGROUND_GAP = 24;
const GRAPH_BACKGROUND_SIZE = 1;

type CSSPropertiesWithVariables = CSSProperties & Record<`--${string}`, string | number>;

const GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  "--xy-controls-button-color-props": "rgb(from var(--color-af-ink) r g b / 0.72)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

const GRAPH_LAYOUT_CACHE = new Map<string, GraphLayout>();
const GRAPH_LAYOUT_PROMISE_CACHE = new Map<string, Promise<GraphLayout>>();

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}

function useGraphLayout(snapshot: DashboardSnapshot) {
  const topologyKey = useMemo(
    () => currentActivityTopologyKey(snapshot.topology),
    [snapshot.topology],
  );
  const layoutTopology = useMemo(() => snapshot.topology, [snapshot.topology]);
  const [graphLayout, setGraphLayout] = useState<GraphLayout>(EMPTY_GRAPH_LAYOUT);

  useEffect(() => {
    let cancelled = false;
    const cachedLayout = GRAPH_LAYOUT_CACHE.get(topologyKey);
    if (cachedLayout) {
      setGraphLayout(cachedLayout);
      return () => {
        cancelled = true;
      };
    }

    const inFlightLayout =
      GRAPH_LAYOUT_PROMISE_CACHE.get(topologyKey) ?? buildGraphLayout(layoutTopology);
    GRAPH_LAYOUT_PROMISE_CACHE.set(topologyKey, inFlightLayout);

    inFlightLayout
      .then((layout) => {
        GRAPH_LAYOUT_CACHE.set(topologyKey, layout);
        GRAPH_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setGraphLayout(layout);
        }
      })
      .catch(() => {
        GRAPH_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setGraphLayout(EMPTY_GRAPH_LAYOUT);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [layoutTopology, topologyKey]);

  return graphLayout;
}

function useCurrentActivityImport({
  activateFactory,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  "activateFactory" | "onFactoryActivated" | "onFactoryImportReady" | "readFactoryImportFile"
>) {
  const {
    closePreview: closeImportPreview,
    openPreview,
    previewState: importPreviewState,
  } = useFactoryImportPreview({
    onPreviewReady: onFactoryImportReady,
  });
  const handleFactoryActivated = useCallback(() => {
    closeImportPreview();
    onFactoryActivated?.();
  }, [closeImportPreview, onFactoryActivated]);
  const {
    activateImport,
    activationState,
    clearActivationError,
  } = useFactoryImportActivation({
    activateFactory,
    onActivated: handleFactoryActivated,
  });
  const handleImportPreviewReady = useCallback((value: FactoryPngImportValue, file: File) => {
    clearActivationError();
    openPreview(value, file);
  }, [clearActivationError, openPreview]);
  const drop = useFactoryPngDrop({
    onImportReady: handleImportPreviewReady,
    readFactoryImportFile,
  });

  return {
    activateImport,
    activationState,
    clearActivationError,
    closeImportPreview,
    importPreviewState,
    ...drop,
  };
}

function useCurrentActivityGraphViewModel({
  now,
  onSelectStateNode,
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  "now" | "onSelectStateNode" | "onSelectWorkItem" | "onSelectWorkstation" | "selection" | "snapshot"
>) {
  const activeExecutions = useMemo(
    () =>
      snapshot.runtime.active_dispatch_ids
        ?.map((dispatchId) => snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId])
        .filter((execution): execution is DashboardActiveExecution => execution !== undefined) ??
      [],
    [snapshot.runtime.active_dispatch_ids, snapshot.runtime.active_executions_by_dispatch_id],
  );
  const activeExecutionsByWorkstationNodeID = useMemo(
    () =>
      activeExecutions.reduce<Record<string, DashboardActiveExecution[]>>((accumulator, execution) => {
        const executions = accumulator[execution.workstation_node_id] ?? [];
        accumulator[execution.workstation_node_id] = [...executions, execution];
        return accumulator;
      }, {}),
    [activeExecutions],
  );
  const graphLayout = useGraphLayout(snapshot);
  const graphKey = useMemo(() => currentActivityGraphKey(graphLayout), [graphLayout]);
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graphKey] ?? EMPTY_NODE_POSITIONS,
  );
  const setStoredNodePosition = useCurrentActivityGraphStore((state) => state.setNodePosition);
  const visibleGraphEdges = useMemo(() => buildVisibleGraphEdges(graphLayout), [graphLayout]);
  const handleAssignments = useMemo(
    () => buildHandleAssignments(visibleGraphEdges),
    [visibleGraphEdges],
  );
  const activeGraphHighlights = useMemo(
    () => buildActiveGraphHighlights(activeExecutions, visibleGraphEdges),
    [activeExecutions, visibleGraphEdges],
  );
  const activeItemLabelsByPlaceId = useMemo(
    () => buildActiveItemLabelsByPlaceId(activeExecutions),
    [activeExecutions],
  );
  const baseNodes = useMemo<CurrentActivityNode[]>(
    () =>
      buildCurrentActivityNodes({
        activeExecutionsByWorkstationNodeID,
        activeGraphHighlights,
        activeItemLabelsByPlaceId,
        graphLayout,
        handleAssignments,
        now,
        onSelectStateNode,
        onSelectWorkItem,
        onSelectWorkstation,
        selection,
        snapshot,
        storedNodePositions,
      }),
    [
      activeExecutionsByWorkstationNodeID,
      activeGraphHighlights,
      activeItemLabelsByPlaceId,
      graphLayout,
      handleAssignments,
      now,
      onSelectStateNode,
      onSelectWorkItem,
      onSelectWorkstation,
      selection,
      snapshot,
      storedNodePositions,
    ],
  );
  const [nodes, setNodes] = useState<CurrentActivityNode[]>([]);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(currentNodes.map((node) => [node.id, node.position]));
      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes((currentNodes) => applyNodeChanges(changes, currentNodes) as CurrentActivityNode[]);
  }, []);
  const edges = useMemo(
    () => buildGraphEdges(activeGraphHighlights, handleAssignments, visibleGraphEdges),
    [activeGraphHighlights, handleAssignments, visibleGraphEdges],
  );
  const initialFitViewOptions = useMemo<FitViewOptions>(
    () => ({
      maxZoom: 1.15,
      minZoom: 0.7,
      nodes: initialFocusNodes(graphLayout),
      padding: 0.18,
    }),
    [graphLayout],
  );

  return {
    edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey: initialFitViewOptions.nodes?.map((node) => node.id).join(":") || "full-graph",
    initialFitViewOptions,
    nodes,
    setStoredNodePosition,
  };
}

function EmptyCurrentActivityCard() {
  return (
    <section
      aria-labelledby="workflow-graph-heading"
      className="relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4"
    >
      <div className="mb-4 flex items-end justify-between gap-4 max-[720px]:flex-col max-[720px]:items-start [&_h2]:m-0">
        <div>
          <p className="mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent">
            Operator View
          </p>
          <h2 id="workflow-graph-heading">Current activity</h2>
        </div>
      </div>
      <div className="grid min-h-60 items-start gap-[0.35rem] rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0">
        <h3>No workflow topology loaded</h3>
        <p>The factory has not published any workstation graph yet.</p>
      </div>
    </section>
  );
}

export function ReactFlowCurrentActivityCard(props: ReactFlowCurrentActivityCardProps) {
  const graph = useCurrentActivityGraphViewModel(props);
  const imports = useCurrentActivityImport(props);

  if (props.snapshot.topology.workstation_node_ids.length === 0) {
    return <EmptyCurrentActivityCard />;
  }

  const readyImportPreviewState =
    imports.importPreviewState.status === "ready" ? imports.importPreviewState : null;

  return (
    <section
      aria-labelledby="workflow-graph-heading"
      className="relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4"
    >
      <DashboardFlowAxisLegend
        className="absolute left-7 top-7 max-[720px]:left-4 max-[720px]:right-4"
        defaultExpanded={false}
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />

      <div className="relative min-h-0 flex-1">
        <section
          aria-label="Work graph viewport"
          className={cx(
            "relative h-full min-h-0 overflow-hidden rounded-[1.4rem] border transition-colors",
            (imports.dropState.status === "drag-active" || imports.dropState.status === "reading") &&
              "border-af-accent/35 bg-af-accent/6",
            imports.dropState.status === "error" && "border-af-danger/18",
            imports.dropState.status === "idle" && "border-transparent",
          )}
          data-current-activity-drop-state={graphDropStateAttribute(imports.dropState)}
          data-current-activity-flow
          onDragEnter={imports.onDragEnter}
          onDragLeave={imports.onDragLeave}
          onDragOver={imports.onDragOver}
          onDrop={imports.onDrop}
        >
          <ReactFlow
            edges={graph.edges}
            fitView
            fitViewOptions={graph.initialFitViewOptions}
            key={graph.initialFitViewKey}
            maxZoom={2}
            minZoom={0.25}
            nodeTypes={CURRENT_ACTIVITY_NODE_TYPES}
            nodes={graph.nodes}
            nodesDraggable={true}
            onNodeDragStop={(_, node) => {
              if (graph.graphKey) {
                graph.setStoredNodePosition(graph.graphKey, node.id, node.position);
              }
            }}
            onNodesChange={graph.handleNodesChange}
            panOnDrag
            proOptions={{ hideAttribution: true }}
            zoomOnScroll
          >
            <Background color={GRAPH_BACKGROUND_COLOR} gap={GRAPH_BACKGROUND_GAP} size={GRAPH_BACKGROUND_SIZE} />
            <Controls
              fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
              showInteractive={false}
              style={GRAPH_CONTROLS_STYLE}
            />
          </ReactFlow>
          <GraphDropOverlay dropState={imports.dropState} />
        </section>
        {readyImportPreviewState ? (
          <FactoryImportPreviewDialog
            activationState={imports.activationState}
            onCancel={() => {
              imports.clearActivationError();
              imports.closeImportPreview();
            }}
            onConfirm={() => {
              void imports.activateImport(readyImportPreviewState.value);
            }}
            previewState={readyImportPreviewState}
          />
        ) : null}
        {imports.dropState.status === "error" ? (
          <GraphImportErrorPanel
            error={imports.dropState.error}
            fileName={imports.dropState.fileName}
            onDismiss={imports.clearError}
          />
        ) : null}
      </div>
    </section>
  );
}
