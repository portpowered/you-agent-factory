import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import { workstationSupportsProgressOutcomeRoutes } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import { workstationRequiresWorkerAssignment } from "../../current-factory-definition/lib/workstation-worker-assignment";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import type {
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "./factory-graph-draft-types";
import { edgeChangeId } from "./factory-graph-draft-types";
import { PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS } from "./factory-graph-progress-outcome-connection-anchors";

export {
  mergeAuthoredProgressOutcomeConnectionAnchors,
  PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS,
} from "./factory-graph-progress-outcome-connection-anchors";

export interface FactoryGraphConnectionAnchor {
  description: string;
  edgeKind: FactoryGraphDraftEdgeChange["kind"];
  edgeKinds?: readonly FactoryGraphDraftEdgeChange["kind"][];
  id: string;
  label: string;
  role: "source" | "target";
  side: "left" | "right";
}

export interface FactoryGraphConnectionEndpoint {
  anchorId: string;
  nodeId: string;
}

export interface FactoryGraphConnectionAnchorContext {
  workstation: WorkstationProgressOutcomeRouteContext;
}

export interface FactoryGraphConnectionResolver {
  resolveWorkstation?: (
    workstationName: string,
  ) => WorkstationProgressOutcomeRouteContext | undefined;
}

const ANCHORS_BY_KIND: Record<
  FactoryGraphNodeKind,
  FactoryGraphConnectionAnchor[]
> = {
  resource: [
    {
      description: "",
      edgeKind: "worker-resource",
      id: "worker-resource-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-resource",
      id: "workstation-resource-source",
      label: "",
      role: "source",
      side: "right",
    },
  ],
  worker: [
    {
      description: "",
      edgeKind: "worker-resource",
      id: "worker-input-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "worker-assignment",
      id: "worker-assignment-source",
      label: "",
      role: "source",
      side: "right",
    },
  ],
  "work-state": [
    {
      description: "",
      edgeKind: "workstation-input",
      id: "workstation-input-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-output",
      edgeKinds: [
        "workstation-output",
        "workstation-on-continue",
        "workstation-on-failure",
        "workstation-on-rejection",
      ],
      id: "work-state-input-target",
      label: "",
      role: "target",
      side: "left",
    },
  ],
  "work-type": [],
  workstation: [
    {
      description: "",
      edgeKind: "workstation-input",
      id: "workstation-input-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "worker-assignment",
      id: "worker-assignment-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "workstation-resource",
      id: "workstation-resource-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "workstation-output",
      id: "workstation-output-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-on-continue",
      id: "workstation-on-continue-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-on-failure",
      id: "workstation-on-failure-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-on-rejection",
      id: "workstation-on-rejection-source",
      label: "",
      role: "source",
      side: "right",
    },
  ],
};

export function factoryGraphConnectionAnchorContext(
  workstation: WorkstationProgressOutcomeRouteContext,
): FactoryGraphConnectionAnchorContext {
  return { workstation };
}

export function createFactoryGraphWorkstationResolver(
  workstations: readonly FactoryWorkstation[] | undefined,
): FactoryGraphConnectionResolver {
  const byName = new Map(
    (workstations ?? []).map((workstation) => [workstation.name, workstation]),
  );

  return {
    resolveWorkstation: (workstationName) => byName.get(workstationName),
  };
}

function filterWorkstationConnectionAnchors(
  anchors: FactoryGraphConnectionAnchor[],
  context?: FactoryGraphConnectionAnchorContext,
) {
  const filtered =
    context && !workstationRequiresWorkerAssignment(context.workstation)
      ? anchors.filter((anchor) => anchor.id !== "worker-assignment-target")
      : anchors;
  if (
    !context ||
    workstationSupportsProgressOutcomeRoutes(context.workstation)
  ) {
    return filtered;
  }
  return filtered.filter(
    (anchor) => !PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(anchor.id),
  );
}

export function getFactoryGraphConnectionAnchors(
  kind: FactoryGraphNodeKind,
  context?: FactoryGraphConnectionAnchorContext,
) {
  const anchors = ANCHORS_BY_KIND[kind];
  if (kind !== "workstation") {
    return anchors;
  }

  return filterWorkstationConnectionAnchors(anchors, context);
}

export function getLocalizedFactoryGraphConnectionAnchors(
  kind: FactoryGraphNodeKind,
  locale?: string | null,
  context?: FactoryGraphConnectionAnchorContext,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  return getFactoryGraphConnectionAnchors(kind, context).map((anchor) => ({
    ...anchor,
    description: messages.connectionAnchorDescription(anchor.id),
    label: messages.connectionAnchorLabel(anchor.id),
  }));
}

export function getFactoryGraphConnectionAnchor(
  kind: FactoryGraphNodeKind,
  anchorId: string,
  context?: FactoryGraphConnectionAnchorContext,
) {
  return (
    getFactoryGraphConnectionAnchors(kind, context).find(
      (anchor) => anchor.id === anchorId,
    ) ?? null
  );
}

export function resolveFactoryGraphConnectionAnchorContext(
  node: FactoryGraphNode,
  resolver?: FactoryGraphConnectionResolver,
): FactoryGraphConnectionAnchorContext | undefined {
  if (node.kind !== "workstation" || node.key.kind !== "workstation") {
    return undefined;
  }

  const workstation = resolver?.resolveWorkstation?.(node.key.name);
  return workstation
    ? factoryGraphConnectionAnchorContext(workstation)
    : undefined;
}

export function buildFactoryGraphEdgeChangeFromConnection(
  topology: FactoryGraphTopology,
  endpoint: {
    sourceAnchorId: string;
    sourceNodeId: string;
    targetAnchorId: string;
    targetNodeId: string;
  },
  resolver?: FactoryGraphConnectionResolver,
): FactoryGraphDraftEdgeChange | null {
  const sourceNode = findNode(topology, endpoint.sourceNodeId);
  const targetNode = findNode(topology, endpoint.targetNodeId);
  if (!sourceNode || !targetNode) {
    return null;
  }

  const sourceContext = resolveFactoryGraphConnectionAnchorContext(
    sourceNode,
    resolver,
  );
  const targetContext = resolveFactoryGraphConnectionAnchorContext(
    targetNode,
    resolver,
  );
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    sourceNode.kind,
    endpoint.sourceAnchorId,
    sourceContext,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    targetNode.kind,
    endpoint.targetAnchorId,
    targetContext,
  );
  if (!sourceAnchor || !targetAnchor) {
    return null;
  }
  if (!isCompatibleFactoryGraphConnectionAnchors(sourceAnchor, targetAnchor)) {
    return null;
  }

  return {
    kind: sourceAnchor.edgeKind,
    source: sourceNode.key,
    target: targetNode.key,
  };
}

export function isValidFactoryGraphConnection(input: {
  sourceAnchorId: string;
  sourceNodeKind: FactoryGraphNodeKind;
  targetAnchorId: string;
  targetNodeKind: FactoryGraphNodeKind;
  sourceWorkstation?: WorkstationProgressOutcomeRouteContext;
  targetWorkstation?: WorkstationProgressOutcomeRouteContext;
}) {
  const sourceContext = input.sourceWorkstation
    ? factoryGraphConnectionAnchorContext(input.sourceWorkstation)
    : undefined;
  const targetContext = input.targetWorkstation
    ? factoryGraphConnectionAnchorContext(input.targetWorkstation)
    : undefined;
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    input.sourceNodeKind,
    input.sourceAnchorId,
    sourceContext,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    input.targetNodeKind,
    input.targetAnchorId,
    targetContext,
  );
  if (!sourceAnchor || !targetAnchor) {
    return false;
  }

  return isCompatibleFactoryGraphConnectionAnchors(sourceAnchor, targetAnchor);
}

export function isCompatibleFactoryGraphConnectionAnchors(
  sourceAnchor: FactoryGraphConnectionAnchor,
  targetAnchor: FactoryGraphConnectionAnchor,
) {
  return (
    sourceAnchor.role === "source" &&
    targetAnchor.role === "target" &&
    anchorSupportsEdgeKind(targetAnchor, sourceAnchor.edgeKind)
  );
}

function anchorSupportsEdgeKind(
  anchor: FactoryGraphConnectionAnchor,
  edgeKind: FactoryGraphDraftEdgeChange["kind"],
) {
  return (anchor.edgeKinds ?? [anchor.edgeKind]).includes(edgeKind);
}

export function applyFactoryGraphEdgeAddition(
  currentDraft: FactoryGraphDraft,
  currentTopology: FactoryGraphTopology,
  edgeChange: FactoryGraphDraftEdgeChange,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);
  const nextEdgeId = edgeChangeId(edgeChange);
  const removalIndex = nextDraft.edgeChanges.removals.findIndex(
    (entry) => edgeChangeId(entry) === nextEdgeId,
  );
  if (removalIndex >= 0) {
    nextDraft.edgeChanges.removals.splice(removalIndex, 1);
    return nextDraft;
  }

  if (currentTopology.edges.some((edge) => edge.id === nextEdgeId)) {
    return nextDraft;
  }

  if (edgeChange.kind === "worker-assignment") {
    nextDraft.edgeChanges.additions = nextDraft.edgeChanges.additions.filter(
      (entry) =>
        !(
          entry.kind === "worker-assignment" &&
          entry.target.kind === "workstation" &&
          edgeChange.target.kind === "workstation" &&
          entry.target.name === edgeChange.target.name
        ),
    );
  }

  nextDraft.edgeChanges.additions = appendUniqueEdgeChange(
    nextDraft.edgeChanges.additions,
    edgeChange,
  );
  return nextDraft;
}

export function applyFactoryGraphEdgeRemoval(
  currentDraft: FactoryGraphDraft,
  currentTopology: FactoryGraphTopology,
  edge: FactoryGraphEdge,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);
  if (edge.kind === "work-type-state") {
    return nextDraft;
  }
  const nextEdgeId = edge.id;

  const hadPendingAddition = nextDraft.edgeChanges.additions.some(
    (entry) => edgeChangeId(entry) === nextEdgeId,
  );
  nextDraft.edgeChanges.additions = nextDraft.edgeChanges.additions.filter(
    (entry) => edgeChangeId(entry) !== nextEdgeId,
  );
  if (hadPendingAddition) {
    return nextDraft;
  }
  if (!currentTopology.edges.some((entry) => entry.id === nextEdgeId)) {
    return nextDraft;
  }

  nextDraft.edgeChanges.removals = appendUniqueEdgeChange(
    nextDraft.edgeChanges.removals,
    {
      kind: edge.kind,
      source: edge.source,
      target: edge.target,
    },
  );
  return nextDraft;
}

export function buildFactoryGraphConnectionNotice(options: {
  locale?: string | null;
  resolver?: FactoryGraphConnectionResolver;
  sourceAnchorId: string;
  sourceNode: FactoryGraphNode;
  targetAnchorId: string;
  targetNode: FactoryGraphNode;
}) {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const sourceContext = resolveFactoryGraphConnectionAnchorContext(
    options.sourceNode,
    options.resolver,
  );
  const targetContext = resolveFactoryGraphConnectionAnchorContext(
    options.targetNode,
    options.resolver,
  );
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    options.sourceNode.kind,
    options.sourceAnchorId,
    sourceContext,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    options.targetNode.kind,
    options.targetAnchorId,
    targetContext,
  );
  if (!sourceAnchor || !targetAnchor) {
    return messages.connectionFallbackNotice;
  }

  return messages.connectionIncompatibleNotice(
    messages.connectionAnchorLabel(sourceAnchor.id),
    options.sourceNode.label,
    messages.connectionAnchorLabel(targetAnchor.id),
    options.targetNode.label,
  );
}

function appendUniqueEdgeChange(
  edges: FactoryGraphDraftEdgeChange[],
  edgeChange: FactoryGraphDraftEdgeChange,
) {
  const nextEdgeId = edgeChangeId(edgeChange);
  if (edges.some((entry) => edgeChangeId(entry) === nextEdgeId)) {
    return edges;
  }
  return [...edges, edgeChange];
}

function findNode(topology: FactoryGraphTopology, nodeId: string) {
  return topology.nodes.find((node) => node.id === nodeId) ?? null;
}
