import { MarkerType, type Edge, type FitViewOptions } from "@xyflow/react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type {
  GraphNodePosition,
  GraphNodePositions,
} from "./state/currentActivityGraphStore";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import type { CurrentActivityNode } from "../flowchart/current-activity-nodes";
import type {
  GraphLayout,
  PositionedEdge,
  PositionedPlaceNode,
  PositionedWorkstationNode,
} from "../flowchart/layout";

const EDGE_STROKE_MUTED = "var(--color-af-edge-muted)";
const EDGE_STROKE_SOFT = "var(--color-af-edge-muted-soft)";
const EDGE_STROKE_DANGER_MUTED = "var(--color-af-edge-danger-muted)";
const EDGE_STROKE_ACTIVE = "var(--color-af-success)";

export const EMPTY_GRAPH_LAYOUT: GraphLayout = { edges: [], height: 0, nodes: [], width: 0 };
export const EMPTY_NODE_POSITIONS: GraphNodePositions = {};

interface HandleAssignments {
  incomingHandleCounts: Map<string, number>;
  outgoingHandleCounts: Map<string, number>;
  sourceHandlesByEdgeId: Map<string, string>;
  targetHandlesByEdgeId: Map<string, string>;
}

interface ActiveGraphHighlights {
  activeEdgeIds: ReadonlySet<string>;
  activePlaceNodeIds: ReadonlySet<string>;
  activeWorkstationNodeIds: ReadonlySet<string>;
  hasActiveFlow: boolean;
  relatedNodeIds: ReadonlySet<string>;
}

function edgeIsFailure(edge: PositionedEdge): boolean {
  return edge.outcomeKind === "failed" || edge.stateCategory === "FAILED";
}

function edgeTouchesResource(edge: PositionedEdge): boolean {
  return edge.sourcePlaceKind === "resource" || edge.targetPlaceKind === "resource";
}

function edgeReturnsToResource(edge: PositionedEdge): boolean {
  return edge.targetPlaceKind === "resource";
}

function edgeStyle(edge: PositionedEdge, activeFlow: boolean, muted: boolean): Edge["style"] {
  if (activeFlow) {
    return {
      stroke: EDGE_STROKE_ACTIVE,
      strokeDasharray: "8 6",
      strokeWidth: 2.8,
    };
  }

  const opacity = muted ? 0.34 : undefined;

  if (edge.sourcePlaceKind === "resource") {
    return {
      opacity,
      stroke: EDGE_STROKE_SOFT,
      strokeDasharray: "2 7",
      strokeWidth: 1.5,
    };
  }
  if (edge.outcomeKind === "accepted") {
    return { opacity, stroke: EDGE_STROKE_MUTED, strokeWidth: 1.6 };
  }
  if (edgeIsFailure(edge)) {
    return {
      opacity: muted ? 0.68 : undefined,
      stroke: EDGE_STROKE_DANGER_MUTED,
      strokeDasharray: "3 6",
      strokeWidth: 1.8,
    };
  }
  return {
    opacity,
    stroke: EDGE_STROKE_MUTED,
    strokeDasharray: "8 8",
    strokeWidth: 1.6,
  };
}

function edgeMarkerColor(edge: PositionedEdge, activeFlow: boolean): string {
  if (activeFlow) {
    return EDGE_STROKE_ACTIVE;
  }
  if (edgeIsFailure(edge)) {
    return EDGE_STROKE_DANGER_MUTED;
  }
  return edge.sourcePlaceKind === "resource" ? EDGE_STROKE_SOFT : EDGE_STROKE_MUTED;
}

function edgeSemantic(edge: PositionedEdge): boolean {
  return edge.outcomeKind !== "accepted" || edgeIsFailure(edge);
}

function edgeLabel(edge: PositionedEdge, activeFlow: boolean): string | undefined {
  return activeFlow ? edge.label || undefined : undefined;
}

function activeTokenLabel(execution: DashboardActiveExecution, workID: string, fallbackID: string): string {
  const workItem = execution.work_items?.find((item) => item.work_id === workID);
  return workItem?.display_name || workItem?.work_id || fallbackID;
}

function workstationGraphNodeId(nodeId: string): string {
  return `workstation:${nodeId}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
}

export function buildHandleAssignments(edges: PositionedEdge[]): HandleAssignments {
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
    const workstationNodeId = workstationGraphNodeId(execution.workstation_node_id);
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
      !edgeIsFailure(edge);

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

export function buildVisibleGraphEdges(graphLayout: GraphLayout): PositionedEdge[] {
  return graphLayout.edges.filter((edge) => !edgeReturnsToResource(edge));
}

export function buildActiveItemLabelsByPlaceId(activeExecutions: DashboardActiveExecution[]) {
  const labelsByPlaceId = new Map<string, string[]>();
  const seenByPlaceId = new Map<string, Set<string>>();

  for (const execution of activeExecutions) {
    for (const token of execution.consumed_tokens ?? []) {
      const label = token.name || activeTokenLabel(execution, token.work_id, token.token_id);
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

function finitePosition(position: GraphNodePosition | undefined): position is GraphNodePosition {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
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
  activeExecutionsByWorkstationNodeID: Record<string, DashboardActiveExecution[]>;
  activeGraphHighlights: ActiveGraphHighlights;
  activeItemLabelsByPlaceId: Map<string, string[]>;
  graphLayout: GraphLayout;
  handleAssignments: HandleAssignments;
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  storedNodePositions: GraphNodePositions;
}

export function buildCurrentActivityNodes({
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
}: BuildCurrentActivityNodesInput): CurrentActivityNode[] {
  const nextNodes: CurrentActivityNode[] = [];

  for (const positionedNode of graphLayout.nodes) {
    if (positionedNode.nodeKind !== "workstation") {
      const placeNode = positionedNode as PositionedPlaceNode;
      const place = placeNode.place;
      const position = nodePosition(
        placeNode.nodeId,
        { x: placeNode.x, y: placeNode.y },
        storedNodePositions,
      );
      const basePlaceNode = {
        className: "border-0 bg-transparent p-0 text-af-ink",
        draggable: true,
        height: placeNode.height,
        id: placeNode.nodeId,
        initialHeight: placeNode.height,
        initialWidth: placeNode.width,
        measured: { height: placeNode.height, width: placeNode.width },
        position,
        width: placeNode.width,
      };
      const basePlaceData = {
        activeFlow: activeGraphHighlights.activePlaceNodeIds.has(placeNode.nodeId),
        activeItemLabels: activeItemLabelsByPlaceId.get(place.place_id) ?? [],
        incomingHandleCount: handleAssignments.incomingHandleCounts.get(placeNode.nodeId) ?? 1,
        muted:
          place.kind !== "resource" &&
          activeGraphHighlights.hasActiveFlow &&
          !activeGraphHighlights.relatedNodeIds.has(placeNode.nodeId),
        outgoingHandleCount: handleAssignments.outgoingHandleCounts.get(placeNode.nodeId) ?? 1,
        selectedStateNode:
          selection?.kind === "state-node" &&
          selection.placeId === place.place_id,
        tokenCount: snapshot.runtime.place_token_counts?.[place.place_id] ?? 0,
      };

      if (place.kind === "work_state") {
        nextNodes.push({
          ...basePlaceNode,
          data: { ...basePlaceData, onSelectStateNode, place },
          selectable: true,
          type: "statePosition",
        });
        continue;
      }

      if (place.kind === "resource") {
        nextNodes.push({
          ...basePlaceNode,
          data: { ...basePlaceData, place },
          selectable: false,
          type: "resource",
        });
        continue;
      }

      nextNodes.push({
        ...basePlaceNode,
        data: { ...basePlaceData, place },
        selectable: false,
        type: "constraint",
      });
      continue;
    }

    const workstationNode = positionedNode as PositionedWorkstationNode;
    const workstation = snapshot.topology.workstation_nodes_by_id[workstationNode.workstationNodeId];
    if (!workstation) {
      continue;
    }

    const executions = activeExecutionsByWorkstationNodeID[workstation.node_id] ?? [];
    const position = nodePosition(
      workstationNode.nodeId,
      { x: workstationNode.x, y: workstationNode.y },
      storedNodePositions,
    );

    nextNodes.push({
      className: "border-0 bg-transparent p-0 text-af-ink",
      data: {
        active: executions.length > 0,
        activeFlow: activeGraphHighlights.activeWorkstationNodeIds.has(workstationNode.nodeId),
        executions,
        incomingHandleCount: handleAssignments.incomingHandleCounts.get(workstationNode.nodeId) ?? 1,
        muted:
          activeGraphHighlights.hasActiveFlow &&
          !activeGraphHighlights.relatedNodeIds.has(workstationNode.nodeId),
        now,
        onSelectWorkItem,
        onSelectWorkstation,
        outgoingHandleCount: handleAssignments.outgoingHandleCounts.get(workstationNode.nodeId) ?? 1,
        selectedWorkID:
          selection?.kind === "work-item" && selection.nodeId === workstation.node_id
            ? selection.workID
            : null,
        selectedWorkstation:
          selection?.kind === "node" && selection.nodeId === workstation.node_id,
        workstation,
      },
      draggable: true,
      height: workstationNode.height,
      id: workstationNode.nodeId,
      initialHeight: workstationNode.height,
      initialWidth: workstationNode.width,
      measured: { height: workstationNode.height, width: workstationNode.width },
      position,
      selectable: true,
      type: "workstation",
      width: workstationNode.width,
    });
  }

  return nextNodes;
}

export function buildGraphEdges(
  activeGraphHighlights: ActiveGraphHighlights,
  handleAssignments: HandleAssignments,
  visibleGraphEdges: PositionedEdge[],
): Edge[] {
  return visibleGraphEdges.map((edge) => {
    const activeFlow = activeGraphHighlights.activeEdgeIds.has(edge.edgeId);
    const semantic = edgeSemantic(edge);
    const muted =
      activeGraphHighlights.hasActiveFlow &&
      !activeFlow &&
      !semantic &&
      (!activeGraphHighlights.relatedNodeIds.has(edge.fromNodeId) ||
        !activeGraphHighlights.relatedNodeIds.has(edge.toNodeId));

    return {
      animated: activeFlow,
      className: [
        activeFlow ? "agent-flow-edge--active" : "",
        semantic ? "agent-flow-edge--semantic" : "",
        muted ? "agent-flow-edge--muted" : "",
      ].filter(Boolean).join(" "),
      id: edge.edgeId,
      label: edgeLabel(edge, activeFlow),
      labelBgStyle: {
        fill: "var(--color-af-surface)",
        fillOpacity: activeFlow || semantic ? 0.92 : 0,
      },
      labelStyle: { fill: "var(--color-af-ink)" },
      markerEnd: {
        color: edgeMarkerColor(edge, activeFlow),
        type: MarkerType.ArrowClosed,
      },
      source: edge.fromNodeId,
      sourceHandle: handleAssignments.sourceHandlesByEdgeId.get(edge.edgeId),
      style: edgeStyle(edge, activeFlow, muted),
      target: edge.toNodeId,
      targetHandle: handleAssignments.targetHandlesByEdgeId.get(edge.edgeId),
      type: "default",
    };
  });
}

export function initialFocusNodes(graphLayout: GraphLayout): FitViewOptions["nodes"] | undefined {
  const initialPlace = graphLayout.nodes
    .filter(
      (node): node is PositionedPlaceNode =>
        node.nodeKind !== "workstation" && node.place.state_category === "INITIAL",
    )
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  if (!initialPlace) {
    return undefined;
  }

  const firstConnectedWorkstation = graphLayout.edges
    .filter((edge) => edge.fromNodeId === initialPlace.nodeId)
    .map((edge) => graphLayout.nodes.find((node) => node.nodeId === edge.toNodeId))
    .filter((node): node is PositionedWorkstationNode => node?.nodeKind === "workstation")
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  return [initialPlace, firstConnectedWorkstation]
    .filter((node): node is PositionedPlaceNode | PositionedWorkstationNode => node !== undefined)
    .map((node) => ({ id: node.nodeId }));
}

