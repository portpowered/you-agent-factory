import type {
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import { edgeChangeId } from "./factory-graph-draft-types";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export interface FactoryGraphConnectionAnchor {
  description: string;
  edgeKind: FactoryGraphDraftEdgeChange["kind"];
  id: string;
  label: string;
  role: "source" | "target";
  side: "left" | "right";
}

export interface FactoryGraphConnectionEndpoint {
  anchorId: string;
  nodeId: string;
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
      id: "worker-resource-target",
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
      id: "workstation-output-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "workstation-on-continue",
      id: "workstation-on-continue-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "workstation-on-failure",
      id: "workstation-on-failure-target",
      label: "",
      role: "target",
      side: "left",
    },
    {
      description: "",
      edgeKind: "workstation-on-rejection",
      id: "workstation-on-rejection-target",
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

export function getFactoryGraphConnectionAnchors(kind: FactoryGraphNodeKind) {
  return ANCHORS_BY_KIND[kind];
}

export function getLocalizedFactoryGraphConnectionAnchors(
  kind: FactoryGraphNodeKind,
  locale?: string | null,
) {
  const messages = getFactoryGraphEditorMessages(locale);
  return ANCHORS_BY_KIND[kind].map((anchor) => ({
    ...anchor,
    description: messages.connectionAnchorDescription(anchor.id),
    label: messages.connectionAnchorLabel(anchor.id),
  }));
}

export function getFactoryGraphConnectionAnchor(
  kind: FactoryGraphNodeKind,
  anchorId: string,
) {
  return ANCHORS_BY_KIND[kind].find((anchor) => anchor.id === anchorId) ?? null;
}

export function buildFactoryGraphEdgeChangeFromConnection(
  topology: FactoryGraphTopology,
  endpoint: {
    sourceAnchorId: string;
    sourceNodeId: string;
    targetAnchorId: string;
    targetNodeId: string;
  },
): FactoryGraphDraftEdgeChange | null {
  const sourceNode = findNode(topology, endpoint.sourceNodeId);
  const targetNode = findNode(topology, endpoint.targetNodeId);
  if (!sourceNode || !targetNode) {
    return null;
  }

  const sourceAnchor = getFactoryGraphConnectionAnchor(
    sourceNode.kind,
    endpoint.sourceAnchorId,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    targetNode.kind,
    endpoint.targetAnchorId,
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
}) {
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    input.sourceNodeKind,
    input.sourceAnchorId,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    input.targetNodeKind,
    input.targetAnchorId,
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
    sourceAnchor.edgeKind === targetAnchor.edgeKind
  );
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
  sourceAnchorId: string;
  sourceNode: FactoryGraphNode;
  targetAnchorId: string;
  targetNode: FactoryGraphNode;
}) {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    options.sourceNode.kind,
    options.sourceAnchorId,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    options.targetNode.kind,
    options.targetAnchorId,
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
