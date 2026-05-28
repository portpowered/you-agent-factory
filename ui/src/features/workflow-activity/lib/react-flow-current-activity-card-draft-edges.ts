import type { DashboardEdgeOutcomeKind } from "../../../api/dashboard/types";
import type {
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { GraphLayout, PositionedEdge } from "../../flowchart/lib/layout";
import { currentActivityNodeIdForFactoryGraphKey } from "./current-activity-factory-graph-node-ids";
import { buildVisibleGraphEdges } from "./react-flow-current-activity-card-graph";

function legacyCurrentActivityNodeIdForFactoryGraphKey(
  key: FactoryGraphDraftEdgeChange["source"],
): string {
  if (key.kind === "workstation") {
    return `workstation:${key.name}`;
  }
  if (key.kind === "work-state") {
    return `place:${key.workTypeName}:${key.stateName}`;
  }

  return `place:${key.name}`;
}

function resolveVisibleNodeId(
  graphLayout: GraphLayout,
  key: FactoryGraphDraftEdgeChange["source"],
): string | null {
  const preferredNodeId = currentActivityNodeIdForFactoryGraphKey(key);
  if (graphLayout.nodes.some((node) => node.nodeId === preferredNodeId)) {
    return preferredNodeId;
  }
  if (key.kind === "work-state") {
    const resourceNodeId = `resource:${key.workTypeName}`;
    if (graphLayout.nodes.some((node) => node.nodeId === resourceNodeId)) {
      return resourceNodeId;
    }
  }
  const legacyNodeId = legacyCurrentActivityNodeIdForFactoryGraphKey(key);
  if (graphLayout.nodes.some((node) => node.nodeId === legacyNodeId)) {
    return legacyNodeId;
  }

  return null;
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
    edgeChange.kind !== "worker-assignment" &&
    edgeChange.kind !== "worker-resource" &&
    edgeChange.kind !== "workstation-resource" &&
    edgeChange.kind !== "workstation-input" &&
    edgeChange.kind !== "workstation-output" &&
    edgeChange.kind !== "workstation-on-continue" &&
    edgeChange.kind !== "workstation-on-failure" &&
    edgeChange.kind !== "workstation-on-rejection"
  ) {
    return null;
  }

  const sourceNodeId = resolveVisibleNodeId(graphLayout, edgeChange.source);
  const targetNodeId = resolveVisibleNodeId(graphLayout, edgeChange.target);
  if (!sourceNodeId || !targetNodeId) {
    return null;
  }
  const sourceResourceAlias =
    edgeChange.source.kind === "work-state" &&
    sourceNodeId === `resource:${edgeChange.source.workTypeName}`;
  const edgeKind =
    edgeChange.kind === "workstation-input" && sourceResourceAlias
      ? "workstation-resource"
      : edgeChange.kind;

  return {
    edgeId: `${edgeKind}:${sourceNodeId}->${targetNodeId}`,
    fromNodeId: sourceNodeId,
    label:
      edgeChange.target.kind === "work-state" && !sourceResourceAlias
        ? `${edgeChange.target.workTypeName}:${edgeChange.target.stateName}`
        : "",
    labelX: 0,
    labelY: 0,
    outcomeKind: positionedEdgeOutcomeKind(edgeChange.kind),
    path: "",
    sourcePlaceKind: sourceResourceAlias
      ? "resource"
      : edgeChange.source.kind === "work-state"
        ? "work_state"
        : edgeChange.source.kind === "resource"
          ? "resource"
          : edgeChange.source.kind === "worker" ||
              edgeChange.source.kind === "work-type"
            ? "constraint"
            : undefined,
    stateCategory: edgeKind === "workstation-on-failure" ? "FAILED" : undefined,
    targetPlaceKind:
      edgeChange.target.kind === "work-state"
        ? "work_state"
        : edgeChange.target.kind === "worker" ||
            edgeChange.target.kind === "work-type"
          ? "constraint"
          : undefined,
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
      (edge) =>
        !baseEdges.some((existingEdge) => existingEdge.edgeId === edge.edgeId),
    ),
  ];

  return {
    pendingAdditionEdgeIds,
    visibleGraphEdges,
  };
}
