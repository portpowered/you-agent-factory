import type { Edge, Node } from "@xyflow/react";

import {
  addToFactoryGraphEditorSelection,
  areFactoryGraphEditorSelectionStatesEqual,
  type FactoryGraphEditorSelectionItems,
  type FactoryGraphEditorSelectionState,
  type FactoryGraphEditorSelectionTarget,
  removeFromFactoryGraphEditorSelection,
  replaceFactoryGraphEditorSelection,
} from "./factory-graph-editor-selection";

export type FactoryGraphEditorSelectionPointerModifiers = {
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
};

export function isToggleSelectionModifier(
  modifiers: FactoryGraphEditorSelectionPointerModifiers,
): boolean {
  return modifiers.metaKey || modifiers.ctrlKey;
}

export function toggleFactoryGraphEditorSelectionItem(
  state: FactoryGraphEditorSelectionState,
  item: FactoryGraphEditorSelectionTarget,
): FactoryGraphEditorSelectionState {
  const selectedIds =
    item.kind === "node" ? state.selectedNodeIds : state.selectedEdgeIds;
  if (selectedIds.has(item.id)) {
    return removeFromFactoryGraphEditorSelection(state, {
      ...(item.kind === "node"
        ? { nodeIds: [item.id] }
        : { edgeIds: [item.id] }),
    });
  }

  return addToFactoryGraphEditorSelection(state, {
    ...(item.kind === "node" ? { nodeIds: [item.id] } : { edgeIds: [item.id] }),
    primaryTarget: item,
  });
}

export function applyGraphItemClickSelection(
  state: FactoryGraphEditorSelectionState,
  item: FactoryGraphEditorSelectionTarget,
  modifiers: FactoryGraphEditorSelectionPointerModifiers,
): FactoryGraphEditorSelectionState {
  if (isToggleSelectionModifier(modifiers)) {
    return toggleFactoryGraphEditorSelectionItem(state, item);
  }

  return replaceFactoryGraphEditorSelection(state, {
    ...(item.kind === "node" ? { nodeIds: [item.id] } : { edgeIds: [item.id] }),
    primaryTarget: item,
  });
}

export function resolveFactoryGraphEdgeIdFromRenderedEdge(
  edge: Pick<Edge, "data" | "id">,
): string {
  const factoryGraphEdgeId = (
    edge.data as { factoryGraphEdgeId?: string } | undefined
  )?.factoryGraphEdgeId;
  return factoryGraphEdgeId ?? edge.id;
}

export function collectFactoryGraphSelectionItemsFromReactFlow(
  nodes: readonly Node[],
  edges: readonly Edge[],
): FactoryGraphEditorSelectionItems {
  const nodeIds = nodes.map((node) => node.id);
  const edgeIds = edges.map((edge) =>
    resolveFactoryGraphEdgeIdFromRenderedEdge(edge),
  );
  const lastEdgeId = edgeIds.at(-1);
  const lastNodeId = nodeIds.at(-1);

  return {
    edgeIds,
    nodeIds,
    primaryTarget: lastEdgeId
      ? { kind: "edge", id: lastEdgeId }
      : lastNodeId
        ? { kind: "node", id: lastNodeId }
        : null,
  };
}

export function applyFactoryGraphEditorReactFlowSelection(
  state: FactoryGraphEditorSelectionState,
  items: FactoryGraphEditorSelectionItems,
  mode: "add" | "replace",
): FactoryGraphEditorSelectionState {
  return mode === "add"
    ? addToFactoryGraphEditorSelection(state, items)
    : replaceFactoryGraphEditorSelection(state, items);
}

export function isFactoryGraphEditorReactFlowSelectionNoOp(
  state: FactoryGraphEditorSelectionState,
  items: FactoryGraphEditorSelectionItems,
  mode: "add" | "replace",
): boolean {
  const nextState = applyFactoryGraphEditorReactFlowSelection(
    state,
    items,
    mode,
  );
  return areFactoryGraphEditorSelectionStatesEqual(state, nextState);
}
