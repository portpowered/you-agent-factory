import type {
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import { edgeChangeId } from "./factory-graph-draft-types";

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
      description: "Provide this resource to a worker.",
      edgeKind: "worker-resource",
      id: "worker-resource-source",
      label: "Worker",
      role: "source",
      side: "right",
    },
    {
      description: "Provide this resource to a workstation.",
      edgeKind: "workstation-resource",
      id: "workstation-resource-source",
      label: "Station",
      role: "source",
      side: "right",
    },
  ],
  worker: [
    {
      description: "Accept a resource required by this worker.",
      edgeKind: "worker-resource",
      id: "worker-resource-target",
      label: "Resource",
      role: "target",
      side: "left",
    },
    {
      description: "Assign this worker to a workstation.",
      edgeKind: "worker-assignment",
      id: "worker-assignment-source",
      label: "Assign",
      role: "source",
      side: "right",
    },
  ],
  "work-state": [
    {
      description: "Route this work state into a workstation input.",
      edgeKind: "workstation-input",
      id: "workstation-input-source",
      label: "Input",
      role: "source",
      side: "right",
    },
    {
      description: "Receive a successful workstation output.",
      edgeKind: "workstation-output",
      id: "workstation-output-target",
      label: "Success",
      role: "target",
      side: "left",
    },
    {
      description: "Receive a workstation continue transition.",
      edgeKind: "workstation-on-continue",
      id: "workstation-on-continue-target",
      label: "Continue",
      role: "target",
      side: "left",
    },
    {
      description: "Receive a workstation failure transition.",
      edgeKind: "workstation-on-failure",
      id: "workstation-on-failure-target",
      label: "Failure",
      role: "target",
      side: "left",
    },
    {
      description: "Receive a workstation rejection transition.",
      edgeKind: "workstation-on-rejection",
      id: "workstation-on-rejection-target",
      label: "Reject",
      role: "target",
      side: "left",
    },
  ],
  "work-type": [],
  workstation: [
    {
      description: "Accept an input work state for this workstation.",
      edgeKind: "workstation-input",
      id: "workstation-input-target",
      label: "Input",
      role: "target",
      side: "left",
    },
    {
      description: "Accept a worker assignment for this workstation.",
      edgeKind: "worker-assignment",
      id: "worker-assignment-target",
      label: "Worker",
      role: "target",
      side: "left",
    },
    {
      description: "Accept a resource requirement for this workstation.",
      edgeKind: "workstation-resource",
      id: "workstation-resource-target",
      label: "Resource",
      role: "target",
      side: "left",
    },
    {
      description: "Route successful output from this workstation.",
      edgeKind: "workstation-output",
      id: "workstation-output-source",
      label: "Success",
      role: "source",
      side: "right",
    },
    {
      description: "Route a continue transition from this workstation.",
      edgeKind: "workstation-on-continue",
      id: "workstation-on-continue-source",
      label: "Continue",
      role: "source",
      side: "right",
    },
    {
      description: "Route a failure transition from this workstation.",
      edgeKind: "workstation-on-failure",
      id: "workstation-on-failure-source",
      label: "Failure",
      role: "source",
      side: "right",
    },
    {
      description: "Route a rejection transition from this workstation.",
      edgeKind: "workstation-on-rejection",
      id: "workstation-on-rejection-source",
      label: "Reject",
      role: "source",
      side: "right",
    },
  ],
};

export function getFactoryGraphConnectionAnchors(kind: FactoryGraphNodeKind) {
  return ANCHORS_BY_KIND[kind];
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

export function buildFactoryGraphConnectionNotice(options: {
  sourceAnchorId: string;
  sourceNode: FactoryGraphNode;
  targetAnchorId: string;
  targetNode: FactoryGraphNode;
}) {
  const sourceAnchor = getFactoryGraphConnectionAnchor(
    options.sourceNode.kind,
    options.sourceAnchorId,
  );
  const targetAnchor = getFactoryGraphConnectionAnchor(
    options.targetNode.kind,
    options.targetAnchorId,
  );
  if (!sourceAnchor || !targetAnchor) {
    return "Choose a compatible source and target anchor before creating a connection.";
  }

  return `${sourceAnchor.label} connections from ${options.sourceNode.label} cannot connect to ${targetAnchor.label} on ${options.targetNode.label}.`;
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
