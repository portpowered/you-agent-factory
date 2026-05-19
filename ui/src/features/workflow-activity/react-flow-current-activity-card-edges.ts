import { type Edge, type FitViewOptions, MarkerType } from "@xyflow/react";

import type {
  GraphLayout,
  PositionedEdge,
  PositionedPlaceNode,
  PositionedWorkstationNode,
} from "../flowchart/layout";
import type { ActiveGraphHighlights, HandleAssignments } from "./react-flow-current-activity-card-graph";

const EDGE_STROKE_MUTED = "var(--color-af-edge-muted)";
const EDGE_STROKE_SOFT = "var(--color-af-edge-muted-soft)";
const EDGE_STROKE_DANGER_MUTED = "var(--color-af-edge-danger-muted)";
const EDGE_STROKE_ACTIVE = "var(--color-af-success)";

function edgeIsFailure(edge: PositionedEdge): boolean {
  return edge.outcomeKind === "failed" || edge.stateCategory === "FAILED";
}

function edgeStyle(
  edge: PositionedEdge,
  activeFlow: boolean,
  muted: boolean,
): Edge["style"] {
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
  return edge.sourcePlaceKind === "resource"
    ? EDGE_STROKE_SOFT
    : EDGE_STROKE_MUTED;
}

function edgeSemantic(edge: PositionedEdge): boolean {
  return edge.outcomeKind !== "accepted" || edgeIsFailure(edge);
}

function edgeLabel(
  edge: PositionedEdge,
  activeFlow: boolean,
): string | undefined {
  return activeFlow ? edge.label || undefined : undefined;
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
      ]
        .filter(Boolean)
        .join(" "),
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

export function initialFocusNodes(
  graphLayout: GraphLayout,
): FitViewOptions["nodes"] | undefined {
  const initialPlace = graphLayout.nodes
    .filter(
      (node): node is PositionedPlaceNode =>
        node.nodeKind !== "workstation" &&
        node.place.state_category === "INITIAL",
    )
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  if (!initialPlace) {
    return undefined;
  }

  const firstConnectedWorkstation = graphLayout.edges
    .filter((edge) => edge.fromNodeId === initialPlace.nodeId)
    .map((edge) =>
      graphLayout.nodes.find((node) => node.nodeId === edge.toNodeId),
    )
    .filter(
      (node): node is PositionedWorkstationNode =>
        node?.nodeKind === "workstation",
    )
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  return [initialPlace, firstConnectedWorkstation]
    .filter(
      (node): node is PositionedPlaceNode | PositionedWorkstationNode =>
        node !== undefined,
    )
    .map((node) => ({ id: node.nodeId }));
}
