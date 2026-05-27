import {
  buildDraftAppliedFactoryDefinition,
  buildPendingFactoryDefinition,
} from "./factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import { validateFactoryGraphDraft } from "./factory-graph-draft-validation";
import {
  applyFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  validateFactoryGraphAddEntityDraft,
} from "./factory-graph-editor-additions";
import {
  applyFactoryGraphEdgeAddition,
  applyFactoryGraphEdgeRemoval,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
} from "./factory-graph-editor-connections";
import {
  applyFactoryGraphEntityRemoval,
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "./factory-graph-editor-removals";

export type FactoryGraphOperationReason =
  | "BLOCKED_REMOVAL"
  | "DUPLICATE_IDENTIFIER"
  | "INVALID_CONNECTION"
  | "INVALID_FIELD"
  | "INVALID_SAVE"
  | "NODE_NOT_FOUND"
  | "PROTECTED_EDGE"
  | "UNKNOWN_EDGE";

export type FactoryGraphOperationResult<T> =
  | {
      ok: true;
      value: T;
    }
  | {
      message: string;
      ok: false;
      reason: FactoryGraphOperationReason;
      fieldErrors?: FactoryGraphAddEntityFieldErrors;
      validationErrors?: FactoryGraphDraftValidationError[];
    };

export interface FactoryGraphState {
  draft: FactoryGraphDraft;
  graph: FactoryGraphTopology;
  pendingFactoryDefinition: CanonicalFactoryDefinition | null;
  saveInput: CanonicalFactoryDefinition | null;
  validationErrors: FactoryGraphDraftValidationError[];
}

export interface FactoryGraphReactFlowNode {
  data: {
    kind: FactoryGraphNode["kind"];
    label: string;
  };
  id: string;
  position: {
    x: number;
    y: number;
  };
  type: FactoryGraphNode["kind"];
}

export interface FactoryGraphReactFlowEdge {
  data: {
    kind: FactoryGraphEdge["kind"];
  };
  id: string;
  source: string;
  target: string;
  type: FactoryGraphEdge["kind"];
}

export interface FactoryGraphReactFlowProjection {
  edges: FactoryGraphReactFlowEdge[];
  nodes: FactoryGraphReactFlowNode[];
}

export function buildFactoryGraphState(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
}): FactoryGraphState {
  const validationErrors = validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
  );
  const pendingFactoryDefinition =
    validationErrors.length === 0
      ? buildDraftAppliedFactoryDefinition(
          options.baseFactoryDefinition,
          options.draft,
        )
      : null;

  return {
    draft: structuredClone(options.draft),
    graph: buildFactoryGraphTopologyFromDefinition(
      pendingFactoryDefinition ?? options.baseFactoryDefinition,
    ),
    pendingFactoryDefinition,
    saveInput: pendingFactoryDefinition,
    validationErrors,
  };
}

export function addFactoryGraphNode(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  node: FactoryGraphAddEntityDraft;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ) ?? options.baseFactoryDefinition;
  const fieldErrors = validateFactoryGraphAddEntityDraft(
    options.node,
    currentFactoryDefinition,
  );
  const firstFieldError = Object.values(fieldErrors).find(Boolean);

  if (firstFieldError) {
    return {
      message: firstFieldError,
      ok: false,
      fieldErrors,
      reason:
        fieldErrors.name !== undefined
          ? "DUPLICATE_IDENTIFIER"
          : "INVALID_FIELD",
    };
  }

  return {
    ok: true,
    value: applyFactoryGraphAddEntityDraft(options.draft, options.node),
  };
}

export function removeFactoryGraphNode(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  nodeId: string;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const intent = buildFactoryGraphRemovalIntent(options);
  if (!intent) {
    return {
      message: `Graph node "${options.nodeId}" was not found.`,
      ok: false,
      reason: "NODE_NOT_FOUND",
    };
  }
  if (intent.ineligibleReason) {
    return {
      message: intent.ineligibleReason,
      ok: false,
      reason: "BLOCKED_REMOVAL",
    };
  }

  return {
    ok: true,
    value: applyFactoryGraphEntityRemoval(
      options.draft,
      options.baseFactoryDefinition,
      intent.key,
    ),
  };
}

export function connectFactoryGraphNodes(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  sourceAnchorId: string;
  sourceNodeId: string;
  targetAnchorId: string;
  targetNodeId: string;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const state = buildFactoryGraphState(options);
  const edgeChange = buildFactoryGraphEdgeChangeFromConnection(state.graph, {
    sourceAnchorId: options.sourceAnchorId,
    sourceNodeId: options.sourceNodeId,
    targetAnchorId: options.targetAnchorId,
    targetNodeId: options.targetNodeId,
  });

  if (!edgeChange) {
    const sourceNode = state.graph.nodes.find(
      (node) => node.id === options.sourceNodeId,
    );
    const targetNode = state.graph.nodes.find(
      (node) => node.id === options.targetNodeId,
    );

    return {
      message:
        sourceNode && targetNode
          ? buildFactoryGraphConnectionNotice({
              sourceAnchorId: options.sourceAnchorId,
              sourceNode,
              targetAnchorId: options.targetAnchorId,
              targetNode,
            })
          : "Choose compatible graph connection handles.",
      ok: false,
      reason: "INVALID_CONNECTION",
    };
  }

  return connectFactoryGraphEdgeChange({
    baseFactoryDefinition: options.baseFactoryDefinition,
    draft: options.draft,
    edgeChange,
  });
}

export function connectFactoryGraphEdgeChange(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  edgeChange: FactoryGraphDraftEdgeChange;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const state = buildFactoryGraphState(options);
  const nextDraft = applyFactoryGraphEdgeAddition(
    options.draft,
    state.graph,
    options.edgeChange,
  );
  const validationErrors = validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    nextDraft,
  );
  if (validationErrors.length > 0) {
    return {
      message: validationErrors[0]?.message ?? "Graph connection is invalid.",
      ok: false,
      reason: "INVALID_CONNECTION",
      validationErrors,
    };
  }

  return {
    ok: true,
    value: nextDraft,
  };
}

export function disconnectFactoryGraphEdge(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  edgeId: string;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const intent = buildFactoryGraphEdgeRemovalIntent(options);
  if (!intent) {
    return {
      message: `Graph edge "${options.edgeId}" was not found.`,
      ok: false,
      reason: "UNKNOWN_EDGE",
    };
  }
  if (intent.ineligibleReason) {
    return {
      message: intent.ineligibleReason,
      ok: false,
      reason: "PROTECTED_EDGE",
    };
  }

  const state = buildFactoryGraphState(options);
  return {
    ok: true,
    value: applyFactoryGraphEdgeRemoval(
      options.draft,
      state.graph,
      intent.edge,
    ),
  };
}

export function validateFactoryGraphState(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
}): FactoryGraphDraftValidationError[] {
  return validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
  );
}

export function applyFactoryGraphPendingEdits(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
}): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const validationErrors = validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
  );
  if (validationErrors.length > 0) {
    return {
      message:
        validationErrors[0]?.message ??
        "Graph edits must be valid before they can be applied.",
      ok: false,
      reason: "INVALID_SAVE",
      validationErrors,
    };
  }

  return {
    ok: true,
    value: buildDraftAppliedFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ),
  };
}

export function buildFactoryGraphSaveInput(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
}): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  return applyFactoryGraphPendingEdits(options);
}

export function projectFactoryGraphToReactFlow(
  graph: FactoryGraphTopology,
): FactoryGraphReactFlowProjection {
  return {
    edges: graph.edges.map((edge) => ({
      data: {
        kind: edge.kind,
      },
      id: edge.id,
      source: edge.sourceId,
      target: edge.targetId,
      type: edge.kind,
    })),
    nodes: graph.nodes.map((node, index) => ({
      data: {
        kind: node.kind,
        label: node.label,
      },
      id: node.id,
      position: {
        x: (index % 4) * 240,
        y: Math.floor(index / 4) * 160,
      },
      type: node.kind,
    })),
  };
}
