import type {
  FactoryGraphEdgeKind,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { workstationSupportsProgressOutcomeRoutes } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  type FactoryGraphConnectionAnchorContext,
  type FactoryGraphConnectionEndpoint,
  factoryGraphConnectionAnchorContext,
  getFactoryGraphConnectionAnchors,
  getLocalizedFactoryGraphConnectionAnchors,
  mergeAuthoredProgressOutcomeConnectionAnchors,
  PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS,
} from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import type { ActivityGraphNodeHandle } from "../../flowchart/components/current-activity-node-shell";
import type {
  PositionedEdge,
  PositionedNode,
} from "../../flowchart/lib/layout";

export interface CurrentActivityEditorState {
  activeTool: "add" | "connect" | "delete" | null;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  onConnectionAnchorClick: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
}

type CurrentActivityEndpointKind = Extract<
  FactoryGraphNodeKind,
  "resource" | "worker" | "work-state" | "workstation"
>;

export function supportedSemanticHandleIdsForEdge(
  edge: PositionedEdge,
  nodeKindsById?: ReadonlyMap<string, PositionedNode["nodeKind"]>,
) {
  const edgeKind = edgeKindForPositionedEdge(edge);
  const sourceKind = endpointNodeKind(
    edge.fromNodeId,
    edge.sourcePlaceKind,
    nodeKindsById,
  );
  const targetKind = endpointNodeKind(
    edge.toNodeId,
    edge.targetPlaceKind,
    nodeKindsById,
  );
  if (!edgeKind || !sourceKind || !targetKind) {
    return null;
  }

  const sourceHandleId = connectionAnchorId(sourceKind, edgeKind, "source");
  const targetHandleId = connectionAnchorId(targetKind, edgeKind, "target");
  if (!sourceHandleId || !targetHandleId) {
    return null;
  }

  return {
    sourceHandleId,
    targetHandleId,
  };
}

export const supportedEditorHandleIdsForEdge =
  supportedSemanticHandleIdsForEdge;

export function authoredProgressOutcomeSourceHandlesByWorkstationNodeId(
  edges: readonly PositionedEdge[],
  nodes: readonly PositionedNode[],
): ReadonlyMap<string, ReadonlySet<string>> {
  const nodeKindsById = new Map(
    nodes.map((node) => [node.nodeId, node.nodeKind]),
  );
  const handlesByWorkstationNodeId = new Map<string, Set<string>>();

  for (const edge of edges) {
    const supportedHandles = supportedSemanticHandleIdsForEdge(
      edge,
      nodeKindsById,
    );
    if (
      !supportedHandles ||
      !PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(supportedHandles.sourceHandleId)
    ) {
      continue;
    }

    if (!edge.fromNodeId.startsWith("workstation:")) {
      continue;
    }

    const handleIds =
      handlesByWorkstationNodeId.get(edge.fromNodeId) ?? new Set<string>();
    handleIds.add(supportedHandles.sourceHandleId);
    handlesByWorkstationNodeId.set(edge.fromNodeId, handleIds);
  }

  return handlesByWorkstationNodeId;
}

export function resolveWorkstationConnectionAnchorContext(
  factory: CanonicalFactoryDefinition | undefined,
  factoryGraphNodeId: string,
): FactoryGraphConnectionAnchorContext | undefined {
  const workstationName = factoryGraphNodeId.startsWith("workstation:")
    ? factoryGraphNodeId.slice("workstation:".length)
    : factoryGraphNodeId;
  const workstation = (factory?.workstations ?? []).find(
    (candidate) =>
      candidate.name === workstationName || candidate.id === workstationName,
  );

  return workstation
    ? factoryGraphConnectionAnchorContext(workstation)
    : undefined;
}

export function buildSemanticGraphHandles(args: {
  authoredProgressOutcomeSourceHandleIds?: ReadonlySet<string>;
  connectionAnchorContext?: FactoryGraphConnectionAnchorContext;
  editor?: CurrentActivityEditorState;
  locale?: string | null;
  nodeId: string;
  nodeKind: FactoryGraphNodeKind;
}) {
  const connectable =
    args.editor?.editorMode === true &&
    args.editor.canInteractWithEditor &&
    args.editor.activeTool === "connect";

  const supportsProgressOutcomeRoutes =
    args.nodeKind !== "workstation" ||
    !args.connectionAnchorContext ||
    workstationSupportsProgressOutcomeRoutes(
      args.connectionAnchorContext.workstation,
    );

  let anchors = getLocalizedFactoryGraphConnectionAnchors(
    args.nodeKind,
    args.locale,
    args.nodeKind === "workstation" ? args.connectionAnchorContext : undefined,
  );
  if (args.nodeKind === "workstation") {
    anchors = mergeAuthoredProgressOutcomeConnectionAnchors(
      anchors,
      args.authoredProgressOutcomeSourceHandleIds,
    );
  }

  const handles: ActivityGraphNodeHandle[] = anchors.map((anchor) => {
    const isAuthoredOnlyProgressOutcomeHandle =
      args.nodeKind === "workstation" &&
      !supportsProgressOutcomeRoutes &&
      PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(anchor.id);
    const selected =
      args.editor?.pendingConnectionSource?.nodeId === args.nodeId &&
      args.editor.pendingConnectionSource.anchorId === anchor.id;
    const validTarget =
      connectable &&
      !isAuthoredOnlyProgressOutcomeHandle &&
      args.editor?.pendingConnectionSource !== null &&
      args.editor?.pendingConnectionSource?.nodeId !== args.nodeId &&
      anchor.role === "target";
    const visible = args.editor?.editorMode === true;

    return {
      buttonAriaLabel: anchor.description,
      buttonPressed: selected || undefined,
      buttonDisabled:
        !visible || isAuthoredOnlyProgressOutcomeHandle || undefined,
      buttonTitle: anchor.description,
      connectable: connectable && !isAuthoredOnlyProgressOutcomeHandle,
      hidden:
        !visible || isAuthoredOnlyProgressOutcomeHandle || undefined,
      id: anchor.id,
      label: anchor.label,
      onButtonClick: () =>
        args.editor?.onConnectionAnchorClick({
          anchorId: anchor.id,
          nodeId: args.nodeId,
        }),
      side: anchor.side,
      type: anchor.role,
      variant: selected ? "selected" : validTarget ? "valid-target" : "default",
    } satisfies ActivityGraphNodeHandle;
  });

  return handles;
}

export const buildEditorHandles = buildSemanticGraphHandles;

function endpointNodeKind(
  nodeId: string,
  placeKind:
    | PositionedEdge["sourcePlaceKind"]
    | PositionedEdge["targetPlaceKind"],
  nodeKindsById?: ReadonlyMap<string, PositionedNode["nodeKind"]>,
): CurrentActivityEndpointKind | null {
  const nodeKind = nodeKindsById?.get(nodeId);
  if (nodeKind === "workstation") {
    return "workstation";
  }
  if (nodeKind === "resource") {
    return "resource";
  }
  if (nodeKind === "state_position") {
    return "work-state";
  }
  if (nodeKind === "constraint" && nodeId.startsWith("place:worker:")) {
    return "worker";
  }

  if (nodeId.startsWith("workstation:")) {
    return "workstation";
  }
  if (nodeId.startsWith("resource:")) {
    return "resource";
  }
  if (nodeId.startsWith("worker:")) {
    return "worker";
  }
  if (nodeId.startsWith("work-state:")) {
    return "work-state";
  }
  if (placeKind === "resource") {
    return "resource";
  }
  if (placeKind === "work_state") {
    return "work-state";
  }
  if (nodeId.startsWith("place:worker:")) {
    return "worker";
  }
  if (nodeId.startsWith("place:") && !nodeId.endsWith(":available")) {
    return "work-state";
  }
  return null;
}

function edgeKindForPositionedEdge(
  edge: PositionedEdge,
): FactoryGraphEdgeKind | null {
  const edgeKind = edge.edgeId.split(":")[0];
  if (isFactoryGraphEdgeKind(edgeKind)) {
    return edgeKind;
  }

  const sourceKind = endpointNodeKind(
    edge.fromNodeId,
    edge.sourcePlaceKind,
    undefined,
  );
  const targetKind = endpointNodeKind(
    edge.toNodeId,
    edge.targetPlaceKind,
    undefined,
  );

  if (sourceKind === "resource" && targetKind === "worker") {
    return "worker-resource";
  }
  if (sourceKind === "worker" && targetKind === "workstation") {
    return "worker-assignment";
  }
  if (sourceKind === "resource" && targetKind === "workstation") {
    return "workstation-resource";
  }
  if (sourceKind === "work-state" && targetKind === "workstation") {
    return "workstation-input";
  }
  if (sourceKind === "workstation" && targetKind === "work-state") {
    if (edge.outcomeKind === "failed" || edge.stateCategory === "FAILED") {
      return "workstation-on-failure";
    }
    if (edge.outcomeKind === "rejected") {
      return "workstation-on-rejection";
    }
    if (edge.outcomeKind === "continue") {
      return "workstation-on-continue";
    }
    return "workstation-output";
  }

  return null;
}

function isFactoryGraphEdgeKind(value: string): value is FactoryGraphEdgeKind {
  return (
    value === "worker-assignment" ||
    value === "worker-resource" ||
    value === "workstation-input" ||
    value === "workstation-on-continue" ||
    value === "workstation-on-failure" ||
    value === "workstation-on-rejection" ||
    value === "workstation-output" ||
    value === "workstation-resource" ||
    value === "work-type-state"
  );
}

function connectionAnchorId(
  nodeKind: CurrentActivityEndpointKind,
  edgeKind: FactoryGraphEdgeKind,
  role: "source" | "target",
) {
  if (edgeKind === "work-type-state") {
    return null;
  }

  return (
    getFactoryGraphConnectionAnchors(nodeKind).find(
      (anchor) =>
        anchor.role === role &&
        (anchor.edgeKinds ?? [anchor.edgeKind]).includes(edgeKind),
    )?.id ?? null
  );
}
