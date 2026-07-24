import { getFactoryGraphEditorMessages } from "../../messages/editor";
import {
  buildDraftAppliedFactoryDefinition,
  buildPendingFactoryDefinition,
} from "../draft/factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import { removeInternalSystemTimeFactoryGraph } from "../draft/factory-graph-draft-save-sanitizer";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import {
  validateFactoryGraphDraft,
  validateFactoryGraphDraftStructural,
} from "../draft/factory-graph-draft-validation";
import {
  applyFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  validateFactoryGraphAddEntityDraft,
} from "../editor/factory-graph-editor-additions";
import {
  applyFactoryGraphEdgeAddition,
  applyFactoryGraphEdgeRemoval,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
  createFactoryGraphWorkstationResolver,
} from "../editor/factory-graph-editor-connections";
import {
  applyFactoryGraphEntityRemoval,
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
} from "../editor-runtime/factory-graph-editor-removals";
import {
  applyFactoryGraphDocRemoval,
  buildFactoryGraphDocRemovalIntent,
  parseFactoryBundledDocNodeId,
} from "../factory-graph-doc-editor";
import {
  applyPendingFactoryLayout,
  type FactoryLayout,
  factoryLayoutFromDefinition,
  hasFactoryLayoutChanges,
} from "../layout/factory-graph-layout-operations";
import { preparePendingFactoryLayoutForSave } from "../layout/factory-graph-layout-validation";
import {
  applyFactoryGraphSelectionBatchRemoval,
  buildFactoryGraphSelectionBatchRemovalPlan,
} from "../selection/factory-graph-editor-selection-batch-delete";
import { materializeFactoryGraphEntityIdsForSave } from "./factory-graph-public-ids";

export {
  type FactoryGraphReactFlowEdge,
  type FactoryGraphReactFlowEditorOverlay,
  type FactoryGraphReactFlowNode,
  type FactoryGraphReactFlowProjection,
  type FactoryGraphReactFlowRuntimeOverlay,
  projectFactoryGraphToReactFlow,
} from "../projection/factory-graph-react-flow-projection";

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
  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    options.baseFactoryDefinition,
    options.draft,
    options.locale,
  );
  const saveInput =
    validationErrors.length === 0 ? pendingFactoryDefinition : null;

  return {
    draft: structuredClone(options.draft),
    graph: buildFactoryGraphTopologyFromDefinition(
      pendingFactoryDefinition ?? options.baseFactoryDefinition,
    ),
    pendingFactoryDefinition,
    saveInput,
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
  const firstFieldError =
    resolveFirstFactoryGraphAddFieldErrorMessage(fieldErrors);

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
  const docTargetPath = parseFactoryBundledDocNodeId(options.nodeId);
  if (docTargetPath) {
    const docIntent = buildFactoryGraphDocRemovalIntent(options);
    if (!docIntent) {
      return {
        message: messages.operationNodeNotFound(options.nodeId),
        ok: false,
        reason: "NODE_NOT_FOUND",
      };
    }
    if (docIntent.ineligibleReason) {
      return {
        message: docIntent.ineligibleReason,
        ok: false,
        reason: "BLOCKED_REMOVAL",
      };
    }

    return {
      ok: true,
      value: applyFactoryGraphDocRemoval(
        options.draft,
        options.baseFactoryDefinition,
        docIntent.targetPath,
      ),
    };
  }

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

export function removeFactoryGraphSelection(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  edgeIds: readonly string[];
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  nodeIds: readonly string[];
}): FactoryGraphOperationResult<FactoryGraphDraft> {
  const plan = buildFactoryGraphSelectionBatchRemovalPlan({
    baseFactoryDefinition: options.baseFactoryDefinition,
    draft: options.draft,
    hiddenNodeClasses: options.hiddenNodeClasses,
    locale: options.locale,
    selection: {
      edgeIds: options.edgeIds,
      nodeIds: options.nodeIds,
    },
  });

  if (!plan) {
    const messages = getFactoryGraphEditorMessages(options.locale);
    return {
      message: messages.operationNodeNotFound("selection"),
      ok: false,
      reason: "NODE_NOT_FOUND",
    };
  }

  if (plan.ineligibleReason) {
    return {
      message: plan.ineligibleReason,
      ok: false,
      reason: "BLOCKED_REMOVAL",
    };
  }

  return {
    ok: true,
    value: applyFactoryGraphSelectionBatchRemoval(
      options.draft,
      options.baseFactoryDefinition,
      plan,
    ),
  };
}

function buildEditingFactoryGraphTopology(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
}): FactoryGraphTopology {
  return buildFactoryGraphTopologyFromDefinition(
    buildDraftAppliedFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ),
  );
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
  const editingFactoryDefinition = buildDraftAppliedFactoryDefinition(
    options.baseFactoryDefinition,
    options.draft,
  );
  const graph = buildEditingFactoryGraphTopology(options);
  const workstationResolver = createFactoryGraphWorkstationResolver(
    editingFactoryDefinition.workstations,
    editingFactoryDefinition.workers,
  );
  const edgeChange = buildFactoryGraphEdgeChangeFromConnection(
    graph,
    {
      sourceAnchorId: options.sourceAnchorId,
      sourceNodeId: options.sourceNodeId,
      targetAnchorId: options.targetAnchorId,
      targetNodeId: options.targetNodeId,
    },
    workstationResolver,
  );

  if (!edgeChange) {
    const sourceNode = graph.nodes.find(
      (node) => node.id === options.sourceNodeId,
    );
    const targetNode = graph.nodes.find(
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
  const graph = buildEditingFactoryGraphTopology(options);
  const nextDraft = applyFactoryGraphEdgeAddition(
    options.draft,
    graph,
    options.edgeChange,
  );
  const validationErrors = validateFactoryGraphDraftStructural(
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

  const graph = buildEditingFactoryGraphTopology(options);
  return {
    ok: true,
    value: applyFactoryGraphEdgeRemoval(options.draft, graph, intent.edge),
  };
}

export function applyFactoryGraphPendingEdits(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  pendingLayout?: FactoryLayout | null;
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

  const baseLayout = factoryLayoutFromDefinition(options.baseFactoryDefinition);
  const nextFactoryDefinition = buildDraftAppliedFactoryDefinition(
    options.baseFactoryDefinition,
    options.draft,
  );
  const pendingTopology = buildFactoryGraphTopologyFromDefinition(
    nextFactoryDefinition,
  );
  const preparedPendingLayoutResult =
    options.pendingLayout == null
      ? null
      : preparePendingFactoryLayoutForSave(
          options.pendingLayout,
          pendingTopology,
        );
  const preparedPendingLayout = preparedPendingLayoutResult?.layout ?? null;
  const nextDefinition =
    preparedPendingLayout &&
    hasFactoryLayoutChanges(baseLayout, preparedPendingLayout)
      ? applyPendingFactoryLayout(nextFactoryDefinition, preparedPendingLayout)
      : nextFactoryDefinition;

  return {
    ok: true,
    value: materializeFactoryGraphEntityIdsForSave(
      removeInternalSystemTimeFactoryGraph(nextDefinition),
    ),
  };
}

function resolveFirstFactoryGraphAddFieldErrorMessage(
  fieldErrors: FactoryGraphAddEntityFieldErrors,
): string | undefined {
  for (const value of Object.values(fieldErrors)) {
    if (!value) {
      continue;
    }
    if (typeof value === "string") {
      return value;
    }
    if (typeof value.summary === "string" && value.summary.length > 0) {
      return value.summary;
    }
  }
  return undefined;
}
