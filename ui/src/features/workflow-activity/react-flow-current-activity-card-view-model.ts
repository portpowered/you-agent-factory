import {
  applyNodeChanges,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../api/dashboard/types";
import {
  type CurrentActivityNode,
  CURRENT_ACTIVITY_NODE_TYPES,
} from "../flowchart/current-activity-nodes";
import { buildGraphLayout, type GraphLayout } from "../flowchart/layout";
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
import { useCurrentActivityGraphStore } from "./state/currentActivityGraphStore";
import type { ReactFlowCurrentActivityCardProps } from "./react-flow-current-activity-card-types";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";

const GRAPH_LAYOUT_CACHE = new Map<string, GraphLayout>();
const GRAPH_LAYOUT_PROMISE_CACHE = new Map<string, Promise<GraphLayout>>();

function useGraphLayout(snapshot: DashboardSnapshot) {
  const topologyKey = useMemo(
    () => currentActivityTopologyKey(snapshot.topology),
    [snapshot.topology],
  );
  const layoutTopology = useMemo(() => snapshot.topology, [snapshot.topology]);
  const [graphLayout, setGraphLayout] =
    useState<GraphLayout>(EMPTY_GRAPH_LAYOUT);

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
      GRAPH_LAYOUT_PROMISE_CACHE.get(topologyKey) ??
      buildGraphLayout(layoutTopology);
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

function useCurrentActivityBaseNodes({
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
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkItem"
  | "onSelectWorkstation"
  | "selection"
  | "snapshot"
> & {
  activeExecutionsByWorkstationNodeID: Record<
    string,
    DashboardActiveExecution[]
  >;
  activeGraphHighlights: ReturnType<typeof buildActiveGraphHighlights>;
  activeItemLabelsByPlaceId: ReturnType<typeof buildActiveItemLabelsByPlaceId>;
  graphLayout: GraphLayout;
  handleAssignments: ReturnType<typeof buildHandleAssignments>;
  storedNodePositions: typeof EMPTY_NODE_POSITIONS;
}) {
  return useMemo<CurrentActivityNode[]>(
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
}

export function useCurrentActivityGraphViewModel({
  now,
  onSelectStateNode,
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkItem"
  | "onSelectWorkstation"
  | "selection"
  | "snapshot"
>) {
  const activeExecutions = useActiveExecutions(snapshot);
  const activeExecutionsByWorkstationNodeID = useMemo(
    () => groupActiveExecutionsByWorkstationNodeID(activeExecutions),
    [activeExecutions],
  );
  const graphLayout = useGraphLayout(snapshot);
  const graphKey = useMemo(
    () => currentActivityGraphKey(graphLayout),
    [graphLayout],
  );
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graphKey] ?? EMPTY_NODE_POSITIONS,
  );
  const setStoredNodePosition = useCurrentActivityGraphStore(
    (state) => state.setNodePosition,
  );
  const visibleGraphEdges = useMemo(
    () => buildVisibleGraphEdges(graphLayout),
    [graphLayout],
  );
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
  const baseNodes = useCurrentActivityBaseNodes({
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
  });
  const [nodes, setNodes] = useState<CurrentActivityNode[]>([]);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );
      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes(
      (currentNodes) =>
        applyNodeChanges(changes, currentNodes) as CurrentActivityNode[],
    );
  }, []);
  const edges = useMemo(
    () =>
      buildGraphEdges(
        activeGraphHighlights,
        handleAssignments,
        visibleGraphEdges,
      ),
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
    initialFitViewKey:
      initialFitViewOptions.nodes?.map((node) => node.id).join(":") ||
      "full-graph",
    initialFitViewOptions,
    nodeTypes: CURRENT_ACTIVITY_NODE_TYPES,
    nodes,
    setStoredNodePosition,
  };
}
