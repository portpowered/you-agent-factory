import { getFactoryGraphEditorMessages } from "../messages/editor";
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
  createFactoryGraphWorkstationResolver,
} from "./factory-graph-editor-connections";
import {
  applyFactoryGraphEntityRemoval,
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "./factory-graph-editor-removals";

export {
  type FactoryGraphReactFlowEdge,
  type FactoryGraphReactFlowEditorOverlay,
  type FactoryGraphReactFlowNode,
  type FactoryGraphReactFlowProjection,
  type FactoryGraphReactFlowRuntimeOverlay,
  projectFactoryGraphToReactFlow,
} from "./factory-graph-react-flow-projection";

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

export function buildFactoryGraphState(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  locale?: string | null;
}): FactoryGraphState {
  const validationErrors = validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
    options.locale,
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
  locale?: string | null;
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
    options.locale,
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
  locale?: string | null;
  nodeId: string;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const intent = buildFactoryGraphRemovalIntent(options);
  if (!intent) {
    return {
      message: messages.operationNodeNotFound(options.nodeId),
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
  locale?: string | null;
  sourceAnchorId: string;
  sourceNodeId: string;
  targetAnchorId: string;
  targetNodeId: string;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const state = buildFactoryGraphState(options);
  const workstationResolver = createFactoryGraphWorkstationResolver(
    (
      state.pendingFactoryDefinition ?? options.baseFactoryDefinition
    ).workstations,
  );
  const edgeChange = buildFactoryGraphEdgeChangeFromConnection(
    state.graph,
    {
      sourceAnchorId: options.sourceAnchorId,
      sourceNodeId: options.sourceNodeId,
      targetAnchorId: options.targetAnchorId,
      targetNodeId: options.targetNodeId,
    },
    workstationResolver,
  );

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
              locale: options.locale,
              resolver: workstationResolver,
            })
          : messages.connectionFallbackNotice,
      ok: false,
      reason: "INVALID_CONNECTION",
    };
  }

  return connectFactoryGraphEdgeChange({
    baseFactoryDefinition: options.baseFactoryDefinition,
    draft: options.draft,
    edgeChange,
    locale: options.locale,
  });
}

export function connectFactoryGraphEdgeChange(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  edgeChange: FactoryGraphDraftEdgeChange;
  locale?: string | null;
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
    options.locale,
  );
  if (validationErrors.length > 0) {
    const messages = getFactoryGraphEditorMessages(options.locale);
    return {
      message:
        validationErrors[0]?.message ?? messages.operationConnectionInvalid,
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
  locale?: string | null;
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const messages = getFactoryGraphEditorMessages(options.locale);
  const intent = buildFactoryGraphEdgeRemovalIntent(options);
  if (!intent) {
    return {
      message: messages.operationEdgeNotFound(options.edgeId),
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
  locale?: string | null;
}): FactoryGraphDraftValidationError[] {
  return validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
    options.locale,
  );
}

export function applyFactoryGraphPendingEdits(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  locale?: string | null;
}): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  const validationErrors = validateFactoryGraphDraft(
    options.baseFactoryDefinition,
    options.draft,
    options.locale,
  );
  if (validationErrors.length > 0) {
    const messages = getFactoryGraphEditorMessages(options.locale);
    return {
      message:
        validationErrors[0]?.message ?? messages.operationGraphEditsInvalid,
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
  locale?: string | null;
}): FactoryGraphOperationResult<CanonicalFactoryDefinition> {
  return applyFactoryGraphPendingEdits(options);
}
