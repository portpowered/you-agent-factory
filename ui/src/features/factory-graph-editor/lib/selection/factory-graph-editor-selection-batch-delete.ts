import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { buildPendingFactoryDefinition } from "../draft/factory-graph-draft-apply";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphNodeKind,
} from "../draft/factory-graph-draft-types";
import { applyFactoryGraphEdgeRemoval } from "../editor/factory-graph-editor-connections";
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
import type { FactoryGraphEditorSelectionState } from "./factory-graph-editor-selection";
import { removeFromFactoryGraphEditorSelection } from "./factory-graph-editor-selection";

export type FactoryGraphSelectionBatchRemovalSelection = {
  nodeIds: readonly string[];
  edgeIds: readonly string[];
};

export type FactoryGraphSelectionBatchRemovalConfirmation = {
  confirmDescription: string;
  confirmLabel: string;
  ineligibleReason?: string;
  title: string;
};

export type FactoryGraphSelectionBatchRemovalPlan = {
  edgeIds: readonly string[];
  nodeIds: readonly string[];
  confirmation?: FactoryGraphSelectionBatchRemovalConfirmation;
  ineligibleReason?: string;
};

function uniqueSortedIds(ids: readonly string[]): string[] {
  return [...new Set(ids)].sort();
}

function resolveBatchRemovalConfirmation(options: {
  confirmations: FactoryGraphSelectionBatchRemovalConfirmation[];
  itemCount: number;
  locale?: string | null;
}): FactoryGraphSelectionBatchRemovalConfirmation | undefined {
  if (options.confirmations.length === 0) {
    return undefined;
  }

  if (options.confirmations.length === 1 && options.itemCount === 1) {
    return options.confirmations[0];
  }

  const messages = getFactoryGraphEditorMessages(options.locale);
  return {
    confirmDescription: messages.removalBatchDescription(options.itemCount),
    confirmLabel: messages.removalBatchConfirmLabel(options.itemCount),
    title: messages.removalBatchTitle(options.itemCount),
  };
}

export function buildFactoryGraphSelectionBatchRemovalPlan(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  selection: FactoryGraphSelectionBatchRemovalSelection;
}): FactoryGraphSelectionBatchRemovalPlan | null {
  const sortedNodeIds = uniqueSortedIds(options.selection.nodeIds);
  const sortedEdgeIds = uniqueSortedIds(options.selection.edgeIds);
  if (sortedNodeIds.length === 0 && sortedEdgeIds.length === 0) {
    return null;
  }

  const hiddenNodeClasses = options.hiddenNodeClasses ?? new Set();
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
      options.locale,
    ) ?? options.baseFactoryDefinition;
  const topology = buildFactoryGraphTopologyFromDefinition(
    currentFactoryDefinition,
  );
  const nodeIdsToRemove: string[] = [];
  const edgeIdsToRemove: string[] = [];
  const confirmations: FactoryGraphSelectionBatchRemovalConfirmation[] = [];

  for (const nodeId of sortedNodeIds) {
    const node = topology.nodes.find((entry) => entry.id === nodeId);
    if (!node || hiddenNodeClasses.has(node.kind)) {
      continue;
    }

    const docIntent = buildFactoryGraphDocRemovalIntent({
      baseFactoryDefinition: options.baseFactoryDefinition,
      draft: options.draft,
      locale: options.locale,
      nodeId,
    });
    if (docIntent) {
      if (docIntent.ineligibleReason) {
        return {
          edgeIds: [],
          ineligibleReason: docIntent.ineligibleReason,
          nodeIds: [],
        };
      }

      nodeIdsToRemove.push(nodeId);
      if (docIntent.requiresConfirmation) {
        confirmations.push({
          confirmDescription: docIntent.confirmDescription,
          confirmLabel: docIntent.confirmLabel,
          title: docIntent.title,
        });
      }
      continue;
    }

    const intent = buildFactoryGraphRemovalIntent({
      baseFactoryDefinition: options.baseFactoryDefinition,
      draft: options.draft,
      locale: options.locale,
      nodeId,
    });
    if (!intent) {
      continue;
    }
    if (intent.ineligibleReason) {
      return {
        edgeIds: [],
        ineligibleReason: intent.ineligibleReason,
        nodeIds: [],
      };
    }

    nodeIdsToRemove.push(nodeId);
    if (intent.requiresConfirmation) {
      confirmations.push({
        confirmDescription: intent.confirmDescription,
        confirmLabel: intent.confirmLabel,
        title: intent.title,
      });
    }
  }

  const selectedNodeIdSet = new Set(nodeIdsToRemove);
  for (const edgeId of sortedEdgeIds) {
    const edge = topology.edges.find((entry) => entry.id === edgeId);
    if (!edge) {
      continue;
    }
    if (
      selectedNodeIdSet.has(edge.sourceId) ||
      selectedNodeIdSet.has(edge.targetId)
    ) {
      continue;
    }

    const intent = buildFactoryGraphEdgeRemovalIntent({
      baseFactoryDefinition: options.baseFactoryDefinition,
      draft: options.draft,
      edgeId,
      locale: options.locale,
    });
    if (!intent) {
      continue;
    }
    if (intent.ineligibleReason) {
      return {
        edgeIds: [],
        ineligibleReason: intent.ineligibleReason,
        nodeIds: [],
      };
    }

    edgeIdsToRemove.push(edgeId);
    if (intent.requiresConfirmation) {
      confirmations.push({
        confirmDescription: intent.confirmDescription,
        confirmLabel: intent.confirmLabel,
        title: intent.title,
      });
    }
  }

  if (nodeIdsToRemove.length === 0 && edgeIdsToRemove.length === 0) {
    return null;
  }

  const itemCount = nodeIdsToRemove.length + edgeIdsToRemove.length;
  return {
    edgeIds: edgeIdsToRemove,
    nodeIds: nodeIdsToRemove,
    confirmation: resolveBatchRemovalConfirmation({
      confirmations,
      itemCount,
      locale: options.locale,
    }),
  };
}

export function hasDeletableFactoryGraphSelection(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null | undefined;
  draft: FactoryGraphDraft;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  selection: Pick<
    FactoryGraphEditorSelectionState,
    "selectedEdgeIds" | "selectedNodeIds"
  >;
}): boolean {
  if (!options.baseFactoryDefinition) {
    return false;
  }

  const plan = buildFactoryGraphSelectionBatchRemovalPlan({
    baseFactoryDefinition: options.baseFactoryDefinition,
    draft: options.draft,
    hiddenNodeClasses: options.hiddenNodeClasses,
    locale: options.locale,
    selection: {
      edgeIds: [...options.selection.selectedEdgeIds],
      nodeIds: [...options.selection.selectedNodeIds],
    },
  });

  return (
    plan !== null &&
    plan.ineligibleReason === undefined &&
    (plan.nodeIds.length > 0 || plan.edgeIds.length > 0)
  );
}

export function applyFactoryGraphSelectionBatchRemoval(
  currentDraft: FactoryGraphDraft,
  baseFactoryDefinition: CanonicalFactoryDefinition,
  plan: Pick<FactoryGraphSelectionBatchRemovalPlan, "edgeIds" | "nodeIds">,
): FactoryGraphDraft {
  let nextDraft = structuredClone(currentDraft);

  for (const nodeId of uniqueSortedIds(plan.nodeIds)) {
    const docTargetPath = parseFactoryBundledDocNodeId(nodeId);
    if (docTargetPath) {
      nextDraft = applyFactoryGraphDocRemoval(
        nextDraft,
        baseFactoryDefinition,
        docTargetPath,
      );
      continue;
    }

    const intent = buildFactoryGraphRemovalIntent({
      baseFactoryDefinition,
      draft: nextDraft,
      nodeId,
    });
    if (!intent?.key) {
      continue;
    }

    const currentFactoryDefinition =
      buildPendingFactoryDefinition(baseFactoryDefinition, nextDraft) ??
      baseFactoryDefinition;
    nextDraft = applyFactoryGraphEntityRemoval(
      nextDraft,
      currentFactoryDefinition,
      intent.key,
    );
  }

  for (const edgeId of uniqueSortedIds(plan.edgeIds)) {
    const currentFactoryDefinition =
      buildPendingFactoryDefinition(baseFactoryDefinition, nextDraft) ??
      baseFactoryDefinition;
    const topology = buildFactoryGraphTopologyFromDefinition(
      currentFactoryDefinition,
    );
    const edge = topology.edges.find((entry) => entry.id === edgeId);
    if (!edge) {
      continue;
    }

    nextDraft = applyFactoryGraphEdgeRemoval(nextDraft, topology, edge);
  }

  return nextDraft;
}

export function pruneFactoryGraphEditorSelectionAfterRemoval(
  state: FactoryGraphEditorSelectionState,
  removed: {
    edgeIds?: readonly string[];
    nodeIds?: readonly string[];
  },
): FactoryGraphEditorSelectionState {
  return removeFromFactoryGraphEditorSelection(state, {
    edgeIds: removed.edgeIds,
    nodeIds: removed.nodeIds,
  });
}
