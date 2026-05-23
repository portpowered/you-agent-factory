import type { DashboardActiveExecution, DashboardSnapshot } from "../../../api/dashboard/types";
import type { CurrentActivityNode } from "../../flowchart/public";
import type { GraphLayout, PositionedEdge, PositionedPlaceNode, PositionedWorkstationNode } from "../../flowchart/lib/layout";
import type { CurrentActivitySelection } from "../components/react-flow-current-activity-card";
import type {
  GraphNodePosition,
  GraphNodePositions,
} from "../state/currentActivityGraphStore";

export const EMPTY_GRAPH_LAYOUT: GraphLayout = {
  edges: [],
  height: 0,
  nodes: [],
  width: 0,
};
export const EMPTY_NODE_POSITIONS: GraphNodePositions = {};

export interface HandleAssignments {
  incomingHandleCounts: Map<string, number>;
  outgoingHandleCounts: Map<string, number>;
  sourceHandlesByEdgeId: Map<string, string>;
  targetHandlesByEdgeId: Map<string, string>;
}

export interface ActiveGraphHighlights {
  activeEdgeIds: ReadonlySet<string>;
  activePlaceNodeIds: ReadonlySet<string>;
  activeWorkstationNodeIds: ReadonlySet<string>;
  hasActiveFlow: boolean;
  relatedNodeIds: ReadonlySet<string>;
}

function edgeTouchesResource(edge: PositionedEdge): boolean {
  return (
    edge.sourcePlaceKind === "resource" || edge.targetPlaceKind === "resource"
  );
}

function edgeReturnsToResource(edge: PositionedEdge): boolean {
  return edge.targetPlaceKind === "resource";
}

function activeTokenLabel(
  execution: DashboardActiveExecution,
  workID: string,
  fallbackID: string,
): string {
  const workItem = execution.work_items?.find(
    (item) => item.work_id === workID,
  );
  return workItem?.display_name || workItem?.work_id || fallbackID;
}

function workstationGraphNodeId(nodeId: string): string {
  return `workstation:${nodeId}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
}

export function buildHandleAssignments(
  edges: PositionedEdge[],
): HandleAssignments {
  const incomingHandleCounts = new Map<string, number>();
  const outgoingHandleCounts = new Map<string, number>();
  const sourceHandlesByEdgeId = new Map<string, string>();
  const targetHandlesByEdgeId = new Map<string, string>();

  for (const edge of edges) {
    const sourceIndex = outgoingHandleCounts.get(edge.fromNodeId) ?? 0;
    const targetIndex = incomingHandleCounts.get(edge.toNodeId) ?? 0;

    sourceHandlesByEdgeId.set(edge.edgeId, `out-${sourceIndex}`);
    targetHandlesByEdgeId.set(edge.edgeId, `in-${targetIndex}`);
    outgoingHandleCounts.set(edge.fromNodeId, sourceIndex + 1);
    incomingHandleCounts.set(edge.toNodeId, targetIndex + 1);
  }

  return {
    incomingHandleCounts,
    outgoingHandleCounts,
    sourceHandlesByEdgeId,
    targetHandlesByEdgeId,
  };
}

export function buildActiveGraphHighlights(
  activeExecutions: DashboardActiveExecution[],
  edges: PositionedEdge[],
): ActiveGraphHighlights {
  const activeEdgeIds = new Set<string>();
  const activePlaceNodeIds = new Set<string>();
  const activeWorkstationNodeIds = new Set<string>();
  const consumedPlaceNodeIds = new Set<string>();
  const relatedNodeIds = new Set<string>();

  for (const execution of activeExecutions) {
    const workstationNodeId = workstationGraphNodeId(
      execution.workstation_node_id,
    );
    activeWorkstationNodeIds.add(workstationNodeId);
    relatedNodeIds.add(workstationNodeId);

    for (const token of execution.consumed_tokens ?? []) {
      const placeNodeId = placeGraphNodeId(token.place_id);
      consumedPlaceNodeIds.add(placeNodeId);
      relatedNodeIds.add(placeNodeId);
    }
  }

  for (const edge of edges) {
    const resourceEdge = edgeTouchesResource(edge);
    const flowsIntoActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.toNodeId) &&
      consumedPlaceNodeIds.has(edge.fromNodeId);
    const flowsOutOfActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.fromNodeId) &&
      !(edge.outcomeKind === "failed" || edge.stateCategory === "FAILED");

    if (!flowsIntoActiveWorkstation && !flowsOutOfActiveWorkstation) {
      continue;
    }

    activeEdgeIds.add(edge.edgeId);
    relatedNodeIds.add(edge.fromNodeId);
    relatedNodeIds.add(edge.toNodeId);

    if (flowsOutOfActiveWorkstation) {
      activePlaceNodeIds.add(edge.toNodeId);
    }
  }

  return {
    activeEdgeIds,
    activePlaceNodeIds,
    activeWorkstationNodeIds,
    hasActiveFlow: activeExecutions.length > 0,
    relatedNodeIds,
  };
}

export function buildVisibleGraphEdges(
  graphLayout: GraphLayout,
): PositionedEdge[] {
  return graphLayout.edges.filter((edge) => !edgeReturnsToResource(edge));
}

export function buildActiveItemLabelsByPlaceId(
  activeExecutions: DashboardActiveExecution[],
) {
  const labelsByPlaceId = new Map<string, string[]>();
  const seenByPlaceId = new Map<string, Set<string>>();

  for (const execution of activeExecutions) {
    for (const token of execution.consumed_tokens ?? []) {
      const label =
        token.name ||
        activeTokenLabel(execution, token.work_id, token.token_id);
      const placeLabels = labelsByPlaceId.get(token.place_id) ?? [];
      const seenLabels = seenByPlaceId.get(token.place_id) ?? new Set<string>();
      if (seenLabels.has(label)) {
        continue;
      }

      seenLabels.add(label);
      placeLabels.push(label);
      seenByPlaceId.set(token.place_id, seenLabels);
      labelsByPlaceId.set(token.place_id, placeLabels);
    }
  }

  return labelsByPlaceId;
}

function finitePosition(
  position: GraphNodePosition | undefined,
): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

function nodePosition(
  nodeId: string,
  fallback: GraphNodePosition,
  storedPositions: GraphNodePositions,
): GraphNodePosition {
  const storedPosition = storedPositions[nodeId];
  return finitePosition(storedPosition) ? storedPosition : fallback;
}

interface BuildCurrentActivityNodesInput {
  activeExecutionsByWorkstationNodeID: Record<
    string,
    DashboardActiveExecution[]
  >;
  activeGraphHighlights: ActiveGraphHighlights;
  activeItemLabelsByPlaceId: Map<string, string[]>;
  graphLayout: GraphLayout;
  handleAssignments: HandleAssignments;
  locale?: string;
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  storedNodePositions: GraphNodePositions;
}

function buildPlaceNode(
  positionedNode: PositionedPlaceNode,
  input: BuildCurrentActivityNodesInput,
): CurrentActivityNode {
  const place = positionedNode.place;
  const position = nodePosition(
    positionedNode.nodeId,
    { x: positionedNode.x, y: positionedNode.y },
    input.storedNodePositions,
  );
  const basePlaceNode = {
    className: "border-0 bg-transparent p-0 text-af-ink",
    draggable: true,
    height: positionedNode.height,
    id: positionedNode.nodeId,
    initialHeight: positionedNode.height,
    initialWidth: positionedNode.width,
    measured: { height: positionedNode.height, width: positionedNode.width },
    position,
    width: positionedNode.width,
  };
  const basePlaceData = {
    activeFlow: input.activeGraphHighlights.activePlaceNodeIds.has(
      positionedNode.nodeId,
    ),
    activeItemLabels:
      input.activeItemLabelsByPlaceId.get(place.place_id) ?? [],
    incomingHandleCount:
      input.handleAssignments.incomingHandleCounts.get(positionedNode.nodeId) ??
      1,
    locale: input.locale,
    muted:
      place.kind !== "resource" &&
      input.activeGraphHighlights.hasActiveFlow &&
      !input.activeGraphHighlights.relatedNodeIds.has(positionedNode.nodeId),
    outgoingHandleCount:
      input.handleAssignments.outgoingHandleCounts.get(positionedNode.nodeId) ??
      1,
    selectedStateNode:
      input.selection?.kind === "state-node" &&
      input.selection.placeId === place.place_id,
    tokenCount:
      input.snapshot.runtime.place_token_counts?.[place.place_id] ?? 0,
  };

  if (place.kind === "work_state") {
    return {
      ...basePlaceNode,
      data: {
        ...basePlaceData,
        onSelectStateNode: input.onSelectStateNode,
        place,
      },
      selectable: true,
      type: "statePosition",
    };
  }

  if (place.kind === "resource") {
    return {
      ...basePlaceNode,
      data: { ...basePlaceData, place },
      selectable: false,
      type: "resource",
    };
  }

  return {
    ...basePlaceNode,
    data: { ...basePlaceData, place },
    selectable: false,
    type: "constraint",
  };
}

function buildWorkstationNode(
  positionedNode: PositionedWorkstationNode,
  input: BuildCurrentActivityNodesInput,
): CurrentActivityNode | null {
  const workstation =
    input.snapshot.topology.workstation_nodes_by_id[
      positionedNode.workstationNodeId
    ];
  if (!workstation) {
    return null;
  }

  const executions =
    input.activeExecutionsByWorkstationNodeID[workstation.node_id] ?? [];
  const position = nodePosition(
    positionedNode.nodeId,
    { x: positionedNode.x, y: positionedNode.y },
    input.storedNodePositions,
  );

  return {
    className: "border-0 bg-transparent p-0 text-af-ink",
    data: {
      active: executions.length > 0,
      activeFlow: input.activeGraphHighlights.activeWorkstationNodeIds.has(
        positionedNode.nodeId,
      ),
      executions,
      incomingHandleCount:
        input.handleAssignments.incomingHandleCounts.get(positionedNode.nodeId) ??
        1,
      locale: input.locale,
      muted:
        input.activeGraphHighlights.hasActiveFlow &&
        !input.activeGraphHighlights.relatedNodeIds.has(positionedNode.nodeId),
      now: input.now,
      onSelectWorkID: input.onSelectWorkID,
      onSelectWorkstation: input.onSelectWorkstation,
      outgoingHandleCount:
        input.handleAssignments.outgoingHandleCounts.get(positionedNode.nodeId) ??
        1,
      selectedWorkID:
        input.selection?.kind === "work-item" &&
        input.selection.nodeId === workstation.node_id
          ? input.selection.workID
          : null,
      selectedWorkstation:
        input.selection?.kind === "node" &&
        input.selection.nodeId === workstation.node_id,
      workstation,
    },
    draggable: true,
    height: positionedNode.height,
    id: positionedNode.nodeId,
    initialHeight: positionedNode.height,
    initialWidth: positionedNode.width,
    measured: {
      height: positionedNode.height,
      width: positionedNode.width,
    },
    position,
    selectable: true,
    type: "workstation",
    width: positionedNode.width,
  };
}

export function buildCurrentActivityNodes({
  activeExecutionsByWorkstationNodeID,
  activeGraphHighlights,
  activeItemLabelsByPlaceId,
  graphLayout,
  handleAssignments,
  locale,
  now,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorkstation,
  selection,
  snapshot,
  storedNodePositions,
}: BuildCurrentActivityNodesInput): CurrentActivityNode[] {
  const nextNodes: CurrentActivityNode[] = [];
  const input = {
    activeExecutionsByWorkstationNodeID,
    activeGraphHighlights,
    activeItemLabelsByPlaceId,
    graphLayout,
    handleAssignments,
    locale,
    now,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorkstation,
    selection,
    snapshot,
    storedNodePositions,
  } satisfies BuildCurrentActivityNodesInput;

  for (const positionedNode of graphLayout.nodes) {
    if (positionedNode.nodeKind !== "workstation") {
      nextNodes.push(buildPlaceNode(positionedNode as PositionedPlaceNode, input));
      continue;
    }

    const workstationNode = buildWorkstationNode(
      positionedNode as PositionedWorkstationNode,
      input,
    );
    if (workstationNode) {
      nextNodes.push(workstationNode);
    }
  }

  return nextNodes;
}
