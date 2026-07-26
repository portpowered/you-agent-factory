import { type Edge, type FitViewOptions, MarkerType } from "@xyflow/react";
import { currentActivityGraphEdgeHoverClassName } from "../../flowchart/lib/current-activity-graph-hover";
import type {
  GraphLayout,
  PositionedEdge,
  PositionedPlaceNode,
  PositionedWorkstationNode,
} from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/components/current-activity-nodes";
import {
  type ActiveGraphHighlights,
  filterGraphEdgesForRenderedHandles,
  type HandleAssignments,
} from "./react-flow-current-activity-card-graph";

const EDGE_STROKE_MUTED = "var(--color-outline-variant)";
const EDGE_STROKE_SOFT = "var(--color-outline)";
const EDGE_STROKE_DANGER_MUTED = "var(--color-af-edge-danger-muted)";
const EDGE_STROKE_ACTIVE = "var(--color-success)";

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

function withHoverableEdgeStroke(style: Edge["style"]): Edge["style"] {
  const stroke = style?.stroke;
  if (typeof stroke !== "string") {
    return style;
  }

  return {
    ...style,
    stroke: "var(--af-graph-edge-stroke)",
    "--af-graph-edge-stroke": stroke,
  } as Edge["style"];
}

export function buildGraphEdges(
  activeGraphHighlights: ActiveGraphHighlights,
  handleAssignments: HandleAssignments,
  pendingAdditionEdgeIds: ReadonlySet<string>,
  visibleGraphEdges: PositionedEdge[],
  renderedNodes?: ReadonlyArray<Pick<CurrentActivityNode, "data" | "id">>,
): Edge[] {
  const edgesToRender =
    renderedNodes === undefined
      ? visibleGraphEdges
      : filterGraphEdgesForRenderedHandles(
          visibleGraphEdges,
          handleAssignments,
          renderedNodes,
        );

  return edgesToRender.map((edge) => {
    const activeFlow = activeGraphHighlights.activeEdgeIds.has(edge.edgeId);
    const pendingAddition = pendingAdditionEdgeIds.has(edge.edgeId);
    const semantic = edgeSemantic(edge);
    const muted =
      activeGraphHighlights.hasActiveFlow &&
      !activeFlow &&
      !semantic &&
      (!activeGraphHighlights.relatedNodeIds.has(edge.fromNodeId) ||
        !activeGraphHighlights.relatedNodeIds.has(edge.toNodeId));
    const hoverClassName = currentActivityGraphEdgeHoverClassName({
      activeFlow,
      muted,
      pendingAddition,
      semantic,
    });
    const resolvedStyle = pendingAddition
      ? {
          stroke: "var(--color-on-warning-container)",
          strokeDasharray: "9 4",
          strokeWidth: 2,
        }
      : edgeStyle(edge, activeFlow, muted);
    const style = hoverClassName
      ? withHoverableEdgeStroke(resolvedStyle)
      : resolvedStyle;

    return {
      animated: activeFlow,
      className: [
        activeFlow ? "agent-flow-edge--active" : "",
        semantic ? "agent-flow-edge--semantic" : "",
        muted ? "agent-flow-edge--muted" : "",
        pendingAddition ? "agent-flow-edge--pending-addition" : "",
        hoverClassName ?? "",
      ]
        .filter(Boolean)
        .join(" "),
      id: edge.edgeId,
      data: {
        factoryGraphEdgeId: edge.canonicalEdgeId,
      },
      label: edgeLabel(edge, activeFlow),
      labelBgStyle: {
        fill: "var(--color-surface)",
        fillOpacity: activeFlow || semantic ? 0.92 : 0,
      },
      labelStyle: { fill: "var(--color-on-surface)" },
      markerEnd: {
        color: pendingAddition
          ? "var(--color-on-warning-container)"
          : edgeMarkerColor(edge, activeFlow),
        type: MarkerType.ArrowClosed,
      },
      source: edge.fromNodeId,
      sourceHandle: handleAssignments.sourceHandlesByEdgeId.get(edge.edgeId),
      style,
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
        node.nodeKind !== "doc" &&
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
