import type { DashboardEdgeOutcomeKind } from "../../../api/dashboard/types";
import type { GraphLayout, PositionedEdge } from "../../flowchart/lib/layout";
import type {
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphNodeKey,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { buildVisibleGraphEdges } from "./react-flow-current-activity-card-graph";

function currentActivityEdgeNodeId(key: FactoryGraphNodeKey): string {
  if (key.kind === "workstation") {
    return `workstation:${key.name}`;
  }

  if (key.kind !== "work-state") {
    return `place:${key.name}`;
  }

  return `place:${key.workTypeName}:${key.stateName}`;
}

function positionedEdgeOutcomeKind(
  kind: FactoryGraphDraftEdgeChange["kind"],
): DashboardEdgeOutcomeKind {
  switch (kind) {
    case "workstation-on-continue":
      return "continue";
    case "workstation-on-failure":
      return "failed";
    case "workstation-on-rejection":
      return "rejected";
    case "workstation-input":
    case "workstation-output":
      return "accepted";
    /* c8 ignore next: defensive fallback for future edge kinds outside the current typed union. */
    default:
      return "accepted";
  }
}

function supportedCurrentActivityDraftEdge(
  edgeChange: FactoryGraphDraftEdgeChange,
  graphLayout: GraphLayout,
): PositionedEdge | null {
  if (
    edgeChange.kind !== "workstation-input" &&
    edgeChange.kind !== "workstation-output" &&
    edgeChange.kind !== "workstation-on-continue" &&
    edgeChange.kind !== "workstation-on-failure" &&
    edgeChange.kind !== "workstation-on-rejection"
  ) {
    return null;
  }

  const sourceNodeId = currentActivityEdgeNodeId(edgeChange.source);
  const targetNodeId = currentActivityEdgeNodeId(edgeChange.target);
  const hasSourceNode = graphLayout.nodes.some((node) => node.nodeId === sourceNodeId);
  const hasTargetNode = graphLayout.nodes.some((node) => node.nodeId === targetNodeId);
  if (!hasSourceNode || !hasTargetNode) {
    return null;
  }

  return {
    edgeId: `${edgeChange.kind}:${sourceNodeId}->${targetNodeId}`,
    fromNodeId: sourceNodeId,
    label:
      edgeChange.target.kind === "work-state"
        ? `${edgeChange.target.workTypeName}:${edgeChange.target.stateName}`
        : "",
    labelX: 0,
    labelY: 0,
    outcomeKind: positionedEdgeOutcomeKind(edgeChange.kind),
    path: "",
    sourcePlaceKind:
      edgeChange.source.kind === "work-state" ? "work_state" : undefined,
    stateCategory:
      edgeChange.kind === "workstation-on-failure" ? "FAILED" : undefined,
    targetPlaceKind:
      edgeChange.target.kind === "work-state" ? "work_state" : undefined,
    toNodeId: targetNodeId,
  };
}

export function buildVisibleGraphEdgesWithDraft(options: {
  draft: FactoryGraphDraft;
  graphLayout: GraphLayout;
}): {
  pendingAdditionEdgeIds: ReadonlySet<string>;
  visibleGraphEdges: PositionedEdge[];
} {
  const baseEdges = buildVisibleGraphEdges(options.graphLayout);
  const edgeIdsToRemove = new Set(
    options.draft.edgeChanges.removals
      .map((edgeChange) =>
        supportedCurrentActivityDraftEdge(edgeChange, options.graphLayout),
      )
      .filter((edge): edge is PositionedEdge => edge !== null)
      .map((edge) => edge.edgeId),
  );
  const pendingAdditionEdges = options.draft.edgeChanges.additions
    .map((edgeChange) =>
      supportedCurrentActivityDraftEdge(edgeChange, options.graphLayout),
    )
    .filter((edge): edge is PositionedEdge => edge !== null);
  const pendingAdditionEdgeIds = new Set(
    pendingAdditionEdges.map((edge) => edge.edgeId),
  );
  const visibleGraphEdges = [
    ...baseEdges.filter((edge) => !edgeIdsToRemove.has(edge.edgeId)),
    ...pendingAdditionEdges.filter(
      (edge) => !baseEdges.some((existingEdge) => existingEdge.edgeId === edge.edgeId),
    ),
  ];

  return {
    pendingAdditionEdgeIds,
    visibleGraphEdges,
  };
}
